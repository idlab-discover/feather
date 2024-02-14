package provider

import (
	"context"
	"github.com/containerd/containerd/log"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"io"
)

// GetContainerLogs retrieves the logs of a container by name from the provider.
func (p *Provider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "GetContainerLogs")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)

	log.G(ctx).Debugf("receive GetContainerLogs %q", podName)

	// Get logs from instance
	instance, found := p.getInstance(namespace, podName, containerName)
	if !found {
		return nil, errors.Errorf("failed to find instance (namespace=%s, podName=%s, containerName=%s)", namespace, podName, containerName)
	}
	return instance.Logs(opts)
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *Provider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	ctx, span := trace.StartSpan(ctx, "RunInContainer")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)

	log.G(ctx).Debugf("receive RunInContainer %q", podName)

	// Run command inside instance
	instance, found := p.getInstance(namespace, podName, containerName)
	if !found {
		return errors.Errorf("failed to find instance (namespace=%s, podName=%s, containerName=%s)", namespace, podName, containerName)
	}
	return instance.Run(cmd, attach)
}

// AttachToContainer attaches to the executing process of a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *Provider) AttachToContainer(ctx context.Context, namespace, name, container string, attach api.AttachIO) error {
	ctx, span := trace.StartSpan(ctx, "RunInContainer")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name, containerNameKey, container)

	log.G(ctx).Debugf("receive AttachToContainer %q", container)
	return nil
}
