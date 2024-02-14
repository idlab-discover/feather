package provider

import (
	"context"
	dto "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"strings"
)

func (p *Provider) generateMockMetrics(metricsMap map[string][]*dto.Metric, resourceType string, label []*dto.LabelPair) map[string][]*dto.Metric {
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

func (p *Provider) getMetricType(metricName string) *dto.MetricType {
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

func (p *Provider) GetMetricsResource(ctx context.Context) ([]*dto.MetricFamily, error) {
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
	for _, pod := range p.pods {
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
