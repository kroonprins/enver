package transformations

import (
	"fmt"
	"os"
	"path/filepath"

	"enver/gitutil"
)

// FileTransformation writes the value to a file and returns the file path
type FileTransformation struct {
	Output string
	Key    string
}

// TransformKeyValue writes the value to the output file and returns the new key and file path
func (t *FileTransformation) TransformKeyValue(key, value string) (string, string, error) {
	if t.Output == "" {
		return key, value, fmt.Errorf("output is required for file transformation")
	}
	if t.Key == "" {
		return key, value, fmt.Errorf("key is required for file transformation")
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(t.Output)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return key, value, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write value to file
	if err := os.WriteFile(t.Output, []byte(value), 0644); err != nil {
		return key, value, fmt.Errorf("failed to write file %s: %w", t.Output, err)
	}

	// Check if output file should be added to .gitignore
	if err := gitutil.EnsureGitignored(t.Output); err != nil {
		return key, value, err
	}

	return t.Key, t.Output, nil
}
