package provider

import (
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstanceVolume is a Volume with an expanded VolumeSource
// This makes the life of the backend a lot easier and moves work to the provider
type InstanceVolume struct {
	ID string
	corev1.Volume
	Secret    *InstanceSecretVolumeSource
	ConfigMap *InstanceConfigMapVolumeSource
	Projected *InstanceProjectedVolumeSource
}

type InstanceSecretVolumeSource struct {
	*corev1.SecretVolumeSource
	Object *corev1.Secret
}

type InstanceConfigMapVolumeSource struct {
	*corev1.ConfigMapVolumeSource
	Object *corev1.ConfigMap
}

type InstanceProjectedVolumeSource struct {
	*corev1.ProjectedVolumeSource
	Sources []InstanceVolumeProjection
}

type InstanceVolumeProjection struct {
	corev1.VolumeProjection
	Secret              *InstanceSecretProjection
	ConfigMap           *InstanceConfigMapProjection
	ServiceAccountToken *InstanceServiceAccountTokenProjection
}

type InstanceSecretProjection struct {
	*corev1.SecretProjection
	Object *corev1.Secret
}

type InstanceConfigMapProjection struct {
	*corev1.ConfigMapProjection
	Object *corev1.ConfigMap
}

type InstanceServiceAccountTokenProjection struct {
	*corev1.ServiceAccountTokenProjection
	Object *corev1.Secret
}

// newInstanceVolume creates a new InstanceVolume by looking up the resources of the Volume and extending them with
// a VolumeMount to let the backend know how to mount them
func (p *Provider) newInstanceVolume(pod *corev1.Pod, volume corev1.Volume) (InstanceVolume, error) {
	instanceVolumeID := joinIdentifierFromParts(pod.Namespace, pod.Name, volume.Name)
	instanceVolume := InstanceVolume{ID: instanceVolumeID, Volume: volume}
	switch {
	case volume.HostPath != nil:
	case volume.EmptyDir != nil:
	// TODO: GCEPersistentDisk *corev1.GCEPersistentDiskVolumeSource
	// TODO: AWSElasticBlockStore *corev1.AWSElasticBlockStoreVolumeSource
	// TODO: GitRepo *corev1.GitRepoVolumeSource
	case volume.Secret != nil:
		object, err := p.resourceManager.GetSecret(volume.Secret.SecretName, pod.Namespace)
		if volume.Secret.Optional != nil && !*volume.Secret.Optional && apierrors.IsNotFound(err) {
			return InstanceVolume{}, fmt.Errorf("secret %s is required by volume %s and does not exist", volume.Secret.SecretName, volume.Name)
		}
		if object == nil {
			break
		}
		instanceVolume.Secret = &InstanceSecretVolumeSource{
			SecretVolumeSource: volume.Secret,
			Object:             object,
		}
	case volume.NFS != nil:
	// TODO: ISCSI *ISCSIVolumeSource
	// TODO: Glusterfs *GlusterfsVolumeSource
	// TODO: PersistentVolumeClaim *PersistentVolumeClaimVolumeSource
	// TODO: RBD *RBDVolumeSource
	// TODO: FlexVolume *FlexVolumeSource
	// TODO: Cinder *CinderVolumeSource
	// TODO: CephFS *CephFSVolumeSource
	// TODO: Flocker *FlockerVolumeSource
	// TODO: DownwardAPI *DownwardAPIVolumeSource
	// TODO: FC *FCVolumeSource
	// TODO: AzureFile *AzureFileVolumeSource
	case volume.ConfigMap != nil:
		object, err := p.resourceManager.GetConfigMap(volume.ConfigMap.Name, pod.Namespace)
		if volume.ConfigMap.Optional != nil && !*volume.ConfigMap.Optional && apierrors.IsNotFound(err) {
			return InstanceVolume{}, fmt.Errorf("configMap %s is required by volume %s and does not exist", volume.ConfigMap.Name, volume.Name)
		}
		if object == nil {
			break
		}
		instanceVolume.ConfigMap = &InstanceConfigMapVolumeSource{
			ConfigMapVolumeSource: volume.ConfigMap,
			Object:                object,
		}
	// TODO: VsphereVolume *VsphereVirtualDiskVolumeSource
	// TODO: Quobyte *QuobyteVolumeSource
	// TODO: AzureDisk *AzureDiskVolumeSource
	// TODO: PhotonPersistentDisk *PhotonPersistentDiskVolumeSource
	case volume.Projected != nil:
		projected := &InstanceProjectedVolumeSource{}
		for _, source := range volume.Projected.Sources {
			instanceSource := InstanceVolumeProjection{VolumeProjection: source}
			switch {
			case source.Secret != nil:
				object, err := p.resourceManager.GetSecret(source.Secret.Name, pod.Namespace)
				if source.Secret.Optional != nil && !*source.Secret.Optional && apierrors.IsNotFound(err) {
					return InstanceVolume{}, fmt.Errorf("secret %s is required by volume %s and does not exist", source.Secret.Name, volume.Name)
				}
				if object == nil {
					break
				}
				instanceSource.Secret = &InstanceSecretProjection{
					SecretProjection: source.Secret,
					Object:           object,
				}
			case source.DownwardAPI != nil:
				// TODO (ignored)
			case source.ConfigMap != nil:
				object, err := p.resourceManager.GetConfigMap(source.ConfigMap.Name, pod.Namespace)
				if source.ConfigMap.Optional != nil && !*source.ConfigMap.Optional && apierrors.IsNotFound(err) {
					return InstanceVolume{}, fmt.Errorf("secret %s is required by volume %s and does not exist", source.ConfigMap.Name, volume.Name)
				}
				if object == nil {
					break
				}
				instanceSource.ConfigMap = &InstanceConfigMapProjection{
					ConfigMapProjection: source.ConfigMap,
					Object:              object,
				}
			case source.ServiceAccountToken != nil:
				// Try to find the ServiceAccount
				serviceAccount, err := p.resourceManager.GetServiceAccount(pod.Spec.ServiceAccountName, pod.Namespace)
				if err != nil {
					return InstanceVolume{}, fmt.Errorf("unable to find serviceAccountToken for volume %s", volume.Name)
				}
				// TODO: normally we should perform a TokenRequest here, starting from Kubernetes v1.24, in order to obtain the token?
				object := &corev1.Secret{
					TypeMeta: v1.TypeMeta{
						Kind:       "ServiceAccountToken",
						APIVersion: "",
					},
					ObjectMeta: v1.ObjectMeta{
						Namespace: pod.Namespace,
						Name:      serviceAccount.Name,
					},
					StringData: map[string]string{"token": "{}"}, // TODO: The actual token from the TokenRequest
					Type:       corev1.SecretTypeServiceAccountToken,
				}
				instanceSource.ServiceAccountToken = &InstanceServiceAccountTokenProjection{
					ServiceAccountTokenProjection: source.ServiceAccountToken,
					Object:                        object,
				}

			}
			projected.Sources = append(projected.Sources, instanceSource)
		}
		instanceVolume.Projected = projected
	// TODO: PortworxVolume *PortworxVolumeSource
	// TODO: ScaleIO *ScaleIOVolumeSource
	// TODO: StorageOS *StorageOSVolumeSource
	// TODO: CSI *CSIVolumeSource
	// TODO: Ephemeral *EphemeralVolumeSource
	default:
		return InstanceVolume{}, errors.Errorf("volume %q has an unsupported type", volume.Name)
	}
	return instanceVolume, nil
}
