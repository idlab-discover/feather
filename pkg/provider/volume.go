package provider

//// Volume describes what volumes should be created.
//type Volume int
//
//const (
//	volumeAll Volume = iota
//	volumeConfigMap
//	volumeSecret
//)
//
//// volumes inspects the PodSpec.Volumes attribute and returns a mapping with the volume's Name and the directory on-disk that
//// should be used for this. The on-disk structure is prepared and can be used.
//// which considered what volumes should be setup. Defaults to volumeAll
//func (p *Provider) volumes(pod *corev1.Pod, which Volume) (map[string]string, error) {
//	fnlog := log.
//		WithField("podNamespace", pod.Namespace).
//		WithField("podName", pod.Name)
//
//	vol := make(map[string]string)
//	uid, gid, err := uidGidFromSecurityContext(pod, p.config.OverrideRootUID)
//	if err != nil {
//		return nil, err
//	}
//	for i, v := range pod.Spec.Volumes {
//		fnlog.Debugf("looking at volume %q#%d", v.Name, i)
//		switch {
//		case v.HostPath != nil:
//			if which != volumeAll {
//				continue
//			}
//
//			// v.Path should exist and be usuable by this pod. No checks are done here.
//			vol[v.Name] = ""
//
//		case v.EmptyDir != nil:
//			if which != volumeAll {
//				continue
//			}
//			dir, err := p.setupPaths(pod, emptyDir, i)
//			if err != nil {
//				return nil, err
//			}
//			fnlog.Debugf("created %q for emptyDir %q", dir, v.Name)
//			vol[v.Name] = dir
//
//		case v.Secret != nil:
//			if which != volumeAll && which != volumeSecret {
//				continue
//			}
//			secret, err := p.podResourceManager.SecretLister().Secrets(pod.Namespace).Get(v.Secret.SecretName)
//			if v.Secret.Optional != nil && !*v.Secret.Optional && errors.IsNotFound(err) {
//				return nil, fmt.Errorf("secret %s is required by pod %s and does not exist", v.Secret.SecretName, pod.Name)
//			}
//			if secret == nil {
//				continue
//			}
//
//			dir, err := p.setupPaths(pod, secretDir, i)
//			if err != nil {
//				return nil, err
//			}
//			fnlog.Debugf("created %q for secret %q", dir, v.Name)
//
//			for k, v := range secret.StringData {
//				data, err := base64.StdEncoding.DecodeString(string(v))
//				if err != nil {
//					return nil, err
//				}
//				if err := writeFile(dir, k, uid, gid, data); err != nil {
//					return nil, err
//				}
//			}
//			for k, v := range secret.Data {
//				if err := writeFile(dir, k, uid, gid, []byte(v)); err != nil {
//					return nil, err
//				}
//			}
//			vol[v.Name] = dir
//
//		case v.ConfigMap != nil:
//			if which != volumeAll && which != volumeConfigMap {
//				continue
//			}
//			configMap, err := p.podResourceManager.ConfigMapLister().ConfigMaps(pod.Namespace).Get(v.ConfigMap.Name)
//			if v.ConfigMap.Optional != nil && !*v.ConfigMap.Optional && errors.IsNotFound(err) {
//				return nil, fmt.Errorf("configMap %s is required by pod %s and does not exist", v.ConfigMap.Name, pod.Name)
//			}
//			if configMap == nil {
//				continue
//			}
//
//			dir, err := p.setupPaths(pod, configmapDir, i)
//			if err != nil {
//				return nil, err
//			}
//			fnlog.Debugf("created %q for configmap %q", dir, v.Name)
//
//			for k, v := range configMap.Data {
//				if err := writeFile(dir, k, uid, gid, []byte(v)); err != nil {
//					return nil, err
//				}
//			}
//			for k, v := range configMap.BinaryData {
//				if err := writeFile(dir, k, uid, gid, v); err != nil {
//					return nil, err
//				}
//			}
//			vol[v.Name] = dir
//
//		case v.Projected != nil:
//			for _, source := range v.Projected.Sources {
//				switch {
//				case source.ServiceAccountToken != nil:
//					// This is still stored in a secret, hence the dance to figure out what secret.
//					secrets, err := p.podResourceManager.SecretLister().Secrets(pod.Namespace).List(labels.Everything())
//					if err != nil {
//						return nil, err
//					}
//				Secrets:
//					for _, secret := range secrets {
//						if secret.Type != corev1.SecretTypeServiceAccountToken {
//							continue
//						}
//						// annotation now needs to match the pod.ServiceAccountName
//						for k, a := range secret.ObjectMeta.Annotations {
//							if k == "kubernetes.io/service-account.name" && a == pod.Spec.ServiceAccountName {
//								// this is the secret we're after. Now the projected service account has a path element, which is the only path
//								// we want from this secret, but it could still be in StringData or Data
//								dir, err := p.setupPaths(pod, secretDir, i)
//								if err != nil {
//									return nil, err
//								}
//								fnlog.Debugf("created %q for projected serviceAccountToken (secret) %q", dir, v.Name)
//
//								for k, v := range secret.StringData {
//									data, err := base64.StdEncoding.DecodeString(string(v))
//									if err != nil {
//										return nil, err
//									}
//									if err := writeFile(dir, k, uid, gid, data); err != nil {
//										return nil, err
//									}
//								}
//								for k, v := range secret.Data {
//									if err := writeFile(dir, k, uid, gid, []byte(v)); err != nil {
//										return nil, err
//									}
//								}
//								vol[v.Name] = dir
//								break Secrets
//							}
//						}
//					}
//
//				case source.Secret != nil:
//					secret, err := p.podResourceManager.SecretLister().Secrets(pod.Namespace).Get(source.Secret.Name)
//					if source.Secret.Optional != nil && !*source.Secret.Optional && errors.IsNotFound(err) {
//						return nil, fmt.Errorf("projected secret %s is required by pod %s and does not exist", source.Secret.Name, pod.Name)
//					}
//					if secret == nil {
//						continue
//					}
//
//					dir, err := p.setupPaths(pod, secretDir, i)
//					if err != nil {
//						return nil, err
//					}
//					fnlog.Debugf("created %q for projected secret %q", dir, v.Name)
//
//					for _, keyToPath := range source.ConfigMap.Items {
//						for k, v := range secret.StringData {
//							if keyToPath.Key == k {
//								data, err := base64.StdEncoding.DecodeString(string(v))
//								if err != nil {
//									return nil, err
//								}
//								if err := writeFile(dir, keyToPath.Path, uid, gid, data); err != nil {
//									return nil, err
//								}
//							}
//						}
//						for k, v := range secret.Data {
//							if keyToPath.Key == k {
//								if err := writeFile(dir, keyToPath.Path, uid, gid, []byte(v)); err != nil {
//									return nil, err
//								}
//							}
//						}
//					}
//					vol[v.Name] = dir
//
//				case source.ConfigMap != nil:
//					configMap, err := p.podResourceManager.ConfigMapLister().ConfigMaps(pod.Namespace).Get(source.ConfigMap.Name)
//					if source.ConfigMap.Optional != nil && !*source.ConfigMap.Optional && errors.IsNotFound(err) {
//						return nil, fmt.Errorf("projected configMap %s is required by pod %s and does not exist", source.ConfigMap.Name, pod.Name)
//					}
//					if configMap == nil {
//						continue
//					}
//
//					dir, err := p.setupPaths(pod, configmapDir, i)
//					if err != nil {
//						return nil, err
//					}
//					fnlog.Debugf("created %q for projected configmap %q", dir, v.Name)
//
//					for _, keyToPath := range source.ConfigMap.Items {
//						for k, v := range configMap.Data {
//							if keyToPath.Key == k {
//								if err := writeFile(dir, keyToPath.Path, uid, gid, []byte(v)); err != nil {
//									return nil, err
//								}
//							}
//						}
//						for k, v := range configMap.BinaryData {
//							if keyToPath.Key == k {
//								if err := writeFile(dir, keyToPath.Path, uid, gid, v); err != nil {
//									return nil, err
//								}
//							}
//						}
//					}
//					vol[v.Name] = dir
//
//				}
//			}
//
//		default:
//			return nil, fmt.Errorf("pod %s requires volume %s which is of an unsupported type", pod.Name, v.Name)
//		}
//	}
//
//	return vol, nil
//}
