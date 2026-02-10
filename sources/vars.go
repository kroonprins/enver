package sources

import (
	"enver/transformations"

	"k8s.io/client-go/kubernetes"
)

type VarsFetcher struct{}

func (f *VarsFetcher) Fetch(clientset *kubernetes.Clientset, source Source, outputDirectory string) ([]EnvEntry, error) {
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
	for _, v := range source.Vars {
		if v.Name == "" {
			continue
		}

		if source.ShouldExcludeVariable(v.Name) {
			continue
		}

		// Apply transformations
		transformedKey, transformedValue, err := transformations.ApplyTransformations(v.Name, v.Value, transformConfigs)
		if err != nil {
			return nil, err
		}

		entries = append(entries, EnvEntry{
			Key:        transformedKey,
			Value:      transformedValue,
			SourceType: "Vars",
			Name:       source.Name,
			Namespace:  "",
		})
	}

	return entries, nil
}
