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
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// VarEntry defines a single variable for the Vars source type
type VarEntry struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// VolumeMountKeyMapping defines key mappings for volume mounts in Deployment source
type VolumeMountKeyMapping struct {
	Kind     string            `yaml:"kind"`     // ConfigMap or Secret
	Name     string            `yaml:"name"`     // name of the ConfigMap/Secret
	Mappings map[string]string `yaml:"mappings"` // original key -> new key
}

// ContainerFileExtract defines a file to extract from a container
type ContainerFileExtract struct {
	Container string `yaml:"container"` // container name to extract from
	Path      string `yaml:"path"`      // path to file in the container
	Output    string `yaml:"output"`    // output path relative to output directory
	Key       string `yaml:"key"`       // environment variable name for the file path
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
	Name                   string                  `yaml:"name"`
	Namespace              string                  `yaml:"namespace"`
	Type                   string                  `yaml:"type"`
	Kind                   string                  `yaml:"kind"`                   // for Container source type: Pod, Deployment, StatefulSet, DaemonSet
	Path                   string                  `yaml:"path"`
	Contexts               SourceContexts          `yaml:"contexts"`
	Variables              SourceVariables         `yaml:"variables"`
	Transformations        []TransformationConfig  `yaml:"transformations"`
	Vars                   []VarEntry              `yaml:"vars"`                   // for Vars source type
	Containers             []string                `yaml:"containers"`             // for Deployment/Container source type
	VolumeMountKeyMappings []VolumeMountKeyMapping `yaml:"volumeMountKeyMappings"` // for Deployment source type
	Files                  []ContainerFileExtract  `yaml:"files"`                  // for Container source type
}

// ShouldExcludeVariable returns true if the variable should be excluded
// Supports exact matches and regex patterns
// If include list is specified, only variables matching include patterns are kept
// Exclude patterns are applied after include patterns
func (s *Source) ShouldExcludeVariable(varName string) bool {
	// If include list is specified, variable must match at least one pattern
	if len(s.Variables.Include) > 0 {
		included := false
		for _, pattern := range s.Variables.Include {
			if matchesPattern(varName, pattern) {
				included = true
				break
			}
		}
		if !included {
			return true
		}
	}

	// Check exclude patterns
	for _, pattern := range s.Variables.Exclude {
		if matchesPattern(varName, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern returns true if varName matches the pattern (exact or regex)
func matchesPattern(varName, pattern string) bool {
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

// GetVolumeMountKeyMapping returns the mapped key for a volume mount, or the original key if no mapping exists
func (s *Source) GetVolumeMountKeyMapping(kind, name, key string) string {
	for _, mapping := range s.VolumeMountKeyMappings {
		if mapping.Kind == kind && mapping.Name == name {
			if newKey, ok := mapping.Mappings[key]; ok {
				return newKey
			}
		}
	}
	return key
}

// Fetcher is the interface that all source types must implement
type Fetcher interface {
	Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error)
}
