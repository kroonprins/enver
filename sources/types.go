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

// SourceContexts defines context-based filtering for a source
type SourceContexts struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// Source represents a source configuration from .enver.yaml
type Source struct {
	Name      string         `yaml:"name"`
	Namespace string         `yaml:"namespace"`
	Type      string         `yaml:"type"`
	Path      string         `yaml:"path"`
	Contexts  SourceContexts `yaml:"contexts"`
}

// ShouldInclude returns true if the source should be included for the given contexts
func (s *Source) ShouldInclude(contexts []string) bool {
	// If no contexts provided, include the source
	if len(contexts) == 0 {
		return true
	}

	// If include list is specified, at least one context must be in it
	if len(s.Contexts.Include) > 0 {
		found := false
		for _, selectedCtx := range contexts {
			for _, c := range s.Contexts.Include {
				if c == selectedCtx {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	// If exclude list is specified, none of the contexts can be in it
	for _, selectedCtx := range contexts {
		for _, c := range s.Contexts.Exclude {
			if c == selectedCtx {
				return false
			}
		}
	}

	return true
}

// Fetcher is the interface that all source types must implement
type Fetcher interface {
	Fetch(clientset *kubernetes.Clientset, source Source, namespace string) ([]EnvEntry, error)
}
