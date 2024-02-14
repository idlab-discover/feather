package provider

import (
	"context"
	"gitlab.ilabt.imec.be/fledge/service/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"runtime"
)

func (p *Provider) ConfigureNode(ctx context.Context, n *corev1.Node) { //nolint:golint
	n.Status.Addresses = p.nodeAddresses()
	n.Status.Allocatable = p.nodeCapacity(ctx)
	n.Status.Capacity = p.nodeCapacity(ctx)
	n.Status.Conditions = p.nodeConditions()
	n.Status.DaemonEndpoints = p.nodeDaemonEndpoints()

	// Discover Arch and OS
	n.Status.NodeInfo.Architecture = runtime.GOARCH
	n.Status.NodeInfo.OperatingSystem = runtime.GOOS
	n.ObjectMeta.Labels[corev1.LabelArchStable] = n.Status.NodeInfo.Architecture
	n.ObjectMeta.Labels[corev1.LabelOSStable] = n.Status.NodeInfo.OperatingSystem

	// Prevent load-balancers
	n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	n.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *Provider) nodeAddresses() []corev1.NodeAddress {
	// Discover hostname
	nodeHostName := corev1.NodeAddress{
		Type:    corev1.NodeHostName,
		Address: system.HostName(),
	}
	// Discover internal IP
	nodeInternalIP := corev1.NodeAddress{
		Type:    corev1.NodeInternalIP,
		Address: p.internalIP,
	}
	return []corev1.NodeAddress{nodeHostName, nodeInternalIP}
}

// Capacity returns a resource list containing the nodeCapacity limits.
func (p *Provider) nodeCapacity(ctx context.Context) corev1.ResourceList {
	// Get pods to parse
	pods, _ := p.GetPods(ctx)

	// Get available node
	node := corev1.ResourceList{}
	node[corev1.ResourceCPU], _ = system.CpuCount()
	node[corev1.ResourceMemory], _ = system.MemoryTotal()
	node[corev1.ResourceStorage], _ = system.StorageSize()
	node[corev1.ResourcePods], _ = resource.ParseQuantity("10") // TODO: get from settings?

	// Get resource requests and limits
	limits := corev1.ResourceList{}
	requests := corev1.ResourceList{}
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			for _, name := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceStorage, corev1.ResourceEphemeralStorage} {
				if container.Resources.Limits != nil {
					quantity := container.Resources.Limits.Name(name, resource.DecimalSI)
					limits.Name(name, resource.DecimalSI).Add(*quantity)
				}
				if container.Resources.Requests != nil {
					quantity := container.Resources.Requests.Name(name, resource.DecimalSI)
					requests.Name(name, resource.DecimalSI).Add(*quantity)
				}
			}
		}
	}

	// Merge resources
	resources := node.DeepCopy()
	resources[corev1.ResourceLimitsCPU] = *limits.Cpu()
	resources[corev1.ResourceLimitsMemory] = *limits.Memory()
	resources[corev1.ResourceLimitsEphemeralStorage] = *limits.StorageEphemeral()
	resources[corev1.ResourceRequestsCPU] = *requests.Cpu()
	resources[corev1.ResourceRequestsMemory] = *requests.Memory()
	resources[corev1.ResourceRequestsStorage] = *requests.Storage()
	resources[corev1.ResourceRequestsEphemeralStorage] = *requests.StorageEphemeral()

	return resources
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *Provider) nodeConditions() []corev1.NodeCondition {
	now := metav1.Now()
	// Node is always ready
	readyCondition := corev1.NodeCondition{
		Type:               "Ready",
		Status:             corev1.ConditionTrue,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletReady",
		Message:            "kubelet is ready.",
	}
	// Check for memory conditions
	memoryPressureCondition := corev1.NodeCondition{
		Type:               corev1.NodeMemoryPressure,
		Status:             corev1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasSufficientMemory",
		Message:            "kubelet has sufficient memory available",
	}
	if system.IsMemoryPressure() {
		memoryPressureCondition.Status = corev1.ConditionTrue
	}
	// Check for storage conditions
	diskPressureCondition := corev1.NodeCondition{
		Type:               corev1.NodeDiskPressure,
		Status:             corev1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasNoDiskPressure",
		Message:            "kubelet has no disk pressure",
	}
	if system.IsStoragePressure() {
		diskPressureCondition.Status = corev1.ConditionTrue
	}
	outOfDiskCondition := corev1.NodeCondition{
		Type:               "OutOfDisk",
		Status:             corev1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasSufficientDisk",
		Message:            "kubelet has sufficient disk space available",
	}
	if system.IsStorageFull() {
		outOfDiskCondition.Status = corev1.ConditionTrue
	}
	// Check for network conditions
	networkUnavailableCondition := corev1.NodeCondition{
		Type:               corev1.NodeNetworkUnavailable,
		Status:             corev1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "RouteCreated",
		Message:            "RouteController created a route",
	}
	if !system.IsNetworkAvailable() {
		networkUnavailableCondition.Status = corev1.ConditionTrue
	}

	return []corev1.NodeCondition{
		readyCondition,
		memoryPressureCondition,
		diskPressureCondition,
		outOfDiskCondition,
		networkUnavailableCondition,
	}
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *Provider) nodeDaemonEndpoints() corev1.NodeDaemonEndpoints {
	return corev1.NodeDaemonEndpoints{
		KubeletEndpoint: corev1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}
