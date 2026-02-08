package transformations

import "fmt"

// Config represents a transformation configuration from YAML
type Config struct {
	Type   string
	Target string
	Value  string
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
	default:
		return nil, target, fmt.Errorf("unknown transformation type: %s", cfg.Type)
	}
}

// ApplyTransformations applies a list of transformations to a key-value pair
func ApplyTransformations(key, value string, configs []Config) (string, string, error) {
	for _, cfg := range configs {
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
