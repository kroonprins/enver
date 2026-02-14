package sources

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"enver/transformations"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DeploymentFetcher struct{}

func (f *DeploymentFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, source.Name, err)
	}

	// Convert transformation configs
	var transformConfigs []transformations.Config
	for _, tc := range source.Transformations {
		transformConfigs = append(transformConfigs, transformations.Config{
			Type:          tc.Type,
			Target:        tc.Target,
			Value:         tc.Value,
			Variables:     tc.Variables,
			Output:        tc.Output,
			Key:           tc.Key,
			BaseDirectory: outputDirectory,
		})
	}

	// Build set of container names to include
	containerFilter := make(map[string]bool)
	for _, name := range source.Containers {
		containerFilter[name] = true
	}
	filterContainers := len(containerFilter) > 0

	var entries []EnvEntry

	// Process each container in the deployment
	for _, container := range deployment.Spec.Template.Spec.Containers {
		// Skip if container is not in the filter list
		if filterContainers && !containerFilter[container.Name] {
			continue
		}

		// Process envFrom entries first (env entries take priority and come after)
		for _, envFrom := range container.EnvFrom {
			var envEntries []EnvEntry
			var err error

			if envFrom.ConfigMapRef != nil {
				envEntries, err = f.fetchFromConfigMap(clientset, namespace, envFrom.ConfigMapRef.Name, envFrom.Prefix, source, transformConfigs)
				if err != nil {
					// Check if optional
					if envFrom.ConfigMapRef.Optional != nil && *envFrom.ConfigMapRef.Optional {
						continue
					}
					return nil, err
				}
			} else if envFrom.SecretRef != nil {
				envEntries, err = f.fetchFromSecret(clientset, namespace, envFrom.SecretRef.Name, envFrom.Prefix, source, transformConfigs)
				if err != nil {
					// Check if optional
					if envFrom.SecretRef.Optional != nil && *envFrom.SecretRef.Optional {
						continue
					}
					return nil, err
				}
			}

			entries = append(entries, envEntries...)
		}

		// Process env entries (these take priority over envFrom, so they come last)
		for _, envVar := range container.Env {
			key := envVar.Name
			var value string

			if envVar.Value != "" {
				// Direct value
				value = envVar.Value
			} else if envVar.ValueFrom != nil {
				// Value from reference
				var err error
				value, err = f.resolveValueFrom(clientset, namespace, envVar.ValueFrom)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve env var %s: %w", key, err)
				}
			}

			if value != "" && !source.ShouldExcludeVariable(key) {
				transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, transformConfigs)
				if err != nil {
					return nil, fmt.Errorf("failed to apply transformation: %w", err)
				}

				entries = append(entries, EnvEntry{
					Key:        transformedKey,
					Value:      transformedValue,
					SourceType: "Deployment",
					Name:       fmt.Sprintf("%s/%s", source.Name, container.Name),
					Namespace:  namespace,
				})
			}
		}

		// Process volumeMounts that reference ConfigMaps or Secrets
		volumes := deployment.Spec.Template.Spec.Volumes
		for _, volumeMount := range container.VolumeMounts {
			volumeEntries, err := f.processVolumeMount(clientset, namespace, volumeMount, volumes, source, transformConfigs, outputDirectory)
			if err != nil {
				return nil, err
			}
			entries = append(entries, volumeEntries...)
		}
	}

	return entries, nil
}

func (f *DeploymentFetcher) resolveValueFrom(clientset *kubernetes.Clientset, namespace string, valueFrom *corev1.EnvVarSource) (string, error) {
	if valueFrom.ConfigMapKeyRef != nil {
		ref := valueFrom.ConfigMapKeyRef
		cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
		if err != nil {
			if ref.Optional != nil && *ref.Optional {
				return "", nil
			}
			return "", fmt.Errorf("failed to get configmap %s: %w", ref.Name, err)
		}
		return cm.Data[ref.Key], nil
	}

	if valueFrom.SecretKeyRef != nil {
		ref := valueFrom.SecretKeyRef
		secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
		if err != nil {
			if ref.Optional != nil && *ref.Optional {
				return "", nil
			}
			return "", fmt.Errorf("failed to get secret %s: %w", ref.Name, err)
		}
		value := secret.Data[ref.Key]
		return strings.TrimRight(string(value), "\n\r"), nil
	}

	if valueFrom.FieldRef != nil {
		// Field references (like metadata.name) cannot be resolved without pod context
		return "", nil
	}

	if valueFrom.ResourceFieldRef != nil {
		// Resource field references cannot be resolved without pod context
		return "", nil
	}

	return "", nil
}

func (f *DeploymentFetcher) fetchFromConfigMap(clientset *kubernetes.Clientset, namespace, name, prefix string, source Source, transformConfigs []transformations.Config) ([]EnvEntry, error) {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s/%s: %w", namespace, name, err)
	}

	var entries []EnvEntry
	for key, value := range cm.Data {
		envKey := prefix + key
		if value != "" && !source.ShouldExcludeVariable(envKey) {
			transformedKey, transformedValue, err := transformations.ApplyTransformations(envKey, value, transformConfigs)
			if err != nil {
				return nil, fmt.Errorf("failed to apply transformation: %w", err)
			}

			entries = append(entries, EnvEntry{
				Key:        transformedKey,
				Value:      transformedValue,
				SourceType: "Deployment",
				Name:       fmt.Sprintf("%s (ConfigMap: %s)", source.Name, name),
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}

func (f *DeploymentFetcher) fetchFromSecret(clientset *kubernetes.Clientset, namespace, name, prefix string, source Source, transformConfigs []transformations.Config) ([]EnvEntry, error) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}

	var entries []EnvEntry
	for key, value := range secret.Data {
		envKey := prefix + key
		strValue := strings.TrimRight(string(value), "\n\r")
		if strValue != "" && !source.ShouldExcludeVariable(envKey) {
			transformedKey, transformedValue, err := transformations.ApplyTransformations(envKey, strValue, transformConfigs)
			if err != nil {
				return nil, fmt.Errorf("failed to apply transformation: %w", err)
			}

			entries = append(entries, EnvEntry{
				Key:        transformedKey,
				Value:      transformedValue,
				SourceType: "Deployment",
				Name:       fmt.Sprintf("%s (Secret: %s)", source.Name, name),
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}

func (f *DeploymentFetcher) processVolumeMount(clientset *kubernetes.Clientset, namespace string, volumeMount corev1.VolumeMount, volumes []corev1.Volume, source Source, transformConfigs []transformations.Config, outputDirectory string) ([]EnvEntry, error) {
	// Find the volume that matches this volumeMount
	var volume *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == volumeMount.Name {
			volume = &volumes[i]
			break
		}
	}

	if volume == nil {
		return nil, nil
	}

	var entries []EnvEntry

	// Handle ConfigMap volume
	if volume.ConfigMap != nil {
		cmEntries, err := f.processConfigMapVolume(clientset, namespace, volume.ConfigMap, volumeMount, source, transformConfigs, outputDirectory)
		if err != nil {
			if volume.ConfigMap.Optional != nil && *volume.ConfigMap.Optional {
				return nil, nil
			}
			return nil, err
		}
		entries = append(entries, cmEntries...)
	}

	// Handle Secret volume
	if volume.Secret != nil {
		secretEntries, err := f.processSecretVolume(clientset, namespace, volume.Secret, volumeMount, source, transformConfigs, outputDirectory)
		if err != nil {
			if volume.Secret.Optional != nil && *volume.Secret.Optional {
				return nil, nil
			}
			return nil, err
		}
		entries = append(entries, secretEntries...)
	}

	// Handle Projected volume
	if volume.Projected != nil {
		for _, projSource := range volume.Projected.Sources {
			if projSource.ConfigMap != nil {
				cmEntries, err := f.processProjectedConfigMap(clientset, namespace, projSource.ConfigMap, volumeMount, source, transformConfigs, outputDirectory)
				if err != nil {
					if projSource.ConfigMap.Optional != nil && *projSource.ConfigMap.Optional {
						continue
					}
					return nil, err
				}
				entries = append(entries, cmEntries...)
			}
			if projSource.Secret != nil {
				secretEntries, err := f.processProjectedSecret(clientset, namespace, projSource.Secret, volumeMount, source, transformConfigs, outputDirectory)
				if err != nil {
					if projSource.Secret.Optional != nil && *projSource.Secret.Optional {
						continue
					}
					return nil, err
				}
				entries = append(entries, secretEntries...)
			}
		}
	}

	return entries, nil
}

func (f *DeploymentFetcher) processConfigMapVolume(clientset *kubernetes.Clientset, namespace string, cmVolume *corev1.ConfigMapVolumeSource, volumeMount corev1.VolumeMount, source Source, transformConfigs []transformations.Config, outputDirectory string) ([]EnvEntry, error) {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), cmVolume.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s/%s: %w", namespace, cmVolume.Name, err)
	}

	// Build key to path mapping from items if specified
	keyToPath := make(map[string]string)
	if len(cmVolume.Items) > 0 {
		for _, item := range cmVolume.Items {
			keyToPath[item.Key] = item.Path
		}
	}

	var entries []EnvEntry
	for key, value := range cm.Data {
		// If items are specified, only process those keys
		if len(cmVolume.Items) > 0 {
			if _, ok := keyToPath[key]; !ok {
				continue
			}
		}

		if source.ShouldExcludeVariable(key) {
			continue
		}

		// Determine the file path
		filePath := key
		if path, ok := keyToPath[key]; ok {
			filePath = path
		}
		outputPath := filepath.Join(volumeMount.MountPath, filePath)

		// Apply file transformation
		fileTransformConfigs := append(transformConfigs, transformations.Config{
			Type:          "file",
			Output:        outputPath,
			Key:           key,
			BaseDirectory: outputDirectory,
		})

		transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, fileTransformConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to apply transformation: %w", err)
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Deployment",
			Name:       fmt.Sprintf("%s (Volume: %s, ConfigMap: %s)", source.Name, volumeMount.Name, cmVolume.Name),
			Namespace:  namespace,
		})
	}

	return entries, nil
}

func (f *DeploymentFetcher) processSecretVolume(clientset *kubernetes.Clientset, namespace string, secretVolume *corev1.SecretVolumeSource, volumeMount corev1.VolumeMount, source Source, transformConfigs []transformations.Config, outputDirectory string) ([]EnvEntry, error) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), secretVolume.SecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretVolume.SecretName, err)
	}

	// Build key to path mapping from items if specified
	keyToPath := make(map[string]string)
	if len(secretVolume.Items) > 0 {
		for _, item := range secretVolume.Items {
			keyToPath[item.Key] = item.Path
		}
	}

	var entries []EnvEntry
	for key, value := range secret.Data {
		// If items are specified, only process those keys
		if len(secretVolume.Items) > 0 {
			if _, ok := keyToPath[key]; !ok {
				continue
			}
		}

		if source.ShouldExcludeVariable(key) {
			continue
		}

		strValue := strings.TrimRight(string(value), "\n\r")

		// Determine the file path
		filePath := key
		if path, ok := keyToPath[key]; ok {
			filePath = path
		}
		outputPath := filepath.Join(volumeMount.MountPath, filePath)

		// Apply file transformation
		fileTransformConfigs := append(transformConfigs, transformations.Config{
			Type:          "file",
			Output:        outputPath,
			Key:           key,
			BaseDirectory: outputDirectory,
		})

		transformedKey, transformedValue, err := transformations.ApplyTransformations(key, strValue, fileTransformConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to apply transformation: %w", err)
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Deployment",
			Name:       fmt.Sprintf("%s (Volume: %s, Secret: %s)", source.Name, volumeMount.Name, secretVolume.SecretName),
			Namespace:  namespace,
		})
	}

	return entries, nil
}

func (f *DeploymentFetcher) processProjectedConfigMap(clientset *kubernetes.Clientset, namespace string, cmProjection *corev1.ConfigMapProjection, volumeMount corev1.VolumeMount, source Source, transformConfigs []transformations.Config, outputDirectory string) ([]EnvEntry, error) {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), cmProjection.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s/%s: %w", namespace, cmProjection.Name, err)
	}

	// Build key to path mapping from items if specified
	keyToPath := make(map[string]string)
	if len(cmProjection.Items) > 0 {
		for _, item := range cmProjection.Items {
			keyToPath[item.Key] = item.Path
		}
	}

	var entries []EnvEntry
	for key, value := range cm.Data {
		// If items are specified, only process those keys
		if len(cmProjection.Items) > 0 {
			if _, ok := keyToPath[key]; !ok {
				continue
			}
		}

		if source.ShouldExcludeVariable(key) {
			continue
		}

		// Determine the file path
		filePath := key
		if path, ok := keyToPath[key]; ok {
			filePath = path
		}
		outputPath := filepath.Join(volumeMount.MountPath, filePath)

		// Apply file transformation
		fileTransformConfigs := append(transformConfigs, transformations.Config{
			Type:          "file",
			Output:        outputPath,
			Key:           key,
			BaseDirectory: outputDirectory,
		})

		transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, fileTransformConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to apply transformation: %w", err)
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Deployment",
			Name:       fmt.Sprintf("%s (Projected Volume: %s, ConfigMap: %s)", source.Name, volumeMount.Name, cmProjection.Name),
			Namespace:  namespace,
		})
	}

	return entries, nil
}

func (f *DeploymentFetcher) processProjectedSecret(clientset *kubernetes.Clientset, namespace string, secretProjection *corev1.SecretProjection, volumeMount corev1.VolumeMount, source Source, transformConfigs []transformations.Config, outputDirectory string) ([]EnvEntry, error) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), secretProjection.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretProjection.Name, err)
	}

	// Build key to path mapping from items if specified
	keyToPath := make(map[string]string)
	if len(secretProjection.Items) > 0 {
		for _, item := range secretProjection.Items {
			keyToPath[item.Key] = item.Path
		}
	}

	var entries []EnvEntry
	for key, value := range secret.Data {
		// If items are specified, only process those keys
		if len(secretProjection.Items) > 0 {
			if _, ok := keyToPath[key]; !ok {
				continue
			}
		}

		if source.ShouldExcludeVariable(key) {
			continue
		}

		strValue := strings.TrimRight(string(value), "\n\r")

		// Determine the file path
		filePath := key
		if path, ok := keyToPath[key]; ok {
			filePath = path
		}
		outputPath := filepath.Join(volumeMount.MountPath, filePath)

		// Apply file transformation
		fileTransformConfigs := append(transformConfigs, transformations.Config{
			Type:          "file",
			Output:        outputPath,
			Key:           key,
			BaseDirectory: outputDirectory,
		})

		transformedKey, transformedValue, err := transformations.ApplyTransformations(key, strValue, fileTransformConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to apply transformation: %w", err)
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Deployment",
			Name:       fmt.Sprintf("%s (Projected Volume: %s, Secret: %s)", source.Name, volumeMount.Name, secretProjection.Name),
			Namespace:  namespace,
		})
	}

	return entries, nil
}
