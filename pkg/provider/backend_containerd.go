package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/google/uuid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"gitlab.ilabt.imec.be/fledge/service/pkg/storage"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type ContainerdBackend struct {
	config Config

	context context.Context
	client  *containerd.Client
}

func NewContainerdBackend(ctx context.Context, cfg Config) (*ContainerdBackend, error) {
	client, err := containerd.New(
		"/run/containerd/containerd.sock",
		containerd.WithDefaultNamespace("fledge"),
		containerd.WithDefaultPlatform(platforms.Default()),
	)
	if client == nil {
		return nil, errors.Wrap(err, "containerd")
	}

	b := &ContainerdBackend{
		config:  cfg,
		context: namespaces.WithNamespace(ctx, "fledge"),
		client:  client,
	}

	// Scrub old containerd instances
	containers, err := b.client.Containers(b.context)
	if err != nil {
		log.G(b.context).Error(errors.Wrap(err, "containerd"))
	}
	for _, c := range containers {
		log.G(b.context).Info(c.ID())
		err = b.DeleteInstance(&Instance{ID: c.ID()})
		if err != nil {
			log.G(b.context).Error(errors.Wrap(err, "containerd"))
		}
	}

	return b, nil
}

func (b *ContainerdBackend) GetInstanceStatus(instance *Instance) (corev1.ContainerStatus, error) {
	// Load existing container
	container, err := b.client.LoadContainer(b.context, instance.ID)
	if err != nil {
		err = errors.Wrap(err, "containerd")
		return corev1.ContainerStatus{}, err
	}
	// Get container info
	info, err := container.Info(b.context)
	if err != nil {
		err = errors.Wrap(err, "containerd")
		return corev1.ContainerStatus{}, err
	}
	task, err := container.Task(b.context, nil)
	if err != nil {
		err = errors.Wrap(err, "containerd")
		return corev1.ContainerStatus{}, err
	}
	taskStatus, err := task.Status(b.context)
	if err != nil {
		err = errors.Wrap(err, "containerd")
		return corev1.ContainerStatus{}, err
	}
	// Return status (https://pkg.go.dev/github.com/containerd/containerd#ProcessStatus)
	state := corev1.ContainerState{}
	switch taskStatus.Status {
	case containerd.Created:
		state.Waiting = &corev1.ContainerStateWaiting{
			Reason:  "Starting",
			Message: "Starting container",
		}
	case containerd.Running:
		fallthrough
	case containerd.Pausing:
		fallthrough
	case containerd.Paused:
		state.Running = &corev1.ContainerStateRunning{
			StartedAt: metav1.NewTime(taskStatus.ExitTime),
		}
	case containerd.Stopped:
		fallthrough
	case containerd.Unknown:
		state.Terminated = &corev1.ContainerStateTerminated{
			ExitCode:    int32(taskStatus.ExitStatus),
			Reason:      "Stopped",
			Message:     "Container stopped",
			StartedAt:   metav1.NewTime(info.CreatedAt),
			FinishedAt:  metav1.NewTime(taskStatus.ExitTime),
			ContainerID: container.ID(),
		}
	}
	return corev1.ContainerStatus{
		Name:  info.ID,
		State: state,
	}, nil
}

func (b *ContainerdBackend) CreateInstance(instance *Instance) error {
	// Clean up pre-existing instance (TODO can we do this in SIGKILL or on startup?)
	err := b.DeleteInstance(instance)
	if err != nil {
		log.G(b.context).Error(err)
	}

	// Container.Image
	image, err := b.client.GetImage(b.context, instance.Image)
	if (err != nil && instance.ImagePullPolicy == corev1.PullIfNotPresent) || instance.ImagePullPolicy == corev1.PullAlways {
		if instance.ImagePullPolicy == corev1.PullAlways {
			b.client.ImageService().Delete(b.context, instance.Image)
		}
		named, err := refdocker.ParseDockerRef(instance.Image)
		if err != nil {
			return errors.Wrap(err, "containerd")
		}
		ref := named.String()
		refDomain := refdocker.Domain(named)
		resolver, err := dockerconfigresolver.New(b.context, refDomain)
		if err != nil {
			return errors.Wrap(err, "containerd")
		}
		pullOpts := []containerd.RemoteOpt{containerd.WithResolver(resolver), containerd.WithPullUnpack, containerd.WithSchema1Conversion}
		image, err = b.client.Pull(b.context, ref, pullOpts...)
		if err != nil {
			return errors.Wrap(err, "containerd")
		}
	}

	// Get container and specification options
	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
		containerd.WithImageConfigLabels(image),
		containerd.WithSnapshotter(containerd.DefaultSnapshotter),
		containerd.WithNewSnapshot(instance.ID, image),
		containerd.WithImageStopSignal(image, "SIGTERM"),
	}
	var specOpts []oci.SpecOpts
	// Container.Command
	imageArgs := append(instance.Command, instance.Args...)
	// Container.WorkingDir
	processCwd := instance.WorkingDir
	if len(processCwd) > 0 {
		specOpts = append(specOpts, oci.WithProcessCwd(processCwd))
	}
	// Container.Ports
	portsContainerOpts, err := b.getPortsOpts(instance.Ports)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}
	containerOpts = append(containerOpts, portsContainerOpts...)
	// Container.EnvFrom (TODO)
	// Container.Env
	env := make([]string, 0)
	for _, envVar := range instance.Env {
		if envVar.ValueFrom != nil {
			// TODO
			envVar = corev1.EnvVar{
				Name:  "DUMMY",
				Value: "",
			}
		}
		// Expand variable in the container arguments
		for i, a := range imageArgs {
			imageArgs[i] = strings.Replace(a, fmt.Sprintf("$(%s)", envVar.Name), envVar.Value, -1)
		}
		env = append(env, fmt.Sprintf("%s=%q", envVar.Name, envVar.Value))
	}
	specOpts = append(specOpts, oci.WithEnv(env))
	// Container.Resources (TODO)
	if cpuLimitMillis := instance.Resources.Limits.Cpu().MilliValue(); cpuLimitMillis > 0 {
		var (
			period = uint64(100000)
			quota  = 100 * cpuLimitMillis
		)
		specOpts = append(specOpts, oci.WithCPUCFS(quota, period))
	}
	if memoryLimit := instance.Resources.Limits.Memory().Value(); memoryLimit > 0 {
		specOpts = append(specOpts, oci.WithMemoryLimit(uint64(memoryLimit)))
	}
	// Container.VolumeMounts
	volumeMountsContainerOpts, volumeMountsSpecOpts, err := b.getVolumeMountsOpts(instance.VolumeMounts)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}
	containerOpts = append(containerOpts, volumeMountsContainerOpts...)
	specOpts = append(specOpts, volumeMountsSpecOpts...)
	// Container.VolumeDevices (TODO)
	// Container.LivenessProbe (TODO)
	// Container.ReadinessProbe (TODO)
	// Container.StartupProbe (TODO)
	// Container.Lifecycle (TODO)
	// Container.TerminationMessagePath (TODO)
	// Container.SecurityContext(TODO)
	// Container.Stdin (TODO)
	// Container.StdinOnce (TODO)
	// Container.TTY (TODO)

	// Pod.HostNetwork
	if !instance.HostNetwork {
		// TODO: Network access inside the container fails without this, but we do not always want host networking
		hostNetworkSpecOpts := []oci.SpecOpts{
			oci.WithHostNamespace(specs.NetworkNamespace),
			oci.WithHostHostsFile,
			oci.WithHostResolvconf,
		}
		specOpts = append(specOpts, hostNetworkSpecOpts...)
	} else {
		hostNetworkSpecOpts := []oci.SpecOpts{
			oci.WithHostNamespace(specs.NetworkNamespace),
			oci.WithHostHostsFile,
			oci.WithHostResolvconf,
		}
		specOpts = append(specOpts, hostNetworkSpecOpts...)
	}

	// Add ImageConfig
	if len(imageArgs) == 0 {
		specOpts = append(specOpts, oci.WithImageConfig(image))
	} else {
		specOpts = append(specOpts, oci.WithImageConfigArgs(image, imageArgs))
	}

	// Create container
	containerOpts = append(containerOpts, containerd.WithNewSpec(specOpts...))
	container, err := b.client.NewContainer(
		b.context,
		instance.ID,
		containerOpts...,
	)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Create logsFile
	r, w, _ := os.Pipe()
	if err = os.MkdirAll(b.instanceDir(instance), 0775); err != nil {
		return errors.Wrap(err, "containerd")
	}
	logsFile, err := os.Create(b.instanceLogsPath(instance))
	if err != nil {
		return errors.Wrap(err, "containerd")
	}
	// Write everything to a log file continuously
	go func() { _, _ = io.Copy(logsFile, r) }()

	// Create container IO
	cioOpts := []cio.Opt{cio.WithStreams(os.Stdin, w, w)}
	if instance.TTY {
		cioOpts = append(cioOpts, cio.WithTerminal)
	}
	ioCreator := cio.NewCreator(cioOpts...)
	// Create new task
	containerTask, err := container.NewTask(
		b.context,
		ioCreator,
	)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Wait for task to be created
	_, err = containerTask.Wait(b.context)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	return nil
}

func (b *ContainerdBackend) StartInstance(instance *Instance) error {
	// Load existing container
	container, err := b.client.LoadContainer(b.context, instance.ID)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Load existing task
	task, err := container.Task(b.context, nil)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Start task
	if err = task.Start(b.context); err != nil {
		return errors.Wrap(err, "containerd")
	}

	return nil
}

func (b *ContainerdBackend) UpdateInstance(instance *Instance) error {
	// TODO: Can we do this more performant?
	if err := b.DeleteInstance(instance); err != nil {
		return err
	}
	return b.CreateInstance(instance)
}

func (b *ContainerdBackend) KillInstance(instance *Instance, signal syscall.Signal) error {
	// Load existing container
	container, err := b.client.LoadContainer(b.context, instance.ID)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Load existing task
	task, err := container.Task(b.context, nil)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Kill task
	killOpts := []containerd.KillOpts{containerd.WithKillAll}
	if err = task.Kill(b.context, signal, killOpts...); err != nil {
		return errors.Wrap(err, "containerd")
	}

	return nil
}

func (b *ContainerdBackend) DeleteInstance(instance *Instance) error {
	// Load existing container
	container, err := b.client.LoadContainer(b.context, instance.ID)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	err = func() error {
		//Load existing task
		task, err := container.Task(b.context, nil)
		if errdefs.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		// Delete task
		tDeleteOpts := []containerd.ProcessDeleteOpts{containerd.WithProcessKill}
		if _, err = task.Delete(b.context, tDeleteOpts...); err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Delete container
	cDeleteOpts := []containerd.DeleteOpts{containerd.WithSnapshotCleanup}
	if err = container.Delete(b.context, cDeleteOpts...); err != nil {
		return errors.Wrap(err, "containerd")
	}

	return nil
}

func (b *ContainerdBackend) GetInstanceLogs(instance *Instance, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	containerLogger, err := NewContainerLogger(b.instanceLogsPath(instance), opts)
	if err != nil {
		return nil, errors.Wrap(err, "containerd")
	}
	return containerLogger, nil
}

func (b *ContainerdBackend) RunInInstance(instance *Instance, cmd []string, attach api.AttachIO) error {
	// Load existing container
	container, err := b.client.LoadContainer(b.context, instance.ID)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Load existing task
	task, err := container.Task(b.context, nil)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Generate process ID
	execID := fmt.Sprintf("exec-%s", uuid.New().String())

	// Create process
	pSpec := specs.Process{
		Terminal: attach.TTY(),
		Args:     cmd,
	}

	// Create container IO
	cioOpts := []cio.Opt{cio.WithStreams(attach.Stdin(), attach.Stdout(), attach.Stderr())}
	if attach.TTY() {
		cioOpts = append(cioOpts, cio.WithTerminal)
	}
	ioCreator := cio.NewCreator(cioOpts...)

	// Exec process in task
	process, err := task.Exec(b.context, execID, &pSpec, ioCreator)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}
	defer process.Delete(b.context)

	statusC, err := process.Wait(b.context)
	if err != nil {
		return errors.Wrap(err, "containerd")
	}

	if err = process.Start(b.context); err != nil {
		return errors.Wrap(err, "containerd")
	}

	// Get status code
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return errors.Wrap(err, "containerd")
	}
	if code != 0 {
		err = fmt.Errorf("exec failed with exit code %d", code)
		return errors.Wrap(err, "containerd")
	}

	return nil
}

func (b *ContainerdBackend) getPortsOpts(containerPorts []corev1.ContainerPort) ([]containerd.NewContainerOpts, error) {
	// TODO:  ad-hoc; check nerdctl/cmd/nerdctl/container_run_network.go
	var ports []gocni.PortMapping
	for _, cp := range containerPorts {
		// TODO: Just expose a port for now, do nothing special
		port := gocni.PortMapping{
			HostPort:      cp.HostPort,
			ContainerPort: cp.ContainerPort,
			Protocol:      string(cp.Protocol),
			HostIP:        cp.HostIP,
		}
		ports = append(ports, port)
	}

	portsJson, err := json.Marshal(ports)
	if err != nil {
		return nil, errors.Wrap(err, "containerd")
	}
	portsLabels := map[string]string{labels.Ports: string(portsJson)}
	return []containerd.NewContainerOpts{containerd.WithAdditionalContainerLabels(portsLabels)}, nil
}

func (b *ContainerdBackend) getVolumeMountsOpts(volumeMounts []InstanceVolumeMount) ([]containerd.NewContainerOpts, []oci.SpecOpts, error) {
	var mounts []specs.Mount
	for _, vm := range volumeMounts {
		v := vm.Volume
		switch {
		case v.HostPath != nil:
			switch *v.HostPath.Type {
			case corev1.HostPathDirectoryOrCreate:
				if err := os.MkdirAll(v.HostPath.Path, 0755); err != nil {
					return nil, nil, err
				}
				fallthrough
			case "":
				fallthrough
			case corev1.HostPathDirectory:
			default:
				return nil, nil, errors.Errorf("volumeMount %q has unsupported hostPath.type %q", v.ID, *v.HostPath.Type)
			}
			options := []string{"bind"}
			if vm.ReadOnly {
				options = append(options, "ro")
			}
			mount := specs.Mount{
				Type:        "bind",
				Source:      v.HostPath.Path,
				Destination: vm.MountPath,
				Options:     options,
			}
			mounts = append(mounts, mount)
		case v.EmptyDir != nil: // TODO (ignored)
		// TODO: GCEPersistentDisk *corev1.GCEPersistentDiskVolumeSource
		// TODO: AWSElasticBlockStore *corev1.AWSElasticBlockStoreVolumeSource
		// TODO: GitRepo *corev1.GitRepoVolumeSource
		case v.Secret != nil: // TODO (ignored)
		case v.NFS != nil: // TODO (ignored)
		// TODO: ISCSI *ISCSIVolumeSource
		// TODO: Glusterfs *GlusterfsVolumeSource
		// TODO: PersistentVolumeClaim *PersistentVolumeClaimVolumeSource
		// TODO: RBD *RBDVolumeSource
		// TODO: FlexVolume *FlexVolumeSource
		// TODO: Cinder *CinderVolumeSource
		// TODO: CephFS *CephFSVolumeSource
		// TODO: Flocker *FlockerVolumeSource
		// TODO: DownwardAPI *DownwardAPIVolumeSource
		// TODO: FC *FCVolumeSource
		// TODO: AzureFile *AzureFileVolumeSource
		case v.ConfigMap != nil: // TODO (ignored)
			// TODO (ignored)
			//for k, v := range volume.ConfigMap.Items {
			//}
			// Create the configmap files in tmpfs (TODO: possible insecure)
			tempDir, err := os.MkdirTemp("", "")
			if err != nil {
				return nil, nil, err
			}
			for path, data := range v.ConfigMap.Object.Data {
				tempPath := filepath.Join(tempDir, path)
				if err := os.WriteFile(tempPath, []byte(data), 0644); err != nil {
					return nil, nil, err
				}
				options := []string{"rbind"}
				if vm.ReadOnly {
					options = append(options, "ro")
				} else {
					options = append(options, "rw")
				}
				mount := specs.Mount{
					Type:        "bind",
					Source:      tempPath,
					Destination: filepath.Join(vm.MountPath, path),
					Options:     options,
				}
				mounts = append(mounts, mount)
			}
		// TODO: VsphereVolume *VsphereVirtualDiskVolumeSource
		// TODO: Quobyte *QuobyteVolumeSource
		// TODO: AzureDisk *AzureDiskVolumeSource
		// TODO: PhotonPersistentDisk *PhotonPersistentDiskVolumeSource
		case v.Projected != nil:
			// Create directory to prepare for projection
			path := b.volumeDir(&v)
			if err := os.MkdirAll(path, 0755); err != nil {
				return nil, nil, err
			}
			// TODO: Solve this with another virtio-fs mount?
			for _, s := range v.Projected.Sources {
				switch {
				case s.Secret != nil: // TODO (ignored)
					for key, val := range s.Secret.Object.StringData {
						file := filepath.Join(path, key)
						// TODO: permissions
						if err := os.WriteFile(file, []byte(val), 0755); err != nil {
							return nil, nil, err
						}
					}
					for key, val := range s.Secret.Object.Data {
						file := filepath.Join(path, key)
						// TODO: permissions
						if err := os.WriteFile(file, val, 0755); err != nil {
							return nil, nil, err
						}
					}
				case s.DownwardAPI != nil: // TODO (ignored)
					// TODO
				case s.ConfigMap != nil: // TODO (ignored)
					for key, val := range s.ConfigMap.Object.Data {
						file := filepath.Join(path, key)
						// TODO: permissions
						if err := os.WriteFile(file, []byte(val), 0755); err != nil {
							return nil, nil, err
						}
					}
				case s.ServiceAccountToken != nil: // TODO (ignored)
					for key, val := range s.ServiceAccountToken.Object.StringData {
						file := filepath.Join(path, key)
						// TODO: permissions
						if err := os.WriteFile(file, []byte(val), 0755); err != nil {
							return nil, nil, err
						}
					}
					for key, val := range s.ServiceAccountToken.Object.Data {
						file := filepath.Join(path, key)
						// TODO: permissions
						if err := os.WriteFile(file, val, 0755); err != nil {
							return nil, nil, err
						}
					}
				}
			}
			options := []string{"rbind"}
			if vm.ReadOnly {
				options = append(options, "ro")
			}
			mount := specs.Mount{
				Type:        "bind",
				Source:      path,
				Destination: vm.MountPath,
				Options:     options,
			}
			mounts = append(mounts, mount)
		// TODO: PortworxVolume *PortworxVolumeSource
		// TODO: ScaleIO *ScaleIOVolumeSource
		// TODO: StorageOS *StorageOSVolumeSource
		// TODO: CSI *CSIVolumeSource
		// TODO: Ephemeral *EphemeralVolumeSource
		default:
			err := errors.Errorf("volumeMount %q has an unsupported type", vm.Name)
			return nil, nil, errors.Wrap(err, "osv")
		}
	}

	mountsJson, err := json.Marshal(mounts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "containerd")
	}
	mountsLabels := map[string]string{labels.Mounts: string(mountsJson)}
	return []containerd.NewContainerOpts{containerd.WithAdditionalContainerLabels(mountsLabels)}, []oci.SpecOpts{oci.WithMounts(mounts)}, nil
}

// Ensure interface is implemented
var _ Backend = (*ContainerdBackend)(nil)

func (b *ContainerdBackend) instanceDir(instance *Instance) string {
	return storage.InstancePath(instance.ID)
}

func (b *ContainerdBackend) instanceLogsPath(instance *Instance) string {
	return filepath.Join(b.instanceDir(instance), "current.logs")
}

func (b *ContainerdBackend) volumeDir(volume *InstanceVolume) string {
	return storage.VolumePath(volume.ID)
}
