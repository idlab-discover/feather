package provider

import (
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"io"
	corev1 "k8s.io/api/core/v1"
	"syscall"
)

const (
	BackendContainerd string = "containerd"
	BackendOsv               = "osv"
)

type BackendOld interface {
	GetContainerName(namespace string, pod corev1.Pod, dc corev1.Container) string
	GetContainerNameAlt(namespace string, podName string, dcName string) string
	CreatePod(pod *corev1.Pod) error
	CreateContainer(namespace string, pod *corev1.Pod, dc *corev1.Container) (string, error)
	UpdatePod(pod *corev1.Pod) error
	DeletePod(pod *corev1.Pod) error
	GetPod(namespace string, name string) (*corev1.Pod, error)
	GetPods() ([]*corev1.Pod, error)
	GetContainerLogs(namespace string, podName string, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error)
	ShutdownPods()
	PodsChanged() bool
	ResetFlags()
}

type Backend interface {
	GetInstanceStatus(instance *Instance) (corev1.ContainerStatus, error)
	CreateInstance(instance *Instance) error
	StartInstance(instance *Instance) error
	UpdateInstance(instance *Instance) error
	KillInstance(instance *Instance, signal syscall.Signal) error
	DeleteInstance(instance *Instance) error
	GetInstanceLogs(instance *Instance, opts api.ContainerLogOpts) (io.ReadCloser, error)
	RunInInstance(instance *Instance, cmd []string, attach api.AttachIO) error
}
