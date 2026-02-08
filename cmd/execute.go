package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"enver/sources"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Execution struct {
	Name     string   `yaml:"name"`
	Output   string   `yaml:"output"`
	Contexts []string `yaml:"contexts"`
}

type ExecuteConfig struct {
	Contexts   []string         `yaml:"contexts"`
	Sources    []sources.Source `yaml:"sources"`
	Executions []Execution      `yaml:"executions"`
}

var executeKubeContext string

var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute predefined .env generation tasks",
	Long:  `Reads the .enver.yaml file and executes all predefined generation tasks defined in the executions field.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := os.ReadFile(".enver.yaml")
		if err != nil {
			return fmt.Errorf("failed to read .enver.yaml: %w", err)
		}

		var config ExecuteConfig
		if err := yaml.Unmarshal(content, &config); err != nil {
			return fmt.Errorf("failed to parse .enver.yaml: %w", err)
		}

		if len(config.Executions) == 0 {
			return fmt.Errorf("no executions found in .enver.yaml")
		}

		if len(config.Sources) == 0 {
			return fmt.Errorf("no sources found in .enver.yaml")
		}

		// Check if any execution needs Kubernetes
		needsKubernetes := false
		for _, execution := range config.Executions {
			for _, source := range config.Sources {
				if !source.ShouldInclude(execution.Contexts) {
					continue
				}
				if source.Type == "ConfigMap" || source.Type == "Secret" {
					needsKubernetes = true
					break
				}
			}
			if needsKubernetes {
				break
			}
		}

		// Build kubeconfig path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

		var clientset *kubernetes.Clientset

		// Only set up Kubernetes client if needed
		if needsKubernetes {
			selectedKubeContext := executeKubeContext
			if selectedKubeContext == "" {
				// Load kubeconfig to get available contexts
				kubeConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
				if err != nil {
					return fmt.Errorf("failed to load kubeconfig: %w", err)
				}

				// Get list of context names
				var contextNames []string
				for name := range kubeConfig.Contexts {
					contextNames = append(contextNames, name)
				}

				if len(contextNames) == 0 {
					return fmt.Errorf("no kubectl contexts found in kubeconfig")
				}

				prompt := promptui.Select{
					Label: "Select kubectl context",
					Items: contextNames,
				}

				_, selectedKubeContext, err = prompt.Run()
				if err != nil {
					return fmt.Errorf("kubectl context selection failed: %w", err)
				}
			}

			// Load kubeconfig with the selected context
			restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
				&clientcmd.ConfigOverrides{CurrentContext: selectedKubeContext},
			).ClientConfig()
			if err != nil {
				return fmt.Errorf("failed to load kubeconfig: %w", err)
			}

			// Create Kubernetes client
			clientset, err = kubernetes.NewForConfig(restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}
		}

		// Map of source types to their fetchers
		fetchers := map[string]sources.Fetcher{
			"ConfigMap": &sources.ConfigMapFetcher{},
			"Secret":    &sources.SecretFetcher{},
			"EnvFile":   &sources.EnvFileFetcher{},
		}

		// Execute each execution
		for _, execution := range config.Executions {
			fmt.Printf("Executing: %s\n", execution.Name)

			// Collect all env vars with their source info
			var envData []sources.EnvEntry

			// Get each source and collect its data
			for _, source := range config.Sources {
				// Check if source should be included based on contexts
				if !source.ShouldInclude(execution.Contexts) {
					continue
				}

				namespace := source.Namespace
				if namespace == "" {
					namespace = "default"
				}

				if source.Type == "" {
					return fmt.Errorf("type is required for source %q in namespace %q", source.Name, namespace)
				}

				fetcher, ok := fetchers[source.Type]
				if !ok {
					return fmt.Errorf("unknown source type %q for %s/%s", source.Type, namespace, source.Name)
				}

				entries, err := fetcher.Fetch(clientset, source, namespace)
				if err != nil {
					return err
				}

				envData = append(envData, entries...)
			}

			// Create output directory if it doesn't exist
			outputDir := filepath.Dir(execution.Output)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Write to output file with comments
			output := ""
			for _, entry := range envData {
				output += fmt.Sprintf("# %s %s/%s\n", entry.SourceType, entry.Namespace, entry.Name)
				output += fmt.Sprintf("%s=%s\n", entry.Key, entry.Value)
			}
			if err := os.WriteFile(execution.Output, []byte(output), 0644); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}

			fmt.Printf("  Wrote %d environment variables to %s\n", len(envData), execution.Output)
		}

		return nil
	},
}

func init() {
	executeCmd.Flags().StringVar(&executeKubeContext, "kube-context", "", "kubectl context to use (prompts if needed and not provided)")
	rootCmd.AddCommand(executeCmd)
}
