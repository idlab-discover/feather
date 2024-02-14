package provider

import (
	corev1 "k8s.io/api/core/v1"
)

// InstanceVolumeMount is a VolumeMount with an assigned InstanceVolume
type InstanceVolumeMount struct {
	corev1.VolumeMount
	Volume InstanceVolume
}
