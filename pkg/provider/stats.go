package provider

import (
	"context"
	"errors"
	"github.com/containerd/containerd/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

// GetStatsSummary gets the stats for the node, including running pods
func (p *Provider) GetStatsSummary(ctx context.Context) (*statsv1alpha1.Summary, error) {
	ctx, span := trace.StartSpan(ctx, "GetStatsSummary")
	defer span.End()

	log.G(ctx).Info("receive GetStatsSummary")

	// TODO Implement

	return nil, errors.New("GetStatsSummary not implemented")
}
