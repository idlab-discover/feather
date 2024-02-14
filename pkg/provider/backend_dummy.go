package provider

import (
	"io"
	"strings"
	"syscall"

	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	corev1 "k8s.io/api/core/v1"
)

type DummyBackend struct {
	config Config
}

func NewDummyBackend(cfg Config) (*DummyBackend, error) {
	b := &DummyBackend{config: cfg}
	return b, nil
}

func (b *DummyBackend) GetInstanceStatus(instance *Instance) (corev1.ContainerStatus, error) {
	dummyStatus := corev1.ContainerStatus{
		Name: "dummy",
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode:    0,
				Message:     "This container is run by a dummy backend which does absolutely nothing.",
				ContainerID: instance.ID,
			},
		},
	}
	return dummyStatus, nil
}

func (b *DummyBackend) CreateInstance(instance *Instance) error {
	return nil
}

func (b *DummyBackend) StartInstance(instance *Instance) error {
	return nil
}

func (b *DummyBackend) UpdateInstance(instance *Instance) error {
	return nil
}

func (b *DummyBackend) KillInstance(instance *Instance, signal syscall.Signal) error {
	return nil
}

func (b *DummyBackend) DeleteInstance(instance *Instance) error {
	return nil
}

func (b *DummyBackend) GetInstanceLogs(instance *Instance, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (b *DummyBackend) RunInInstance(instance *Instance, cmd []string, attach api.AttachIO) error {
	return nil
}

func (b *DummyBackend) CreateVolume(volumeID string, volume corev1.Volume) error {
	return nil
}

func (b *DummyBackend) UpdateVolume(volumeID string, volume corev1.Volume) error {
	return nil
}

func (b *DummyBackend) DeleteVolume(volumeID string) error {
	return nil
}

// Ensure interface is implemented
var _ Backend = (*DummyBackend)(nil)
