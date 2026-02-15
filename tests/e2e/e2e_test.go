package e2e

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var (
	update     = flag.Bool("update", false, "update golden files")
	binaryPath string
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Skip if E2E_TEST is not set
	if os.Getenv("E2E_TEST") == "" {
		fmt.Println("Skipping E2E tests. Set E2E_TEST=1 to run them.")
		os.Exit(0)
	}

	// Build the binary
	fmt.Println("Building enver binary...")
	cmd := exec.Command("go", "build", "-o", "enver-test", "../../.")
	cmd.Dir = "."
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Failed to build binary: %v\n%s\n", err, output)
		os.Exit(1)
	}

	// Get absolute path to binary
	var err error
	binaryPath, err = filepath.Abs("enver-test")
	if err != nil {
		fmt.Printf("Failed to get absolute path: %v\n", err)
		os.Exit(1)
	}

	// Setup: Apply fixtures to Kind cluster
	fmt.Println("Applying test fixtures to Kind cluster...")
	if err := applyFixtures(); err != nil {
		fmt.Printf("Failed to apply fixtures: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	fmt.Println("Cleaning up...")
	cleanupFixtures()
	os.Remove("enver-test")

	os.Exit(code)
}

func applyFixtures() error {
	cmd := exec.Command("kubectl", "apply", "-f", "testdata/fixtures/resources.yaml", "--context", "kind-kind")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %v\n%s", err, output)
	}

	// Wait for resources to be ready
	cmd = exec.Command("kubectl", "wait", "--for=condition=available", "deployment/e2e-deployment",
		"-n", "enver-e2e-test", "--timeout=60s", "--context", "kind-kind")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl wait failed: %v\n%s", err, output)
	}

	return nil
}

func cleanupFixtures() {
	cmd := exec.Command("kubectl", "delete", "namespace", "enver-e2e-test", "--context", "kind-kind", "--ignore-not-found")
	cmd.Run()
}

func TestGenerateConfigMap(t *testing.T) {
	runGenerateTest(t, "configmap-only", "configmap.env")
}

func TestGenerateSecret(t *testing.T) {
	runGenerateTest(t, "secret-only", "secret.env")
}

func TestGenerateDeployment(t *testing.T) {
	runGenerateTest(t, "deployment", "deployment.env")
}

func TestGenerateStatefulSet(t *testing.T) {
	runGenerateTest(t, "statefulset", "statefulset.env")
}

func TestGenerateDaemonSet(t *testing.T) {
	runGenerateTest(t, "daemonset", "daemonset.env")
}

func TestExecuteCommand(t *testing.T) {
	// Change to testdata directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir("testdata"); err != nil {
		t.Fatalf("Failed to change to testdata directory: %v", err)
	}

	// Clean up output directory
	os.RemoveAll("output")
	defer os.RemoveAll("output")

	// Run execute command with --all
	cmd := exec.Command(binaryPath, "execute", "--all")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Execute command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify each execution produced correct output
	tests := []struct {
		name       string
		outputFile string
		goldenFile string
	}{
		{"configmap", "output/configmap.env", "golden/configmap.env"},
		{"secret", "output/secret.env", "golden/secret.env"},
		{"deployment", "output/deployment.env", "golden/deployment.env"},
		{"statefulset", "output/statefulset.env", "golden/statefulset.env"},
		{"daemonset", "output/daemonset.env", "golden/daemonset.env"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := os.ReadFile(tc.outputFile)
			if err != nil {
				t.Fatalf("Failed to read output file %s: %v", tc.outputFile, err)
			}

			if *update {
				if err := os.WriteFile(tc.goldenFile, actual, 0644); err != nil {
					t.Fatalf("Failed to update golden file: %v", err)
				}
				return
			}

			expected, err := os.ReadFile(tc.goldenFile)
			if err != nil {
				t.Fatalf("Failed to read golden file %s: %v", tc.goldenFile, err)
			}

			if !compareEnvFiles(string(expected), string(actual)) {
				t.Errorf("Output mismatch for %s\nExpected:\n%s\nActual:\n%s", tc.name, expected, actual)
			}
		})
	}
}

func TestExecuteByName(t *testing.T) {
	// Change to testdata directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir("testdata"); err != nil {
		t.Fatalf("Failed to change to testdata directory: %v", err)
	}

	// Clean up output directory
	os.RemoveAll("output")
	defer os.RemoveAll("output")

	// Run execute command with specific name
	cmd := exec.Command(binaryPath, "execute", "--name", "configmap-test")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Execute command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify output exists
	if _, err := os.Stat("output/configmap.env"); os.IsNotExist(err) {
		t.Error("Expected output/configmap.env to be created")
	}

	// Verify other outputs don't exist (only configmap-test was run)
	if _, err := os.Stat("output/secret.env"); !os.IsNotExist(err) {
		t.Error("Expected output/secret.env to not exist")
	}
}

func TestGenerateWithTransformations(t *testing.T) {
	// Change to testdata directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir("testdata"); err != nil {
		t.Fatalf("Failed to change to testdata directory: %v", err)
	}

	// Clean up output directory
	os.RemoveAll("output")
	defer os.RemoveAll("output")

	// Run generate with deployment context (has volume mounts with file transformation)
	cmd := exec.Command(binaryPath, "generate",
		"--context", "deployment",
		"--kube-context", "kind-kind",
		"--output-name", "transform-test.env",
		"--output-directory", "output")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Generate command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify file transformation created the files
	if _, err := os.Stat("output/config-volume/config.json"); os.IsNotExist(err) {
		t.Error("Expected output/config-volume/config.json to be created by file transformation")
	}

	if _, err := os.Stat("output/config-volume/settings.yaml"); os.IsNotExist(err) {
		t.Error("Expected output/config-volume/settings.yaml to be created by file transformation")
	}
}

func TestGenerateNoKubeContext(t *testing.T) {
	// Change to testdata directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir("testdata"); err != nil {
		t.Fatalf("Failed to change to testdata directory: %v", err)
	}

	// Run generate without kube-context when needed
	cmd := exec.Command(binaryPath, "generate",
		"--context", "configmap-only",
		"--output-name", "should-fail.env",
		"--output-directory", "output")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Error("Expected command to fail without kube-context")
	}
}

func runGenerateTest(t *testing.T, context, goldenFile string) {
	t.Helper()

	// Change to testdata directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir("testdata"); err != nil {
		t.Fatalf("Failed to change to testdata directory: %v", err)
	}

	// Clean up output directory
	os.RemoveAll("output")
	defer os.RemoveAll("output")

	outputFile := filepath.Join("output", goldenFile)

	// Run generate command
	cmd := exec.Command(binaryPath, "generate",
		"--context", context,
		"--kube-context", "kind-kind",
		"--output-name", goldenFile,
		"--output-directory", "output")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Generate command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Read actual output
	actual, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	goldenPath := filepath.Join("golden", goldenFile)

	// Update golden file if requested
	if *update {
		if err := os.WriteFile(goldenPath, actual, 0644); err != nil {
			t.Fatalf("Failed to update golden file: %v", err)
		}
		return
	}

	// Read expected output
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Failed to read golden file: %v", err)
	}

	// Compare
	if !compareEnvFiles(string(expected), string(actual)) {
		t.Errorf("Output mismatch\nExpected:\n%s\nActual:\n%s", expected, actual)
	}
}

// compareEnvFiles compares two env files, ignoring order of variables within sections
// since map iteration order in Go is non-deterministic
func compareEnvFiles(expected, actual string) bool {
	expectedSections := parseEnvSections(expected)
	actualSections := parseEnvSections(actual)

	if len(expectedSections) != len(actualSections) {
		return false
	}

	for i := range expectedSections {
		if expectedSections[i].header != actualSections[i].header {
			return false
		}
		if !equalSorted(expectedSections[i].vars, actualSections[i].vars) {
			return false
		}
	}

	return true
}

type envSection struct {
	header string
	vars   []string
}

func parseEnvSections(content string) []envSection {
	var sections []envSection
	var current *envSection

	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if current != nil {
				sections = append(sections, *current)
			}
			current = &envSection{header: line}
		} else if current != nil {
			current.vars = append(current.vars, line)
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}

	return sections
}

func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
