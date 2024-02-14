package provider

import (
	"context"
	"github.com/containerd/containerd/reference/docker"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"gitlab.ilabt.imec.be/fledge/service/pkg/storage"
	"io"
	corev1 "k8s.io/api/core/v1"
	"syscall"
)

// An Instance represents a Container with extensions for a Backend
// It's desirable that the backend only knows as much as it needs to set up a Container
type Instance struct {
	ID      string
	Backend Backend
	*corev1.Container
	VolumeMounts []InstanceVolumeMount
	HostNetwork  bool
}

// newInstance extracts the information it needs from the Pod and lets all the rest be handled by the Backend
// This is a heavy function right now, but it lets us deal with the important stuff in the backend so that we
// can let the provider handle everything else
func (p *Provider) newInstance(ctx context.Context, pod *corev1.Pod, container *corev1.Container) (*Instance, error) {
	// Check for name collision just in case
	instanceID := podAndContainerToIdentifier(pod, container)
	if _, ok := p.instances[instanceID]; ok {
		return nil, errors.Errorf("name collision for instance %q", instanceID)
	}

	// Convert image name to something universal
	imageRef, err := docker.ParseDockerRef(container.Image)
	if err != nil {
		return nil, errors.Errorf("failed parsing reference %q", container.Image)
	}
	container.Image = imageRef.String()

	// Get the config of the image to determine the backend
	im, err := storage.ImageGetConfig(ctx, container.Image)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get image config of %q", container.Image)
	}
	// If no backend is defined, it's a non-FLEDGE and should be run with containerd
	if im.Backend == "" {
		im.Backend = BackendContainerd
	}
	// Check if the required backend is enabled
	backend := p.backends[im.Backend]
	if backend == nil {
		return nil, errors.Wrapf(err, "failed to find enabled backend %q", im.Backend)
	}

	// Make a lookup for Volumes
	// TODO: This is pretty slow to do this every instance, can we clean this up?
	volumesByName := map[string]corev1.Volume{}
	for _, v := range pod.Spec.Volumes {
		volumesByName[v.Name] = v
	}
	// Map the Volumes and VolumeMounts to InstanceVolumes
	var volumeMounts []InstanceVolumeMount
	for _, vm := range container.VolumeMounts {
		v, ok := volumesByName[vm.Name]
		if !ok {
			err = errors.Errorf("failed to find volume %q in spec of pod %q", vm.Name, pod.Name)
			return nil, err
		}
		volume, err := p.newInstanceVolume(pod, v)
		if err != nil {
			return nil, err
		}
		volumeMount := InstanceVolumeMount{
			VolumeMount: vm,
			Volume:      volume,
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}

	// Make Instance
	return &Instance{
		ID:           instanceID,
		Backend:      backend,
		Container:    container,
		VolumeMounts: volumeMounts,
		HostNetwork:  pod.Spec.HostNetwork,
	}, nil
}

func (i *Instance) Status() (corev1.ContainerStatus, error) {
	return i.Backend.GetInstanceStatus(i)
}

func (i *Instance) Create() error {
	return i.Backend.CreateInstance(i)
}

func (i *Instance) Start() error {
	return i.Backend.StartInstance(i)
}

func (i *Instance) Kill(signal syscall.Signal) error {
	return i.Backend.KillInstance(i, signal)
}

func (i *Instance) Update() error {
	return i.Backend.UpdateInstance(i)
}

func (i *Instance) Delete() error {
	return i.Backend.DeleteInstance(i)
}

func (i *Instance) Logs(opts api.ContainerLogOpts) (io.ReadCloser, error) {
	return i.Backend.GetInstanceLogs(i, opts)
}

func (i *Instance) Run(cmd []string, attach api.AttachIO) error {
	return i.Backend.RunInInstance(i, cmd, attach)
}
