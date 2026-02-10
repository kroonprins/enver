package sources

import (
	"context"
	"fmt"

	"enver/transformations"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ConfigMapFetcher struct{}

func (f *ConfigMapFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s/%s: %w", namespace, source.Name, err)
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

	var entries []EnvEntry
	for key, value := range cm.Data {
		if value != "" && !source.ShouldExcludeVariable(key) {
			// Apply transformations
			transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, transformConfigs)
			if err != nil {
				return nil, fmt.Errorf("failed to apply transformation: %w", err)
			}

			entries = append(entries, EnvEntry{
				Key:        transformedKey,
				Value:      transformedValue,
				SourceType: "ConfigMap",
				Name:       source.Name,
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}
