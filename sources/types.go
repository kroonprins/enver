package sources

import "k8s.io/client-go/kubernetes"

// EnvEntry represents a single environment variable with its source metadata
type EnvEntry struct {
	Key        string
	Value      string
	SourceType string
	Name       string
	Namespace  string
}

// Source represents a source configuration from .enver.yaml
type Source struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Type      string `yaml:"type"`
}

// Fetcher is the interface that all source types must implement
type Fetcher interface {
	Fetch(clientset *kubernetes.Clientset, source Source, namespace string) ([]EnvEntry, error)
}
