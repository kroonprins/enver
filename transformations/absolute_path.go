package transformations

import (
	"path/filepath"
)

// AbsolutePath converts a relative path to an absolute path
type AbsolutePath struct{}

func (t *AbsolutePath) Transform(input string) string {
	absPath, err := filepath.Abs(input)
	if err != nil {
		// If conversion fails, return original
		return input
	}
	return absPath
}
