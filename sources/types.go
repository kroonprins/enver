package sources

import (
	"regexp"

	"k8s.io/client-go/kubernetes"
)

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

// SourceVariables defines variable-level filtering for a source
type SourceVariables struct {
	Exclude []string `yaml:"exclude"`
}

// VarEntry defines a single variable for the Vars source type
type VarEntry struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// TransformationConfig defines a transformation to apply to variables
type TransformationConfig struct {
	Type      string   `yaml:"type"`      // base64_decode, base64_encode, prefix, suffix, file
	Target    string   `yaml:"target"`    // key or value
	Value     string   `yaml:"value"`     // parameter for prefix/suffix
	Variables []string `yaml:"variables"` // limit to these variable names (empty = apply to all)
	Output    string   `yaml:"output"`    // output file path (for file transformation)
	Key       string   `yaml:"key"`       // new key name (for file transformation)
}

// Source represents a source configuration from .enver.yaml
type Source struct {
	Name            string                 `yaml:"name"`
	Namespace       string                 `yaml:"namespace"`
	Type            string                 `yaml:"type"`
	Path            string                 `yaml:"path"`
	Contexts        SourceContexts         `yaml:"contexts"`
	Variables       SourceVariables        `yaml:"variables"`
	Transformations []TransformationConfig `yaml:"transformations"`
	Vars            []VarEntry             `yaml:"vars"`       // for Vars source type
	Containers      []string               `yaml:"containers"` // for Deployment source type
}

// ShouldExcludeVariable returns true if the variable should be excluded
// Supports exact matches and regex patterns
func (s *Source) ShouldExcludeVariable(varName string) bool {
	for _, pattern := range s.Variables.Exclude {
		// First try exact match
		if pattern == varName {
			return true
		}
		// Then try regex match
		if re, err := regexp.Compile(pattern); err == nil {
			if re.MatchString(varName) {
				return true
			}
		}
	}
	return false
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

// GetNamespace returns the namespace, defaulting to "default" if not specified
func (s *Source) GetNamespace() string {
	if s.Namespace == "" {
		return "default"
	}
	return s.Namespace
}

// Fetcher is the interface that all source types must implement
type Fetcher interface {
	Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error)
}
