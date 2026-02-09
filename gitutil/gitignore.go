package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
)

// IsIgnored checks if a file path is covered by .gitignore
func IsIgnored(path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	err := cmd.Run()
	return err == nil
}

// IsGitRepo checks if the current directory is inside a git repository
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	err := cmd.Run()
	return err == nil
}

// EnsureGitignored checks if a file is gitignored, and if not, prompts the user
// to add it to .gitignore. Returns an error if something goes wrong.
func EnsureGitignored(filePath string) error {
	// Skip if not in a git repo
	if !IsGitRepo() {
		return nil
	}

	// Skip if already ignored
	if IsIgnored(filePath) {
		return nil
	}

	// Prompt user
	dir := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)

	var choice string
	prompt := &survey.Select{
		Message: fmt.Sprintf("File %q is not in .gitignore. Add to .gitignore?", filePath),
		Options: []string{
			fmt.Sprintf("Add file (%s)", filePath),
			fmt.Sprintf("Add directory (%s/)", dir),
			"Skip",
		},
	}

	err := survey.AskOne(prompt, &choice)
	if err != nil {
		return fmt.Errorf("gitignore prompt failed: %w", err)
	}

	var entryToAdd string
	switch choice {
	case fmt.Sprintf("Add file (%s)", filePath):
		entryToAdd = filePath
	case fmt.Sprintf("Add directory (%s/)", dir):
		entryToAdd = dir + "/"
	default:
		// User chose to skip
		return nil
	}

	// Find .gitignore location (in repo root)
	gitRoot, err := getGitRoot()
	if err != nil {
		return fmt.Errorf("failed to find git root: %w", err)
	}

	gitignorePath := filepath.Join(gitRoot, ".gitignore")

	// Append to .gitignore
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore: %w", err)
	}
	defer f.Close()

	// Make sure we start on a new line
	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat .gitignore: %w", err)
	}

	prefix := ""
	if stat.Size() > 0 {
		// Read last byte to check if file ends with newline
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			return fmt.Errorf("failed to read .gitignore: %w", err)
		}
		if len(content) > 0 && content[len(content)-1] != '\n' {
			prefix = "\n"
		}
	}

	if _, err := f.WriteString(prefix + entryToAdd + "\n"); err != nil {
		return fmt.Errorf("failed to write to .gitignore: %w", err)
	}

	fmt.Printf("Added %q to .gitignore\n", entryToAdd)

	// Also add the filename pattern (without path) for better coverage
	if entryToAdd == filePath && fileName != entryToAdd {
		// The file was added with full path, no need to add pattern
	}

	return nil
}

func getGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Remove trailing newline
	root := string(output)
	if len(root) > 0 && root[len(root)-1] == '\n' {
		root = root[:len(root)-1]
	}
	return root, nil
}
