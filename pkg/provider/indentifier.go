package provider

import (
	corev1 "k8s.io/api/core/v1"
	"strings"
)

const identifierSeparator = "_"

func podToIdentifier(pod *corev1.Pod) string {
	if pod.Namespace == "" {
		return pod.Name
	}
	return joinIdentifierFromParts(pod.Namespace, pod.Name)
}

func podAndContainerToIdentifier(pod *corev1.Pod, container *corev1.Container) string {
	return joinIdentifierFromParts(podToIdentifier(pod), container.Name)
}

func podAndVolumeToIdentifier(pod *corev1.Pod, volume *corev1.Volume) string {
	return joinIdentifierFromParts(podToIdentifier(pod), volume.Name)
}

func filterIdentifiersByPrefix(ids []string, prefix string) (ret []string) {
	prefix = joinIdentifierFromParts(prefix, "")
	for _, id := range ids {
		if strings.HasPrefix(id, prefix) {
			ret = append(ret, id)
		}
	}
	return
}

func joinIdentifierFromParts(parts ...string) string {
	return strings.Join(parts, identifierSeparator)
}

func splitIdentifierIntoParts(id string) []string {
	return strings.Split(id, identifierSeparator)
}
