package sources

import (
	"context"
	"fmt"
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
