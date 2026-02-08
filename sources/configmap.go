package sources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ConfigMapFetcher struct{}

func (f *ConfigMapFetcher) Fetch(clientset *kubernetes.Clientset, source Source, namespace string) ([]EnvEntry, error) {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap %s/%s: %w", namespace, source.Name, err)
	}

	var entries []EnvEntry
	for key, value := range cm.Data {
		if value != "" {
			entries = append(entries, EnvEntry{
				Key:        key,
				Value:      value,
				SourceType: "ConfigMap",
				Name:       source.Name,
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}
