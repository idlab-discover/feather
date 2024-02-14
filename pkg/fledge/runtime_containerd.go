package fledge

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/containerd/containerd/reference/docker"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/mount"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodContainer struct {
	podName   string
	container containerd.Container
	task      containerd.Task
}

type ContainerdRuntime struct {
	config BrokerConfig

	client                   *containerd.Client
	containerNameTaskMapping map[string]PodContainer
	podSpecs                 map[string]*corev1.Pod
	ctx                      context.Context
	podsChanged              bool
}

func NewContainerdRuntime(cfg BrokerConfig) (*ContainerdRuntime, error) {
	client, err := containerd.New("/run/containerd/containerd.sock")
	if client == nil {
		return nil, err
	}

	cr := &ContainerdRuntime{
		config:                   cfg,
		client:                   client,
		containerNameTaskMapping: make(map[string]PodContainer),
		podSpecs:                 make(map[string]*corev1.Pod),
		ctx:                      namespaces.WithNamespace(context.Background(), "default"),
		podsChanged:              false,
	}

	mount.SetTempMountLocation("/ctdtmp")

	go func() { cr.PollLoop() }()

	return cr, nil
}

func (cr *ContainerdRuntime) PodsChanged() bool {
	return cr.podsChanged
}

func (cr *ContainerdRuntime) ResetFlags() {
	fmt.Println("Setting podsChanged false")
	cr.podsChanged = false
}

func (cr *ContainerdRuntime) PollLoop() {
	for {
		for _, pod := range cr.podSpecs {
			cr.UpdatePodStatus(pod.ObjectMeta.Namespace, pod)
		}

		time.Sleep(3000 * time.Millisecond)
	}
}

func (cr *ContainerdRuntime) GetPod(namespace string, name string) (*corev1.Pod, error) {
	pod, found := cr.podSpecs[namespace+"_"+name]
	if !found {
		return nil, errors.New("Pod not found")
	}
	return pod, nil
}

func (cr *ContainerdRuntime) GetPods() ([]*corev1.Pod, error) {
	var pods []*corev1.Pod
	for _, pod := range cr.podSpecs {
		pods = append(pods, pod)
	}
	return pods, nil
}

func (cr *ContainerdRuntime) GetContainerName(namespace string, pod corev1.Pod, dc corev1.Container) string {
	return namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
}

func (cr *ContainerdRuntime) GetContainerNameAlt(namespace string, podName string, dcName string) string {
	return namespace + "_" + podName + "_" + dcName
}

func (cr *ContainerdRuntime) CreatePod(pod *corev1.Pod) error {
	namespace := pod.ObjectMeta.Namespace

	cr.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	/* TODO: if config.Cfg.IgnoreKubeProxy == "true" && strings.HasPrefix(pod.ObjectMeta.Name, "kube-proxy") {
		IgnoreKubeProxy(pod)
		return
	}*/

	CreateVolumes(cr.ctx, pod)

	initContainers := false
	var containers []corev1.Container
	if len(pod.Spec.InitContainers) > 0 {
		initContainers = true
		containers = pod.Spec.InitContainers
	} else {
		containers = pod.Spec.Containers
	}
	for _, cont := range containers {
		_, err := cr.CreateContainer(namespace, pod, &cont)
		if err != nil {
			delete(cr.podSpecs, namespace+"_"+pod.ObjectMeta.Name)
			return err
		}
	}

	UpdatePostCreationPodStatus(pod, initContainers)
	fmt.Println("Setting podsChanged true")
	cr.podsChanged = true

	return nil
}

func ValidPrefix(tagPrefix string) bool {
	switch tagPrefix {
	case "docker.io":
		fallthrough
	case "k8s.gcr.io":
		return true
	default:
		return false
	}
	return false
}

func (cr *ContainerdRuntime) SetupPorts(pod *corev1.Pod, dc *corev1.Container) {
	//TODO!
}

func (cr *ContainerdRuntime) CleanupPorts(pod *corev1.Pod, dc *corev1.Container) {

}

func (cr *ContainerdRuntime) CreateContainer(namespace string, pod *corev1.Pod, dc *corev1.Container) (string, error) {
	fullName := cr.GetContainerName(namespace, *pod, *dc)
	envVars := GetEnvAsStringArray(dc)

	// Parse image name
	imageRef, err := docker.ParseDockerRef(dc.Image)
	if err != nil {
		return "", err
	}
	imageString := imageRef.String()

	//restart policy won't be set here, instead tasks have to be monitored for their status
	//and restarted if failed (and pod/node is not shutting down). TODO

	//determine ipc and pid mode
	var ipcMode *oci.SpecOpts = nil
	if pod.Spec.HostIPC {
		opt := oci.WithHostNamespace(specs.IPCNamespace)
		ipcMode = &opt
	}

	var pidMode *oci.SpecOpts = nil
	if pod.Spec.HostPID {
		opt := oci.WithHostNamespace(specs.PIDNamespace)
		pidMode = &opt
	}

	//handle privileged containers
	privileged := false
	if dc.SecurityContext != nil && dc.SecurityContext.Privileged != nil {
		privileged = bool(*dc.SecurityContext.Privileged)
	}

	//handle volume mounts
	vmounts := cr.BuildMounts(pod, dc)

	//handle resource limits
	cgroup := cr.SetContainerResources(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, dc)

	//pull image + policy
	image, err := cr.client.GetImage(cr.ctx, imageString)
	if (err != nil && dc.ImagePullPolicy == corev1.PullIfNotPresent) || dc.ImagePullPolicy == corev1.PullAlways {
		if dc.ImagePullPolicy == corev1.PullAlways {
			cr.client.ImageService().Delete(cr.ctx, imageString)
		}
		image, err = cr.client.Pull(cr.ctx, imageString, containerd.WithPullUnpack)
		if err != nil {
			fmt.Printf("Pull failed for image %s\n", imageString)
			fmt.Println(err.Error())
			return "", err
		}
	}

	fmt.Printf("Image exists or successfully pulled: %s\n", image.Name())

	//generate container id + snapshot
	snapshot := fmt.Sprintf("%s-snapshot", fullName)

	args := dc.Command
	args = append(args, dc.Args...)

	//tally all spec options
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithEnv(envVars),
		oci.WithCgroup(cgroup),
		oci.WithMounts(vmounts),
		//netSpecOpts,
	}

	/* TODO:
	//find out if a gpu should be assigned, and if we have the right type for the container
	assignGpu, err := CheckGpuResourceRequired(dc)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	if assignGpu {
		//to be fair, it's not guaranteed to be nvidia, should really check for AMD or other devices instead of just cuda/opencv
		//but then again, it's not like they have a container hook, so we should really just detect and advertise nvidia?
		specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(1), nvidia.WithAllCapabilities))
	}
	*/

	if len(args) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(args...))
	}
	if dc.WorkingDir != "" {
		specOpts = append(specOpts, oci.WithProcessCwd(dc.WorkingDir))
	}
	if ipcMode != nil {
		specOpts = append(specOpts, *ipcMode)
	}
	if pidMode != nil {
		specOpts = append(specOpts, *pidMode)
	}
	//NET: trying something out
	if pod.Spec.HostNetwork {
		specOpts = append(specOpts, oci.WithHostNamespace(specs.NetworkNamespace))
	}
	//assign all caps
	if privileged {
		specOpts = append(specOpts, oci.WithPrivileged)
	}

	// fmt.Printf("Creating container with snapshotter native\n")
	container, err := cr.client.NewContainer(
		cr.ctx,
		fullName,
		//containerd.WithSnapshotter("native"),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(snapshot, image),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	// fmt.Printf("Successfully created container with ID %s and snapshot with ID %s\n", container.ID(), snapshot)

	// create a task from the container
	task, err := container.NewTask(cr.ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	fmt.Println("Task created")
	//defer task.Delete(ctx)

	// make sure we wait before calling start
	exitStatusC, err := task.Wait(cr.ctx)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	fmt.Println("Task awaited with status %d", exitStatusC)

	// call start on the task to execute the redis server
	if err := task.Start(cr.ctx); err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	fmt.Println("Task started")

	//NET: trying to fix the net namespace here
	cr.SetupPodIPs(pod, task)

	cr.containerNameTaskMapping[fullName] = PodContainer{
		podName:   pod.ObjectMeta.Name,
		container: container,
		task:      task,
	}

	return task.ID(), nil
}

/* TODO:
func CheckGpuResourceRequired(dc *corev1.Container) (bool, error) {
	if dc.Resources.Limits == nil {
		dc.Resources.Limits = corev1.ResourceList{}
	}
	if dc.Resources.Requests == nil {
		dc.Resources.Requests = corev1.ResourceList{}
	}

	cudaLimit := dc.Resources.Limits["device/cudagpu"]
	cudaRequest := dc.Resources.Requests["device/cudagpu"]
	opencvLimit := dc.Resources.Limits["device/opencvgpu"]
	opencvRequest := dc.Resources.Requests["device/opencvgpu"]

	if opencvLimit.Value() > 0 || opencvRequest.Value() > 0 {
		if !manager.HasOpenCLCaps() {
			return false, errors.New("OpenCV requested but not present")
		} else {
			return true, nil
		}
	}
	if cudaLimit.Value() > 0 || cudaRequest.Value() > 0 {
		if !manager.HasCudaCaps() {
			return false, errors.New("CUDA requested but not present")
		} else {
			return true, nil
		}
	}
	return false, nil
}
*/

func (cr *ContainerdRuntime) SetupPodIPs(pod *corev1.Pod, task containerd.Task) {
	// TODO: pod.Status.HostIP = config.Cfg.DeviceIP
	if pod.Status.PodIP == "" {
		if pod.Spec.HostNetwork {
			// TODO: pod.Status.PodIP = config.Cfg.DeviceIP
		} else {
			pids, _ := task.Pids(cr.ctx)
			pidJson, _ := json.Marshal(pids)
			fmt.Printf("Container pids %s", string(pidJson))
			pod.Status.PodIP = BindNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, int(pids[0].Pid))
		}
	}
}

func (cr *ContainerdRuntime) BuildMounts(pod *corev1.Pod, dc *corev1.Container) []specs.Mount {
	mounts := []specs.Mount{}
	//mountNames := make(map[string]struct{})
	for _, cVol := range dc.VolumeMounts {
		cmount := cr.CreateMount(pod, cVol)
		if cmount != nil {
			mounts = append(mounts, *cmount)
			//mountNames[cVol.Name] = struct{}{}
		}
	}
	return mounts
}

func (cr *ContainerdRuntime) CreateMount(pod *corev1.Pod, volMount corev1.VolumeMount) *specs.Mount {
	//fmt.Printf("Creating mount for volumemount %s\n", volMount.Name)
	vName := volMount.Name
	var volume corev1.Volume

	for _, vol := range pod.Spec.Volumes {
		if vol.Name == vName {
			volume = vol
		}
	}
	//fmt.Printf("Matching volume %s\n", volume.Name)

	//the vkubelet can handle hostpath volumes, secret volumes and configmap volumes
	hostPath := GetHostMountPath(pod, volume)

	if hostPath == nil {
		fmt.Printf("No hostpath found, mount not supported?\n")
		return nil
	}
	//fmt.Printf("Attempting to mount hostpath %s\n", hostPath)

	mntOpts := []string{}

	if volMount.ReadOnly {
		mntOpts = append(mntOpts, "rbind")
		mntOpts = append(mntOpts, "ro")
	} else {
		mntOpts = append(mntOpts, "rbind")
		mntOpts = append(mntOpts, "rw")
	}

	cMount := specs.Mount{
		Source:      *hostPath + volMount.SubPath,
		Destination: volMount.MountPath,
		Options:     mntOpts, //volMount.ReadOnly,
		Type:        "bind",
	}
	// fmt.Printf("Mount source %s target %s propagation %s\n", cMount.Source, cMount.Destination, "whatever")

	return &cMount

}

func (cr *ContainerdRuntime) SetContainerResources(namespace string, podname string, dc *corev1.Container) string {
	//some default values
	oneCpu, _ := resource.ParseQuantity("1")
	defaultMem, _ := resource.ParseQuantity("150Mi")

	// fmt.Printf("Checking cpu limiting support\n")
	supportCheck, _ := util.ExecShellCommand("ls /sys/fs/cgroup/cpu/ | grep -E 'cpu.cfs_[a-z]*_us'")
	cpuSupported := supportCheck != ""
	// fmt.Printf("Cpu limit support %s\n", cpuSupported)

	var cpuLimit float64
	var memLimit int64

	if dc.Resources.Limits == nil {
		dc.Resources.Limits = corev1.ResourceList{}
	}
	if dc.Resources.Requests == nil {
		dc.Resources.Requests = corev1.ResourceList{}
	}
	memory := dc.Resources.Limits.Memory()
	if memory.IsZero() {
		//if the memory limit isn't filled in, set it to request
		memory = dc.Resources.Requests.Memory()
		dc.Resources.Limits[corev1.ResourceMemory] = *memory
	}
	if !memory.IsZero() {
		memLimit = memory.Value()
	} else {
		//this means neither was set, so update both limit and request with a default value
		dc.Resources.Limits[corev1.ResourceMemory] = defaultMem
		dc.Resources.Requests[corev1.ResourceMemory] = defaultMem
		memLimit = 150 * 1024 * 1024 //150 Mi
	}

	if cpuSupported {
		cpu := dc.Resources.Limits.Cpu()
		if cpu.IsZero() {
			//same if cpu limit isn't filled in
			cpu = dc.Resources.Requests.Cpu()
			dc.Resources.Limits[corev1.ResourceCPU] = *cpu
		}
		if !cpu.IsZero() {
			cpuLimit = float64(cpu.MilliValue()) / 1000.0
		} else {
			dc.Resources.Limits[corev1.ResourceCPU] = oneCpu
			dc.Resources.Requests[corev1.ResourceCPU] = oneCpu
			cpuLimit = 1 // 1 CPU
		}
	}

	cgroup := CreateCgroupIfNotExists(namespace, podname, dc.Name)
	SetMemoryLimit(cgroup, memLimit)
	SetCpuLimit(cgroup, cpuLimit)
	return cgroup
}

func (cr *ContainerdRuntime) UpdatePod(pod *corev1.Pod) error {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	cr.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	for _, cont := range containers {
		cr.UpdateContainer(namespace, pod, &cont)
	}
	fmt.Println("Setting podsChanged true")
	cr.podsChanged = true

	return nil
}

func (cr *ContainerdRuntime) UpdateContainer(namespace string, pod *corev1.Pod, dc *corev1.Container) {
	cr.StopContainer(namespace, pod, dc)
	cr.CreateContainer(namespace, pod, dc)
}

func (cr *ContainerdRuntime) DeletePod(pod *corev1.Pod) error {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	delete(cr.podSpecs, namespace+"_"+pod.ObjectMeta.Name)

	for _, cont := range containers {
		cr.StopContainer(namespace, pod, &cont)
	}
	RemoveNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

	FreeIP(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
	fmt.Println("Setting podsChanged true")
	cr.podsChanged = true

	return nil
}

func (cr *ContainerdRuntime) StopContainer(namespace string, pod *corev1.Pod, dc *corev1.Container) bool {
	fullName := cr.GetContainerName(namespace, *pod, *dc) //namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
	fmt.Printf("Stopping container %s\n", fullName)

	tuple, found := cr.containerNameTaskMapping[fullName]
	if found {
		fmt.Printf("Stopping and removing task id %s\n", tuple.task.ID())
		//time, _ := time.ParseDuration("10s")
		//err := cr.cli.ContainerStop(cr.ctx, contID, nil)
		exitStatus, err := tuple.task.Delete(cr.ctx)
		fmt.Printf("Task stopped status %d \n", exitStatus)

		if err == nil {
			delete(cr.containerNameTaskMapping, fullName)
			fmt.Printf("Removing container %s\n", fullName)

			err = tuple.container.Delete(cr.ctx, containerd.WithSnapshotCleanup)
			DeleteCgroup(GetCgroup(namespace, pod.ObjectMeta.Name, dc.Name))
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return err == nil
	}
	return true
}

func (cr *ContainerdRuntime) UpdatePodStatus(namespace string, pod *corev1.Pod) {
	// TODO: pod.Status.HostIP = config.Cfg.DeviceIP
	fmt.Printf("Update pod status %s\n", pod.ObjectMeta.Name)
	latestStatus := GetHighestPodStatus(pod)
	if latestStatus == nil {
		fmt.Println("No correct status found, returning")
		return
	}
	fmt.Printf("Pod %s status %s\n", pod.ObjectMeta.Name, latestStatus.Type)
	switch latestStatus.Type {
	case corev1.PodReady:
		fmt.Println("Pod ready, just updating")
		//everything good, just update statuses
		//check pod status phases running or succeeded
		cr.UpdateContainerStatuses(namespace, pod, *latestStatus)
		//cr.SetupPodIPs(pod)
	case corev1.PodInitialized:
		if latestStatus.Status == corev1.ConditionTrue {
			fmt.Println("Pod initialized, just updating and upgrading to ready if possible")
			//update statuses, check for PodReady, check for phase running
			cr.UpdateContainerStatuses(namespace, pod, *latestStatus)
		} else {
			fmt.Println("Pod initialized, checking init containers")
			//check init containers
			cr.CheckInitContainers(namespace, pod)
		}
	case corev1.PodReasonUnschedulable:
		fmt.Println("Pod unschedulable, ignoring")
		//don't do anything really
	case corev1.PodScheduled:
		fmt.Println("Pod Scheduled, ignoring")
		//don't do anything either
	}
}

func (cr *ContainerdRuntime) CheckInitContainers(namespace string, pod *corev1.Pod) {
	//ctx := namespaces.WithNamespace(context.Background(), namespace)
	allContainersDone := true
	noErrors := true

	containerStatuses := []corev1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		fullName := cr.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		tuple, found := cr.containerNameTaskMapping[fullName]
		if found {
			state := corev1.ContainerState{}

			taskStatus, _ := tuple.task.Status(cr.ctx)
			switch taskStatus.Status { //contJSON.State.Status {
			case containerd.Created: //"created":
				allContainersDone = false
				state.Waiting = &corev1.ContainerStateWaiting{
					Reason:  "Starting",
					Message: "Starting container",
				}
			case containerd.Running: //"running":
				fallthrough
			case containerd.Paused: //"paused":
				fallthrough
			case containerd.Pausing: //"restarting":
				allContainersDone = false
				state.Running = &corev1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case containerd.Stopped: //"dead":
				if taskStatus.ExitStatus > 0 {
					noErrors = false
				}
				state.Terminated = &corev1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: tuple.container.ID(),
				}
			}

			status := corev1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  tuple.container.ID(),
			}

			containerStatuses = []corev1.ContainerStatus{status}
		}
	}
	pod.Status.ContainerStatuses = containerStatuses

	if noErrors && allContainersDone {
		//start actual containers
		for _, container := range pod.Spec.Containers {
			(*cr).CreateContainer(pod.ObjectMeta.Namespace, pod, &container)
		}
	}
	UpdateInitPodStatus(pod, noErrors, allContainersDone)
}

func (cr *ContainerdRuntime) UpdateContainerStatuses(namespace string, pod *corev1.Pod, podStatus corev1.PodCondition) {
	//ctx := namespaces.WithNamespace(context.Background(), namespace)
	allContainersRunning := true
	allContainersDone := true
	noErrors := true

	var containerStatuses []corev1.ContainerStatus
	for _, cont := range pod.Spec.Containers {
		fullName := cr.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		tuple, found := cr.containerNameTaskMapping[fullName]
		if found {
			state := corev1.ContainerState{}

			taskStatus, _ := tuple.task.Status(cr.ctx)
			switch taskStatus.Status { //contJSON.State.Status {
			case "created":
				allContainersRunning = false
				allContainersDone = false
				state.Waiting = &corev1.ContainerStateWaiting{
					Reason:  "Starting",
					Message: "Starting container",
				}
			case "running":
				fallthrough
			case "paused":
				fallthrough
			case "restarting":
				allContainersDone = false
				state.Running = &corev1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case "removing":
				fallthrough
			case "exited":
				fallthrough
			case "dead":
				if taskStatus.ExitStatus > 0 { //contJSON.State.ExitCode > 0 {
					noErrors = false
				}
				state.Terminated = &corev1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: tuple.container.ID(),
				}
			}

			status := corev1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  tuple.container.ID(),
			}

			containerStatuses = []corev1.ContainerStatus{status}
		}
	}
	changed := UpdatePodStatus(podStatus, containerStatuses, pod, noErrors, allContainersRunning, allContainersDone)
	if changed {
		fmt.Println("Setting podsChanged true")
		cr.podsChanged = true
	}
}

func (cr *ContainerdRuntime) GetContainerLogs(namespace string, podName string, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	//TODO
	return nil, errors.New("GetContainerLogs not implemented")
}

func (cr *ContainerdRuntime) ShutdownPods() {
	for _, pod := range cr.podSpecs {
		cr.DeletePod(pod)
	}
}

// Ensure interface is implemented
var _ Runtime = (*ContainerdRuntime)(nil)
