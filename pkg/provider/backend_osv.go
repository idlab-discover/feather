package provider

import (
	"encoding/json"
	"fmt"
	"github.com/cloudius-systems/capstan/core"
	"github.com/cloudius-systems/capstan/hypervisor/qemu"
	"github.com/cloudius-systems/capstan/nat"
	"github.com/containerd/containerd/log"
	"github.com/pkg/errors"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	"gitlab.ilabt.imec.be/fledge/service/pkg/storage"
	"gitlab.ilabt.imec.be/fledge/service/pkg/system"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"golang.org/x/net/context"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	capstan "github.com/cloudius-systems/capstan/util"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	corev1 "k8s.io/api/core/v1"
)

type OSvBackend struct {
	config Config

	context context.Context
	repo    *capstan.Repo

	instanceStatuses map[string]*corev1.ContainerStatus
	instanceExtras   map[string]*OSvExtras
	volumeExtras     map[string]*OSvExtras
}

func NewOSvBackend(ctx context.Context, cfg Config) (*OSvBackend, error) {
	repo := capstan.NewRepo("")

	b := &OSvBackend{
		config:           cfg,
		context:          ctx,
		repo:             repo,
		instanceStatuses: map[string]*corev1.ContainerStatus{},
		instanceExtras:   map[string]*OSvExtras{},
		volumeExtras:     map[string]*OSvExtras{},
	}

	return b, nil
}

func (b *OSvBackend) GetInstanceStatus(instance *Instance) (corev1.ContainerStatus, error) {
	if instanceStatus, ok := b.instanceStatuses[instance.ID]; ok {
		return *instanceStatus, nil
	}
	err := errors.Errorf("instance %q does not exist", instance.ID)
	return corev1.ContainerStatus{}, errors.Wrap(err, "osv")
}

func (b *OSvBackend) CreateInstance(instance *Instance) error {
	// Clean up pre-existing instance (TODO can we do this in SIGKILL or on startup?)
	// Otherwise capstan will start a pre-existing instance that was stopped
	b.DeleteInstance(instance)

	// Get image config
	imageConf, err := storage.ImageGetConfig(b.context, instance.Image)
	if err != nil {
		return errors.Wrap(err, "osv")
	}

	// Container.Image
	imagePath := filepath.Join(b.repo.RepoPath(), "fledge", storage.CleanName(instance.Image))
	// Check if the image exists
	_, err = os.Stat(imagePath)
	imageExists := !os.IsNotExist(err)
	// Pull image if required
	if (!imageExists && instance.ImagePullPolicy == corev1.PullIfNotPresent) || instance.ImagePullPolicy == corev1.PullAlways {
		if err = b.pullInstanceImage(instance.Image, imageConf.Hypervisor); err != nil {
			return errors.Wrap(err, "osv")
		}
	}

	// Get hypervisor options
	// Container.Image
	image := b.imageDiskPath(instance.Image, imageConf.Hypervisor)
	// Container.Command
	cmd := append(instance.Command, instance.Args...)
	if cmd == nil || len(cmd) == 0 {
		cmd = []string{"runscript", "/run/default;"}
	}
	// Container.WorkingDir (TODO)
	// Container.Ports
	networking := "bridge"
	if len(instance.Ports) > 0 {
		networking = "nat"
	}
	natRules := make([]nat.Rule, 0)
	for _, p := range instance.Ports {
		// TODO: Support for HostIP?
		// TODO: Do we need NAT or can we do Bridge? Use p.HostPort?
		hostPort := int(p.HostPort)
		if hostPort == 0 {
			hostPort, err = system.AvailablePort()
			if err != nil {
				log.G(b.context).Error(errors.Wrap(err, "osv backend"))
				continue
			}
		}
		natRules = append(natRules, nat.Rule{
			HostPort:  strconv.FormatInt(int64(hostPort), 10),
			GuestPort: strconv.FormatInt(int64(p.ContainerPort), 10),
		})
	}
	// Container.EnvFrom (TODO)
	// Container.Env (TODO)
	// Container.Resources
	// TODO: Container.Resources.Requests
	vmCpus := 4 // 4 vCPUs
	if cpuLimit := instance.Resources.Limits.Cpu().Value(); cpuLimit > 0 {
		vmCpus = int(cpuLimit)
	}
	vmMemory := 1024 // 1024 MiB
	if memoryLimit := instance.Resources.Limits.Memory().Value(); memoryLimit > 0 {
		vmMemory = int(memoryLimit >> 20)
	}
	// Container.VolumeMounts
	instanceExtras := &OSvExtras{
		vmArgs: []string{
			"-object", fmt.Sprintf("memory-backend-file,id=mem,size=%dM,mem-path=/dev/shm,share=on", vmMemory),
			"-numa", "node,memdev=mem",
		},
		vmOpts: []string{"--rootfs=zfs", "--verbose"},
	}
	for i, vm := range instance.VolumeMounts {
		volumeMountExtras, err := b.getVolumeMountExtras(instance, i, vm)
		if err != nil {
			return errors.Wrap(err, "osv")
		}
		instanceExtras.extendWith(volumeMountExtras)
	}
	cmd = append(instanceExtras.vmOpts, cmd...)
	b.instanceExtras[instance.ID] = instanceExtras
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

	// Show command in logs
	log.G(b.context).Infof("Setting cmdline: %s\n", strings.Join(cmd, " "))

	// Create hypervisor config
	switch imageConf.Hypervisor {
	case "qemu":
		dir := b.instanceDir(instance)
		conf := &qemu.VMConfig{
			Name:        instance.ID,
			Verbose:     true,
			Cmd:         strings.Join(cmd, " "),
			DisableKvm:  false,
			Persist:     false,
			InstanceDir: dir,
			Monitor:     b.instanceMoniPath(instance),
			ConfigFile:  b.instanceConfPath(instance),
			AioType:     b.repo.QemuAioType,
			Image:       image,
			BackingFile: true,
			Volumes:     []string{}, // TODO
			Memory:      int64(vmMemory),
			Cpus:        vmCpus,
			Networking:  networking,
			Bridge:      "virbr0", // TODO
			NatRules:    natRules,
			MAC:         "", // TODO
			VNCFile:     b.instanceSockPath(instance),
		}
		if err = os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrap(err, "osv")
		}
		if err = qemu.StoreConfig(conf); err != nil {
			return errors.Wrap(err, "osv")
		}
	default:
		err = errors.Errorf("platform %q is not supported", imageConf.Hypervisor)
		return errors.Wrap(err, "osv")
	}

	// ContainerStatus.Name
	parts := splitIdentifierIntoParts(instance.ID)
	name := parts[len(parts)-1]
	// ContainerStatus.State
	state := corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{
			Reason:  "Created",
			Message: "Instance is created",
		},
	}
	// ContainerStatus.LastTerminationState
	lastTerminationState := corev1.ContainerState{}
	if instanceStatus, ok := b.instanceStatuses[instance.ID]; ok {
		lastTerminationState = instanceStatus.State
	}
	// ContainerStatus.Started
	started := false
	// Instance is created, populate its status
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#containerstatus-v1-core
	b.instanceStatuses[instance.ID] = &corev1.ContainerStatus{
		Name:                 name,
		State:                state,
		LastTerminationState: lastTerminationState,
		Ready:                true, // Default value
		RestartCount:         0,    // Default value
		Image:                instance.Image,
		ImageID:              "",
		ContainerID:          fmt.Sprintf("capstan://%s", instance.ID),
		Started:              &started,
	}

	return nil
}

func (b *OSvBackend) StartInstance(instance *Instance) error {
	instanceName, instancePlatform := capstan.SearchInstance(instance.ID)
	if instanceName == "" {
		err := errors.Errorf("instance %q does not exist", instance.ID)
		return errors.Wrap(err, "osv")
	}

	// Get extras for instance
	extras, ok := b.instanceExtras[instance.ID]
	if !ok {
		err := errors.Errorf("instance %q does not have extras", instance.ID)
		return errors.Wrap(err, "osv")
	}

	// Get config for platform
	var (
		cmd  *exec.Cmd
		pErr error
	)
	switch instancePlatform {
	case "qemu":
		conf, err := qemu.LoadConfig(instanceName)
		if err != nil {
			return errors.Wrap(err, "osv")
		}
		cmd, err = qemu.VMCommand(conf, true, extras.vmArgs...)
		if err != nil {
			return errors.Wrap(err, "osv")
		}
	default:
		pErr = errors.Errorf("platform %q is not supported", instancePlatform)
		return errors.Wrap(pErr, "osv")
	}
	r, w, _ := os.Pipe()
	// Start side processes
	ctx, cancel := context.WithCancel(b.context)
	procs := make([]*exec.Cmd, 0)
	for _, p := range extras.vmProc {
		proc := exec.CommandContext(ctx, p[0], p[1:]...)
		proc.Stdout, proc.Stderr = w, w
		if err := proc.Start(); err != nil {
			cancel()
			return errors.Wrap(err, "osv")
		}
		go func() {
			pErr = proc.Wait()
			_, exitMsg := util.ExecParseError(proc.Wait())
			log.G(ctx).Debugf("process %q %s\n", proc.String(), exitMsg)
		}()
		procs = append(procs, proc)
	}
	// Create logfile
	log.G(b.context).Infof("Started instance %q (backend=osv)", instance.ID)
	logsFile, pErr := os.Create(b.instanceLogsPath(instance))
	if pErr != nil {
		return errors.Wrap(pErr, "osv")
	}
	cmd.Stdout, cmd.Stderr = w, w
	// Write everything to a log file continuously
	go func() { _, _ = io.Copy(logsFile, r) }()
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "osv")
	}

	// Instance is started, update its status
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#containerstaterunning-v1-core
	instanceStatus := b.instanceStatuses[instance.ID]
	instanceStatus.State = corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{
			StartedAt: metav1.NewTime(time.Now()),
		},
	}

	// Run goroutine that waits for the instance to exit
	go func() {
		exitCode, exitMsg := util.ExecParseError(cmd.Wait())
		log.G(ctx).Debugf("instance %s\n", exitMsg)
		// Cancel subprocesses
		cancel()
		// Close logfile stream
		logsFile.Close()
		// Instance has terminated, update its status
		// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#containerstateterminated-v1-core
		instanceStatus.State = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode:    int32(exitCode),
				Signal:      int32(syscall.SIGTERM), // Default
				Reason:      "Terminated",
				Message:     exitMsg,
				StartedAt:   instanceStatus.State.Running.StartedAt,
				FinishedAt:  metav1.NewTime(time.Now()),
				ContainerID: instance.ID,
			},
		}
	}()

	return nil
}

func (b *OSvBackend) UpdateInstance(instance *Instance) error {
	// TODO: Can we do this more performant?
	if err := b.DeleteInstance(instance); err != nil {
		return err
	}
	return b.CreateInstance(instance)
}

func (b *OSvBackend) KillInstance(instance *Instance, signal syscall.Signal) error {
	instanceName, instancePlatform := capstan.SearchInstance(instance.ID)
	if instanceName == "" {
		err := errors.Errorf("instance %q does not exist", instance.ID)
		return errors.Wrap(err, "osv")
	}

	// Stop instance with platform
	var err error
	switch instancePlatform {
	case "qemu":
		err = qemu.StopVM(instance.ID)
	default:
		err = errors.Errorf("platform %q is not supported", instancePlatform)
		return errors.Wrap(err, "osv")
	}
	if err != nil {
		return errors.Wrap(err, "osv")
	}
	return nil
}

func (b *OSvBackend) DeleteInstance(instance *Instance) error {
	instanceName, instancePlatform := capstan.SearchInstance(instance.ID)
	if instanceName == "" {
		err := errors.Errorf("instance %q does not exist", instance.ID)
		return errors.Wrap(err, "osv")
	}

	// Stop instance with platform
	var err error
	switch instancePlatform {
	case "qemu":
		qemu.StopVM(instance.ID)
		err = qemu.DeleteVM(instance.ID)
	default:
		err = errors.Errorf("platform %q is not supported", instancePlatform)
		return errors.Wrap(err, "osv")
	}
	if err != nil {
		return errors.Wrap(err, "osv")
	}

	// Instance is deleted, remove its status (TODO: last termination state)
	delete(b.instanceStatuses, instance.ID)

	return nil
}

func (b *OSvBackend) GetInstanceLogs(instance *Instance, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	containerLogger, err := NewContainerLogger(b.instanceLogsPath(instance), opts)
	if err != nil {
		return nil, errors.Wrap(err, "osv")
	}
	return containerLogger, nil
}

func (b *OSvBackend) RunInInstance(instance *Instance, cmd []string, attach api.AttachIO) error {
	return nil
}

type OSvExtras struct {
	// vmProc specify extra processes (e.g. daemons) that needs to be started before the vm
	vmProc [][]string
	// vmArgs specify extra arguments (e.g. mounts) that need to be passed to the hypervisor
	vmArgs []string
	// vmOpts specify extra options (e.g. mounts) that need to be passed to the virtual machine
	vmOpts []string
}

func (e *OSvExtras) extendWith(other *OSvExtras) {
	e.vmProc = append(e.vmProc, other.vmProc...)
	e.vmArgs = append(e.vmArgs, other.vmArgs...)
	e.vmOpts = append(e.vmOpts, other.vmOpts...)
}

func (b *OSvBackend) getVolumeMountExtras(instance *Instance, volumeMountIndex int, volumeMount InstanceVolumeMount) (*OSvExtras, error) {
	volume := volumeMount.Volume

	// Create volumes directory
	volumesDir := b.volumesDir()
	if _, err := os.Stat(volumesDir); os.IsNotExist(err) {
		if err = os.MkdirAll(volumesDir, 0755); err != nil {
			return nil, err
		}
	}

	// Populate extras
	extras := &OSvExtras{}
	switch {
	case volume.HostPath != nil:
		switch *volume.HostPath.Type {
		case corev1.HostPathDirectoryOrCreate:
			if err := os.MkdirAll(volume.HostPath.Path, 0755); err != nil {
				return nil, err
			}
			fallthrough
		case corev1.HostPathDirectory:
		default:
			return nil, errors.Errorf("volumeMount %q has unsupported hostPath.type %q", volume.ID, volume.HostPath.Type)
		}
		/*
			Create options for virtio-fs socket according to scripts/run.py
			https://raw.githubusercontent.com/cloudius-systems/osv/master/scripts/run.py
			https://github.com/cloudius-systems/osv/wiki/virtio-fs
		*/
		// Create temporary file for virtio-fs socket
		socketPath := filepath.Join(volumesDir, fmt.Sprintf("%s.sock", volume.ID))
		// Determine arguments for virtio-fs
		extras.vmProc = [][]string{{
			"virtiofsd",
			"--socket-path", socketPath,
			"--shared-dir", volume.HostPath.Path,
			"--no-announce-submounts",
		}}
		extras.vmArgs = []string{
			"-chardev", fmt.Sprintf("socket,id=char%d,path=%s", volumeMountIndex, socketPath),
			"-device", fmt.Sprintf("vhost-user-fs-pci,queue-size=1024,chardev=char%d,tag=%s", volumeMountIndex, volume.Name),
		}
		extras.vmOpts = []string{
			fmt.Sprintf("--mount-fs=virtiofs,/dev/virtiofs%d,%s", volumeMountIndex, volumeMount.MountPath),
		}
	// TODO: EmptyDir *corev1.EmptyDirVolumeSource
	// TODO: GCEPersistentDisk *corev1.GCEPersistentDiskVolumeSource
	// TODO: AWSElasticBlockStore *corev1.AWSElasticBlockStoreVolumeSource
	// TODO: GitRepo *corev1.GitRepoVolumeSource
	case volume.Secret != nil: // TODO (ignored)
	case volume.NFS != nil: // TODO (ignored)
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
	case volume.ConfigMap != nil:
		// Create mountable directory for the configmaps
		// It is not possible to create a subdirectory when mounting in OSv, so mountPath is mapped here
		configmapsPath := filepath.Join(volumesDir, fmt.Sprintf("%s_configmaps", instance.ID), volumeMount.MountPath)
		if err := os.MkdirAll(configmapsPath, 0755); err != nil {
			return nil, err
		}
		// TODO (ignored)
		//for k, v := range volume.ConfigMap.Items {
		//}
		// Create the configmap files in the directory
		for path, data := range volume.ConfigMap.Object.Data {
			if err := os.WriteFile(filepath.Join(configmapsPath, path), []byte(data), 0644); err != nil {
				return nil, err
			}
		}
		/*
			Create options for virtio-fs socket according to scripts/run.py
			https://raw.githubusercontent.com/cloudius-systems/osv/master/scripts/run.py
			https://github.com/cloudius-systems/osv/wiki/virtio-fs
		*/
		// Create temporary file for virtio-fs socket
		socketPath := filepath.Join(volumesDir, fmt.Sprintf("%s.sock", volume.ID))
		// Determine arguments for virtio-fs
		extras.vmProc = [][]string{{
			"virtiofsd",
			"--socket-path", socketPath,
			"--shared-dir", configmapsPath,
			"--no-announce-submounts",
		}}
		extras.vmArgs = []string{
			"-chardev", fmt.Sprintf("socket,id=char0,path=%s", socketPath),
			"-device", fmt.Sprintf("vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=%s", volume.Name),
		}
		// It is important for OSv in order not to have a slash at the end of a mountPath, otherwise it will not work
		extras.vmOpts = []string{
			fmt.Sprintf("--mount-fs=virtiofs,/dev/virtiofs%d,/run/kubernetes/configmaps", volumeMountIndex),
		}
	// TODO: VsphereVolume *VsphereVirtualDiskVolumeSource
	// TODO: Quobyte *QuobyteVolumeSource
	// TODO: AzureDisk *AzureDiskVolumeSource
	// TODO: PhotonPersistentDisk *PhotonPersistentDiskVolumeSource
	case volume.Projected != nil:
		// TODO: Solve this with another virtio-fs mount?
		for _, source := range volume.Projected.Sources {
			switch {
			case source.Secret != nil: // TODO (ignored)
				// TODO
			case source.DownwardAPI != nil: // TODO (ignored)
				// TODO
			case source.ConfigMap != nil: // TODO (ignored)
				// TODO
			case source.ServiceAccountToken != nil: // TODO (ignored)
				// TODO
			}
		}
	// TODO: PortworxVolume *PortworxVolumeSource
	// TODO: ScaleIO *ScaleIOVolumeSource
	// TODO: StorageOS *StorageOSVolumeSource
	// TODO: CSI *CSIVolumeSource
	// TODO: Ephemeral *EphemeralVolumeSource
	default:
		return nil, errors.Errorf("volumeMount %q has an unsupported type: %+v", volumeMount.Name, volumeMount)
	}
	return extras, nil
}

// Ensure interface is implemented
var _ Backend = (*OSvBackend)(nil)

func (b *OSvBackend) imageDir(imageRef string) string {
	cleaned := storage.CleanName(imageRef)
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	return filepath.Join(b.repo.RepoPath(), "fledge", cleaned)
}

func (b *OSvBackend) imageInfoPath(imageRef string) string {
	return filepath.Join(b.imageDir(imageRef), "index.yaml")
}

func (b *OSvBackend) imageConfPath(imageRef string) string {
	return filepath.Join(b.imageDir(imageRef), "config.yaml")
}

func (b *OSvBackend) imageDiskPath(imageRef string, hypervisor string) string {
	imageDir := b.imageDir(imageRef)
	imageBasePath := filepath.Join(imageDir, filepath.Base(imageDir))
	return fmt.Sprintf("%s.%s", imageBasePath, hypervisor)
}

func (b *OSvBackend) instanceDir(instance *Instance) string {
	return filepath.Join(capstan.ConfigDir(), "instances/qemu", instance.ID)
}

func (b *OSvBackend) instanceConfPath(instance *Instance) string {
	return filepath.Join(b.instanceDir(instance), "osv.config")
}

func (b *OSvBackend) instanceMoniPath(instance *Instance) string {
	return filepath.Join(b.instanceDir(instance), "osv.monitor")
}

func (b *OSvBackend) instanceSockPath(instance *Instance) string {
	return filepath.Join(b.instanceDir(instance), "osv.socket")
}

func (b *OSvBackend) instanceLogsPath(instance *Instance) string {
	return filepath.Join(b.instanceDir(instance), "osv.logs")
}

func (b *OSvBackend) volumesDir() string {
	return filepath.Join(capstan.ConfigDir(), "volumes/qemu")
}

func (b *OSvBackend) volumePath(volumeID string) string {
	volumesDir := b.volumesDir()
	return filepath.Join(volumesDir, volumeID)
}

// pullInstanceImage pulls the image into a local capstan repository
func (b *OSvBackend) pullInstanceImage(imageRef string, hypervisor string) error {
	imageDir := b.imageDir(imageRef)
	// Remove image if it exists
	if _, err := os.Stat(imageDir); !os.IsNotExist(err) {
		if err = os.RemoveAll(imageDir); err != nil {
			return err
		}
	}
	// Create repository directory
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return err
	}
	// Parse image reference
	r, err := ref.New(imageRef)
	if err != nil {
		return err
	}
	if r.Registry == "" {
		return fmt.Errorf("reference %s does not contain a valid registry", r.CommonName())
	}
	// Write image info to index.yaml
	imageInfo := capstan.ImageInfo{
		FormatVersion: "1",
		Version:       r.Tag,
		Created:       time.Now().Format(core.FRIENDLY_TIME_F),
		Description:   "OSv image imported by FLEDGE",
		Build:         "",
	}
	imageInfoFile, err := os.Create(b.imageInfoPath(imageRef))
	if err != nil {
		return err
	}
	imageInfoBytes, err := json.Marshal(imageInfo)
	if err != nil {
		return err
	}
	if _, err = imageInfoFile.Write(imageInfoBytes); err != nil {
		return err
	}
	//// Retrieve the image config
	rc := regclient.New(regclient.WithDockerCreds())
	//imageConf, err := storage.ImageGetConfigWithClient(rc, b.context, r)
	//if err != nil {
	//	return err
	//}
	//// Write image config to config.yaml
	//imageConfFile, err := os.Create(b.imageConfPath(imageRef))
	//if err != nil {
	//	return err
	//}
	//imageConfBytes, err := json.Marshal(imageConf)
	//if err != nil {
	//	return err
	//}
	//if _, err = imageConfFile.Write(imageConfBytes); err != nil {
	//	return err
	//}
	// Retrieve the image layers (should be one)
	layerDescs, err := storage.ImageGetLayersWithClient(rc, b.context, r)
	if err != nil {
		return err
	}
	if len(layerDescs) != 1 {
		return fmt.Errorf("image %s does not contain just one layer", r.CommonName())
	}
	// Check if the layer has the correct type
	layerDesc := layerDescs[0]
	if layerDesc.MediaType != types.MediaTypeOCI1Layer && layerDesc.MediaType != types.MediaTypeOCI1LayerGzip {
		return fmt.Errorf("layer media type %s is not supported", layerDesc.MediaType)
	}
	// Pull the stream
	layerBlob, err := rc.BlobGet(b.context, r, layerDesc)
	if err != nil {
		return err
	}
	layerTarReader, err := layerBlob.ToTarReader()
	if err != nil {
		return err
	}
	tr, err := layerTarReader.GetTarReader()
	if err != nil {
		return err
	}
	hdr, err := tr.Next()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(hdr.Name, ".qemu") {
		return fmt.Errorf("unexpected file %s in layer of image %s", hdr.Name, r.CommonName())
	}
	// Write image layer to <base>.<hypervisor>
	imageDiskFile, err := os.Create(b.imageDiskPath(imageRef, hypervisor))
	if err != nil {
		return err
	}
	if _, err = io.Copy(imageDiskFile, tr); err != nil {
		return err
	}
	return nil
}
