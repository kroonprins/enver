package transformations

import (
	"fmt"
	"path/filepath"
)

// Config represents a transformation configuration from YAML
type Config struct {
	Type          string
	Target        string
	Value         string
	Variables     []string
	Output        string
	Key           string
	BaseDirectory string // base directory for relative paths in file transformation
}

// BuildTransformation creates a Transformation from a config
func BuildTransformation(cfg Config) (Transformation, Target, error) {
	target := TargetValue
	if cfg.Target == "key" {
		target = TargetKey
	}

	switch cfg.Type {
	case "base64_decode":
		return &Base64Decode{}, target, nil
	case "base64_encode":
		return &Base64Encode{}, target, nil
	case "prefix":
		return &Prefix{Value: cfg.Value}, target, nil
	case "suffix":
		return &Suffix{Value: cfg.Value}, target, nil
	case "absolute_path":
		if target == TargetKey {
			return nil, target, fmt.Errorf("absolute_path transformation can only be applied to values")
		}
		return &AbsolutePath{}, target, nil
	default:
		return nil, target, fmt.Errorf("unknown transformation type: %s", cfg.Type)
	}
}

// shouldApplyToVariable checks if the transformation should apply to the given variable
func shouldApplyToVariable(varName string, variables []string) bool {
	// If no variables specified, apply to all
	if len(variables) == 0 {
		return true
	}

	for _, v := range variables {
		if v == varName {
			return true
		}
	}

	return false
}

// ApplyTransformations applies a list of transformations to a key-value pair
func ApplyTransformations(key, value string, configs []Config) (string, string, error) {
	for _, cfg := range configs {
		// Skip if transformation is limited to specific variables and this isn't one
		if !shouldApplyToVariable(key, cfg.Variables) {
			continue
		}

		// Handle file transformation specially since it modifies both key and value
		if cfg.Type == "file" {
			if cfg.Target != "" && cfg.Target != "value" {
				return key, value, fmt.Errorf("file transformation can only be applied to values")
			}
			// Resolve relative paths against base directory
			outputPath := cfg.Output
			if !filepath.IsAbs(outputPath) && cfg.BaseDirectory != "" {
				outputPath = filepath.Join(cfg.BaseDirectory, outputPath)
			}
			ft := &FileTransformation{Output: outputPath, Key: cfg.Key}
			newKey, newValue, err := ft.TransformKeyValue(key, value)
			if err != nil {
				return key, value, err
			}
			key = newKey
			value = newValue
			continue
		}

		t, target, err := BuildTransformation(cfg)
		if err != nil {
			return key, value, err
		}

		switch target {
		case TargetKey:
			key = t.Transform(key)
		case TargetValue:
			value = t.Transform(value)
		}
	}

	return key, value, nil
}
