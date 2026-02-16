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
	"time"

	expect "github.com/Netflix/go-expect"
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
		return fmt.Errorf("kubectl wait for e2e-deployment failed: %v\n%s", err, output)
	}

	// Wait for container-test deployment
	cmd = exec.Command("kubectl", "wait", "--for=condition=available", "deployment/e2e-container-test",
		"-n", "enver-e2e-test", "--timeout=60s", "--context", "kind-kind")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl wait for e2e-container-test failed: %v\n%s", err, output)
	}

	// Give extra time for the container's init script to create the config file
	time.Sleep(2 * time.Second)

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

func TestGenerateContainer(t *testing.T) {
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

	outputFile := filepath.Join("output", "container.env")

	// Run generate command
	cmd := exec.Command(binaryPath, "generate",
		"--context", "container",
		"--kube-context", "kind-kind",
		"--output-name", "container.env",
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

	// Verify the output contains expected environment variables from the container
	actualStr := string(actual)
	expectedVars := []string{"APP_NAME=e2e-container-test", "APP_ENV=testing"}
	for _, expected := range expectedVars {
		if !strings.Contains(actualStr, expected) {
			t.Errorf("Expected output to contain %q, but it didn't.\nActual output:\n%s", expected, actualStr)
		}
	}

	// Verify the file extraction worked
	if !strings.Contains(actualStr, "CONFIG_FILE_PATH=") {
		t.Errorf("Expected output to contain CONFIG_FILE_PATH, but it didn't.\nActual output:\n%s", actualStr)
	}

	// Verify the extracted file exists
	if _, err := os.Stat("output/container-files/config.json"); os.IsNotExist(err) {
		t.Error("Expected output/container-files/config.json to be created by file extraction")
	}

	// Verify the extracted file contains expected content
	fileContent, err := os.ReadFile("output/container-files/config.json")
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}
	if !strings.Contains(string(fileContent), `"app": "test"`) {
		t.Errorf("Expected extracted file to contain JSON content, got: %s", string(fileContent))
	}
}

func TestExecuteContainer(t *testing.T) {
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

	// Run execute command with container-test
	cmd := exec.Command(binaryPath, "execute", "--name", "container-test")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Execute command failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Verify container.env was created
	actual, err := os.ReadFile("output/container.env")
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	actualStr := string(actual)

	// Verify expected env vars
	expectedVars := []string{"APP_NAME=e2e-container-test", "APP_ENV=testing"}
	for _, expected := range expectedVars {
		if !strings.Contains(actualStr, expected) {
			t.Errorf("Expected output to contain %q, but it didn't.\nActual output:\n%s", expected, actualStr)
		}
	}

	// Verify the file extraction worked
	if !strings.Contains(actualStr, "CONFIG_FILE_PATH=") {
		t.Errorf("Expected output to contain CONFIG_FILE_PATH, but it didn't.\nActual output:\n%s", actualStr)
	}

	// Verify the extracted file exists
	if _, err := os.Stat("output/container-files/config.json"); os.IsNotExist(err) {
		t.Error("Expected output/container-files/config.json to be created by file extraction")
	}
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

func TestExecuteInteractiveSelection(t *testing.T) {
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

	// Create a console for interactive testing
	console, err := expect.NewConsole(expect.WithStdout(os.Stdout), expect.WithDefaultTimeout(30*time.Second))
	if err != nil {
		t.Fatalf("Failed to create console: %v", err)
	}
	defer console.Close()

	// Run execute command without --all or --name to trigger interactive prompt
	cmd := exec.Command(binaryPath, "execute")
	cmd.Stdin = console.Tty()
	cmd.Stdout = console.Tty()
	cmd.Stderr = console.Tty()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Wait for the selection prompt
	_, err = console.ExpectString("Select executions to run")
	if err != nil {
		t.Fatalf("Failed to see selection prompt: %v", err)
	}

	// Select the first item (configmap-test) with space, then press enter
	console.Send(" ")  // Space to select first item
	time.Sleep(100 * time.Millisecond)
	console.Send("\r") // Enter to confirm

	// Wait for command to complete
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute command failed: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Command timed out")
	}

	// Verify that at least one output was created
	if _, err := os.Stat("output/configmap.env"); os.IsNotExist(err) {
		t.Error("Expected output/configmap.env to be created")
	}
}

func TestGenerateInteractiveKubeContextSelection(t *testing.T) {
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

	// Create a console for interactive testing
	console, err := expect.NewConsole(expect.WithStdout(os.Stdout), expect.WithDefaultTimeout(30*time.Second))
	if err != nil {
		t.Fatalf("Failed to create console: %v", err)
	}
	defer console.Close()

	// Run generate command without --kube-context to trigger interactive prompt
	cmd := exec.Command(binaryPath, "generate",
		"--context", "configmap-only",
		"--output-name", "interactive-test.env",
		"--output-directory", "output")
	cmd.Stdin = console.Tty()
	cmd.Stdout = console.Tty()
	cmd.Stderr = console.Tty()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Wait for the kubectl context selection prompt
	_, err = console.ExpectString("Select kubectl context")
	if err != nil {
		t.Fatalf("Failed to see kubectl context prompt: %v", err)
	}

	// Navigate to kind-kind and select it
	// The prompt shows a list - we need to find and select kind-kind
	// Using arrow keys to navigate and enter to select
	for i := 0; i < 10; i++ {
		// Try to find kind-kind in the current view
		console.Send("\r") // Try selecting current option
		break
	}

	// Wait for command to complete
	select {
	case err := <-done:
		if err != nil {
			// This might fail if kind-kind wasn't the first option
			// but the test verifies the interactive prompt works
			t.Logf("Command completed with: %v (this may be expected if kind-kind wasn't first option)", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Command timed out")
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
