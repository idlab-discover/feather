package fledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	dto "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"gitlab.ilabt.imec.be/fledge/service/pkg/system"
	"io"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
	"time"
)

const (
	// Provider configuration defaults.
	defaultCPUCapacity      = "20"
	defaultMemoryCapacity   = "100Gi"
	defaultContainerRuntime = "containerd"
	defaultPodCapacity      = "20"

	// Values used in tracing as attribute keys.
	namespaceKey     = "namespace"
	nameKey          = "name"
	containerNameKey = "containerName"
)

// BrokerProvider implements the virtual-kubelet provider interface and forwards calls to runtimes.
type BrokerProvider struct {
	nodeName           string
	operatingSystem    string
	internalIP         string
	daemonEndpointPort int32
	config             BrokerConfig
	runtime            Runtime
	startTime          time.Time
}

// BrokerConfig contains a broker virtual-kubelet's configurable parameters.
type BrokerConfig struct { //nolint:golint
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Runtime string `json:"runtime,omitempty"`
	Pods    string `json:"pods,omitempty"`
}

// NewBrokerProviderBrokerConfig creates a new BrokerProvider.
func NewBrokerProviderBrokerConfig(cfg BrokerConfig, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32) (*BrokerProvider, error) {
	// set defaults
	if cfg.CPU == "" {
		cfg.CPU = defaultCPUCapacity
	}
	if cfg.Memory == "" {
		cfg.Memory = defaultMemoryCapacity
	}
	if cfg.Runtime == "" {
		cfg.Runtime = defaultContainerRuntime
	}
	if cfg.Pods == "" {
		cfg.Pods = defaultPodCapacity
	}

	// setup container runtime
	var (
		runtime Runtime
		err     error
	)
	switch cfg.Runtime {
	case "containerd":
		if runtime, err = NewContainerdRuntime(cfg); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New(fmt.Sprintf("runtime '%s' is not supported\n", cfg.Runtime))
	}

	// setup provider
	provider := BrokerProvider{
		nodeName:           nodeName,
		operatingSystem:    operatingSystem,
		internalIP:         internalIP,
		daemonEndpointPort: daemonEndpointPort,
		config:             cfg,
		runtime:            runtime,
		startTime:          time.Now(),
	}
	return &provider, nil
}

// NewBrokerProvider creates a new BrokerProvider, which implements the PodNotifier interface
func NewBrokerProvider(providerConfig, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32) (*BrokerProvider, error) {
	cfg, err := loadConfig(providerConfig)
	if err != nil {
		return nil, err
	}

	return NewBrokerProviderBrokerConfig(cfg, nodeName, operatingSystem, internalIP, daemonEndpointPort)
}

// loadConfig loads the given json configuration files.
func loadConfig(providerConfig string) (cfg BrokerConfig, err error) {
	data, err := os.ReadFile(providerConfig)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, err
	}
	if cfg.CPU == "" {
		cfg.CPU = defaultCPUCapacity
	}
	if cfg.Memory == "" {
		cfg.Memory = defaultMemoryCapacity
	}
	if cfg.Runtime == "" {
		cfg.Runtime = defaultContainerRuntime
	}
	if cfg.Pods == "" {
		cfg.Pods = defaultPodCapacity
	}

	if _, err = resource.ParseQuantity(cfg.CPU); err != nil {
		return cfg, fmt.Errorf("Invalid CPU value %v", cfg.CPU)
	}
	if _, err = resource.ParseQuantity(cfg.Memory); err != nil {
		return cfg, fmt.Errorf("Invalid memory value %v", cfg.Memory)
	}
	if _, err = resource.ParseQuantity(cfg.Pods); err != nil {
		return cfg, fmt.Errorf("Invalid pods value %v", cfg.Pods)
	}
	return cfg, nil
}

// CreatePod takes a Kubernetes Pod and deploys it within the provider.
func (p *BrokerProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "CreatePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Debugf("receive CreatePod %q", pod.Name)

	return p.runtime.CreatePod(pod)
}

// UpdatePod takes a Kubernetes Pod and updates it within the provider.
func (p *BrokerProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "UpdatePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Debugf("receive UpdatePod %q", pod.Name)

	return p.runtime.UpdatePod(pod)
}

// DeletePod takes a Kubernetes Pod and deletes it from the provider. Once a pod is deleted, the provider is
// expected to call the NotifyPods callback with a terminal pod status where all the containers are in a terminal
// state, as well as the pod. DeletePod may be called multiple times for the same pod.
func (p *BrokerProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "DeletePod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, pod.Namespace, nameKey, pod.Name)

	log.G(ctx).Debugf("receive DeletePod %q", pod.Name)

	return p.runtime.DeletePod(pod)
}

// GetPod retrieves a pod by name from the provider (can be cached).
// The Pod returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *BrokerProvider) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "GetPod")
	defer span.End()

	// Add the pod's coordinates to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name)

	log.G(ctx).Debugf("receive GetPod %q", name)

	return p.runtime.GetPod(namespace, name)
}

// GetContainerLogs retrieves the logs of a container by name from the provider.
func (p *BrokerProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "GetContainerLogs")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)

	log.G(ctx).Info("receive GetContainerLogs %q", podName)

	return p.runtime.GetContainerLogs(namespace, podName, containerName, opts)
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *BrokerProvider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	ctx, span := trace.StartSpan(ctx, "RunInContainer")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)

	log.G(ctx).Info("receive RunInContainer %q", podName)

	// TODO Implement

	return errors.New("RunInContainer not implemented")
}

// AttachToContainer attaches to the executing process of a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *BrokerProvider) AttachToContainer(ctx context.Context, namespace, name, container string, attach api.AttachIO) error {
	ctx, span := trace.StartSpan(ctx, "RunInContainer")
	defer span.End()

	// Add pod and container attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name, containerNameKey, container)

	log.G(ctx).Debugf("receive AttachToContainer %q", container)
	return nil
}

// GetPodStatus retrieves the status of a pod by name from the provider.
// The PodStatus returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *BrokerProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "GetPodStatus")
	defer span.End()

	// Add namespace and name as attributes to the current span.
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, name)

	log.G(ctx).Debugf("receive GetPodStatus %q", name)

	pod, err := p.runtime.GetPod(namespace, name)
	if err != nil {
		return nil, err
	}
	return &pod.Status, nil
}

// GetPods retrieves a list of all pods running on the provider (can be cached).
// The Pods returned are expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *BrokerProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "GetPods")
	defer span.End()

	log.G(ctx).Info("receive GetPods")

	return p.runtime.GetPods()
}

// GetStatsSummary gets the stats for the node, including running pods
func (p *BrokerProvider) GetStatsSummary(ctx context.Context) (*statsv1alpha1.Summary, error) {
	ctx, span := trace.StartSpan(ctx, "GetStatsSummary")
	defer span.End()

	log.G(ctx).Info("receive GetStatsSummary")

	// TODO Implement

	return nil, errors.New("GetStatsSummary not implemented")
}

func (p *BrokerProvider) generateMockMetrics(metricsMap map[string][]*dto.Metric, resourceType string, label []*dto.LabelPair) map[string][]*dto.Metric {
	var (
		cpuMetricSuffix    = "_cpu_usage_seconds_total"
		memoryMetricSuffix = "_memory_working_set_bytes"
		dummyValue         = float64(100)
	)

	if metricsMap == nil {
		metricsMap = map[string][]*dto.Metric{}
	}

	finalCpuMetricName := resourceType + cpuMetricSuffix
	finalMemoryMetricName := resourceType + memoryMetricSuffix

	newCPUMetric := dto.Metric{
		Label: label,
		Counter: &dto.Counter{
			Value: &dummyValue,
		},
	}
	newMemoryMetric := dto.Metric{
		Label: label,
		Gauge: &dto.Gauge{
			Value: &dummyValue,
		},
	}
	// if metric family exists add to metric array
	if cpuMetrics, ok := metricsMap[finalCpuMetricName]; ok {
		metricsMap[finalCpuMetricName] = append(cpuMetrics, &newCPUMetric)
	} else {
		metricsMap[finalCpuMetricName] = []*dto.Metric{&newCPUMetric}
	}
	if memoryMetrics, ok := metricsMap[finalMemoryMetricName]; ok {
		metricsMap[finalMemoryMetricName] = append(memoryMetrics, &newMemoryMetric)
	} else {
		metricsMap[finalMemoryMetricName] = []*dto.Metric{&newMemoryMetric}
	}

	return metricsMap
}

func (p *BrokerProvider) getMetricType(metricName string) *dto.MetricType {
	var (
		dtoCounterMetricType = dto.MetricType_COUNTER
		dtoGaugeMetricType   = dto.MetricType_GAUGE
		cpuMetricSuffix      = "_cpu_usage_seconds_total"
		memoryMetricSuffix   = "_memory_working_set_bytes"
	)
	if strings.HasSuffix(metricName, cpuMetricSuffix) {
		return &dtoCounterMetricType
	}
	if strings.HasSuffix(metricName, memoryMetricSuffix) {
		return &dtoGaugeMetricType
	}

	return nil
}

func (p *BrokerProvider) GetMetricsResource(ctx context.Context) ([]*dto.MetricFamily, error) {
	var span trace.Span
	ctx, span = trace.StartSpan(ctx, "GetMetricsResource") //nolint: ineffassign,staticcheck
	defer span.End()

	var (
		nodeNameStr      = "NodeName"
		podNameStr       = "PodName"
		containerNameStr = "containerName"
	)
	nodeLabels := []*dto.LabelPair{
		{
			Name:  &nodeNameStr,
			Value: &p.nodeName,
		},
	}

	metricsMap := p.generateMockMetrics(nil, "node", nodeLabels)
	pods, _ := p.runtime.GetPods()
	for _, pod := range pods {
		podLabels := []*dto.LabelPair{
			{
				Name:  &nodeNameStr,
				Value: &p.nodeName,
			},
			{
				Name:  &podNameStr,
				Value: &pod.Name,
			},
		}
		metricsMap = p.generateMockMetrics(metricsMap, "pod", podLabels)
		for _, container := range pod.Spec.Containers {
			containerLabels := []*dto.LabelPair{
				{
					Name:  &nodeNameStr,
					Value: &p.nodeName,
				},
				{
					Name:  &podNameStr,
					Value: &pod.Name,
				},
				{
					Name:  &containerNameStr,
					Value: &container.Name,
				},
			}
			metricsMap = p.generateMockMetrics(metricsMap, "container", containerLabels)
		}
	}

	res := []*dto.MetricFamily{}
	for metricName := range metricsMap {
		tempName := metricName
		tempMetrics := metricsMap[tempName]

		metricFamily := dto.MetricFamily{
			Name:   &tempName,
			Type:   p.getMetricType(tempName),
			Metric: tempMetrics,
		}
		res = append(res, &metricFamily)
	}

	return res, nil
}

// TODO: Implement NodeChanged for performance reasons

// Ensure interface is implemented
var _ nodeutil.Provider = (*BrokerProvider)(nil)

func (p *BrokerProvider) ConfigureNode(ctx context.Context, n *v1.Node) { //nolint:golint
	ctx, span := trace.StartSpan(ctx, "mock.ConfigureNode") //nolint:staticcheck,ineffassign
	defer span.End()

	n.Status.Capacity = p.capacity()
	n.Status.Allocatable = p.capacity()
	n.Status.Conditions = p.nodeConditions()
	n.Status.Addresses = p.nodeAddresses()
	n.Status.DaemonEndpoints = p.nodeDaemonEndpoints()

	// Discover Arch and OS
	n.Status.NodeInfo.Architecture = "amd64"
	operatingSystem := p.operatingSystem
	if operatingSystem == "" {
		operatingSystem = "linux"
	}
	n.Status.NodeInfo.OperatingSystem = p.operatingSystem
	n.ObjectMeta.Labels["kubernetes.io/arch"] = n.Status.NodeInfo.Architecture
	n.ObjectMeta.Labels["kubernetes.io/os"] = n.Status.NodeInfo.OperatingSystem

	// Prevent load-balancers
	n.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	n.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
}

// Capacity returns a resource list containing the capacity limits.
func (p *BrokerProvider) capacity() v1.ResourceList {
	// Get pods to parse
	pods, _ := p.runtime.GetPods()

	// Get available node
	node := v1.ResourceList{}
	node[v1.ResourceCPU], _ = system.CpuCount()
	node[v1.ResourceMemory], _ = system.MemoryTotal()
	node[v1.ResourceStorage], _ = system.StorageSize()
	node[v1.ResourcePods], _ = resource.ParseQuantity("5") // TODO: get from settings?

	// Get resource requests and limits
	limits := v1.ResourceList{}
	requests := v1.ResourceList{}
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			for _, name := range []v1.ResourceName{v1.ResourceCPU, v1.ResourceMemory, v1.ResourceStorage, v1.ResourceEphemeralStorage} {
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
	resources[v1.ResourceLimitsCPU] = *limits.Cpu()
	resources[v1.ResourceLimitsMemory] = *limits.Memory()
	resources[v1.ResourceLimitsEphemeralStorage] = *limits.StorageEphemeral()
	resources[v1.ResourceRequestsCPU] = *requests.Cpu()
	resources[v1.ResourceRequestsMemory] = *requests.Memory()
	resources[v1.ResourceRequestsStorage] = *requests.Storage()
	resources[v1.ResourceRequestsEphemeralStorage] = *requests.StorageEphemeral()

	return resources
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *BrokerProvider) nodeConditions() []v1.NodeCondition {
	now := metav1.Now()
	// Node is always ready
	readyCondition := v1.NodeCondition{
		Type:               "Ready",
		Status:             v1.ConditionTrue,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletReady",
		Message:            "kubelet is ready.",
	}
	// Check for memory conditions
	memoryPressureCondition := v1.NodeCondition{
		Type:               v1.NodeMemoryPressure,
		Status:             v1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasSufficientMemory",
		Message:            "kubelet has sufficient memory available",
	}
	if system.IsMemoryPressure() {
		memoryPressureCondition.Status = v1.ConditionTrue
	}
	// Check for storage conditions
	diskPressureCondition := v1.NodeCondition{
		Type:               v1.NodeDiskPressure,
		Status:             v1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasNoDiskPressure",
		Message:            "kubelet has no disk pressure",
	}
	if system.IsStoragePressure() {
		diskPressureCondition.Status = v1.ConditionTrue
	}
	outOfDiskCondition := v1.NodeCondition{
		Type:               "OutOfDisk",
		Status:             v1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "KubeletHasSufficientDisk",
		Message:            "kubelet has sufficient disk space available",
	}
	if system.IsStorageFull() {
		outOfDiskCondition.Status = v1.ConditionTrue
	}
	// Check for network conditions
	networkUnavailableCondition := v1.NodeCondition{
		Type:               v1.NodeNetworkUnavailable,
		Status:             v1.ConditionFalse,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
		Reason:             "RouteCreated",
		Message:            "RouteController created a route",
	}
	if !system.IsNetworkAvailable() {
		networkUnavailableCondition.Status = v1.ConditionTrue
	}

	return []v1.NodeCondition{
		readyCondition,
		memoryPressureCondition,
		diskPressureCondition,
		outOfDiskCondition,
		networkUnavailableCondition,
	}
}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *BrokerProvider) nodeAddresses() []v1.NodeAddress {
	// Discover hostname
	nodeHostName := v1.NodeAddress{
		Type:    v1.NodeHostName,
		Address: p.nodeName,
	}
	// Discover internal IP
	nodeInternalIP := v1.NodeAddress{
		Type:    v1.NodeInternalIP,
		Address: p.internalIP, // TODO Nettings?
	}
	return []v1.NodeAddress{nodeHostName, nodeInternalIP}
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *BrokerProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}

func buildKeyFromNames(namespace string, name string) (string, error) {
	return fmt.Sprintf("%s-%s", namespace, name), nil
}

// buildKey is a helper for building the "key" for the providers pod store.
func buildKey(pod *v1.Pod) (string, error) {
	if pod.ObjectMeta.Namespace == "" {
		return "", fmt.Errorf("pod namespace not found")
	}

	if pod.ObjectMeta.Name == "" {
		return "", fmt.Errorf("pod name not found")
	}

	return buildKeyFromNames(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

// addAttributes adds the specified attributes to the provided span.
// attrs must be an even-sized list of string arguments.
// Otherwise, the span won't be modified.
// TODO: Refactor and move to a "tracing utilities" package.
func addAttributes(ctx context.Context, span trace.Span, attrs ...string) context.Context {
	if len(attrs)%2 == 1 {
		return ctx
	}
	for i := 0; i < len(attrs); i += 2 {
		ctx = span.WithField(ctx, attrs[i], attrs[i+1])
	}
	return ctx
}
