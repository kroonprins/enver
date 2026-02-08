package sources

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"enver/transformations"

	"k8s.io/client-go/kubernetes"
)

type EnvFileFetcher struct{}

func (f *EnvFileFetcher) Fetch(clientset *kubernetes.Clientset, source Source) ([]EnvEntry, error) {
	if source.Path == "" {
		return nil, fmt.Errorf("path is required for EnvFile source %q", source.Name)
	}

	file, err := os.Open(source.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file %s: %w", source.Path, err)
	}
	defer file.Close()

	// Convert transformation configs
	var transformConfigs []transformations.Config
	for _, tc := range source.Transformations {
		transformConfigs = append(transformConfigs, transformations.Config{
			Type:   tc.Type,
			Target: tc.Target,
			Value:  tc.Value,
		})
	}

	var entries []EnvEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key != "" && !source.ShouldExcludeVariable(key) {
			// Apply transformations
			transformedKey, transformedValue, err := transformations.ApplyTransformations(key, value, transformConfigs)
			if err != nil {
				return nil, fmt.Errorf("failed to apply transformation: %w", err)
			}

			entries = append(entries, EnvEntry{
				Key:        transformedKey,
				Value:      transformedValue,
				SourceType: "EnvFile",
				Name:       source.Path,
				Namespace:  "",
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file %s: %w", source.Path, err)
	}

	return entries, nil
}
