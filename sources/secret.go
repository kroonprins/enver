package sources

import (
	"context"
	"fmt"
	"strings"

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

	var entries []EnvEntry
	for key, value := range secret.Data {
		if len(value) > 0 {
			entries = append(entries, EnvEntry{
				Key:        key,
				Value:      strings.TrimRight(string(value), "\n\r"),
				SourceType: "Secret",
				Name:       source.Name,
				Namespace:  namespace,
			})
		}
	}

	return entries, nil
}
