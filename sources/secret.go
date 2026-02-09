package sources

import (
	"context"
	"fmt"
	"strings"

	"enver/transformations"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type SecretFetcher struct{}

func (f *SecretFetcher) Fetch(clientset *kubernetes.Clientset, source Source) ([]EnvEntry, error) {
	namespace := source.GetNamespace()
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, source.Name, err)
	}

	// Convert transformation configs
	var transformConfigs []transformations.Config
	for _, tc := range source.Transformations {
		transformConfigs = append(transformConfigs, transformations.Config{
			Type:      tc.Type,
			Target:    tc.Target,
			Value:     tc.Value,
			Variables: tc.Variables,
			Output:    tc.Output,
			Key:       tc.Key,
		})
	}

	var entries []EnvEntry
	for key, value := range secret.Data {
		if len(value) > 0 && !source.ShouldExcludeVariable(key) {
			strValue := strings.TrimRight(string(value), "\n\r")

			// Apply transformations
			transformedKey, transformedValue, err := transformations.ApplyTransformations(key, strValue, transformConfigs)
			if err != nil {
				return nil, fmt.Errorf("failed to apply transformation: %w", err)
			}

			entries = append(entries, EnvEntry{
				Key:        transformedKey,
				Value:      transformedValue,
				SourceType: "Secret",
				Name:       source.Name,
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}
