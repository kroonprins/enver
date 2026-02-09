package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"enver/sources"

	"github.com/AlecAivazis/survey/v2"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Execution struct {
	Name        string   `yaml:"name"`
	Output      string   `yaml:"output"`
	Contexts    []string `yaml:"contexts"`
	KubeContext string   `yaml:"kube-context"`
}

type ExecuteConfig struct {
	Contexts   []string         `yaml:"contexts"`
	Sources    []sources.Source `yaml:"sources"`
	Executions []Execution      `yaml:"executions"`
}

var executeKubeContext string
var executeNames []string
var executeAll bool

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

		// Determine which executions to run
		var selectedExecutions []Execution

		if executeAll {
			// Run all executions
			selectedExecutions = config.Executions
		} else if len(executeNames) > 0 {
			// Run specified executions
			executionMap := make(map[string]Execution)
			for _, exec := range config.Executions {
				executionMap[exec.Name] = exec
			}

			for _, name := range executeNames {
				exec, ok := executionMap[name]
				if !ok {
					return fmt.Errorf("execution %q not found in .enver.yaml", name)
				}
				selectedExecutions = append(selectedExecutions, exec)
			}
		} else {
			// Prompt user to select executions
			var executionNames []string
			for _, exec := range config.Executions {
				executionNames = append(executionNames, exec.Name)
			}

			var selectedNames []string
			prompt := &survey.MultiSelect{
				Message: "Select executions to run:",
				Options: executionNames,
			}

			err := survey.AskOne(prompt, &selectedNames)
			if err != nil {
				return fmt.Errorf("execution selection failed: %w", err)
			}

			if len(selectedNames) == 0 {
				return fmt.Errorf("no executions selected")
			}

			executionMap := make(map[string]Execution)
			for _, exec := range config.Executions {
				executionMap[exec.Name] = exec
			}

			for _, name := range selectedNames {
				selectedExecutions = append(selectedExecutions, executionMap[name])
			}
		}

		// Build kubeconfig path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

		// Map of source types to their fetchers
		fetchers := map[string]sources.Fetcher{
			"ConfigMap": &sources.ConfigMapFetcher{},
			"Secret":    &sources.SecretFetcher{},
			"EnvFile":   &sources.EnvFileFetcher{},
		}

		// Cache for kubernetes clients by context
		clientCache := make(map[string]*kubernetes.Clientset)

		// Execute each selected execution
		for _, execution := range selectedExecutions {
			fmt.Printf("Executing: %s\n", execution.Name)

			// Check if this execution needs Kubernetes
			executionNeedsKubernetes := false
			for _, source := range config.Sources {
				if !source.ShouldInclude(execution.Contexts) {
					continue
				}
				if source.Type == "ConfigMap" || source.Type == "Secret" {
					executionNeedsKubernetes = true
					break
				}
			}

			var clientset *kubernetes.Clientset

			if executionNeedsKubernetes {
				// Determine kube-context: execution's kube-context > CLI flag > prompt
				selectedKubeContext := execution.KubeContext
				if selectedKubeContext == "" {
					selectedKubeContext = executeKubeContext
				}
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
						Label: fmt.Sprintf("Select kubectl context for execution %q", execution.Name),
						Items: contextNames,
					}

					_, selectedKubeContext, err = prompt.Run()
					if err != nil {
						return fmt.Errorf("kubectl context selection failed: %w", err)
					}
				}

				// Check cache first
				if cached, ok := clientCache[selectedKubeContext]; ok {
					clientset = cached
				} else {
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

					// Cache it
					clientCache[selectedKubeContext] = clientset
				}
			}

			// Collect all env vars with their source info
			var envData []sources.EnvEntry

			// Get each source and collect its data
			for _, source := range config.Sources {
				// Check if source should be included based on contexts
				if !source.ShouldInclude(execution.Contexts) {
					continue
				}

				if source.Type == "" {
					return fmt.Errorf("type is required for source %q in namespace %q", source.Name, source.GetNamespace())
				}

				fetcher, ok := fetchers[source.Type]
				if !ok {
					return fmt.Errorf("unknown source type %q for %s/%s", source.Type, source.GetNamespace(), source.Name)
				}

				entries, err := fetcher.Fetch(clientset, source)
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
			var sb strings.Builder
			for _, entry := range envData {
				sb.WriteString(fmt.Sprintf("# %s %s/%s\n", entry.SourceType, entry.Namespace, entry.Name))
				sb.WriteString(fmt.Sprintf("%s=%s\n", entry.Key, entry.Value))
			}
			if err := os.WriteFile(execution.Output, []byte(sb.String()), 0644); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}

			fmt.Printf("  Wrote %d environment variables to %s\n", len(envData), execution.Output)
		}

		return nil
	},
}

func init() {
	executeCmd.Flags().StringVar(&executeKubeContext, "kube-context", "", "kubectl context to use (overrides execution's kube-context)")
	executeCmd.Flags().StringArrayVar(&executeNames, "name", []string{}, "execution name to run (can be repeated)")
	executeCmd.Flags().BoolVar(&executeAll, "all", false, "run all executions")
	rootCmd.AddCommand(executeCmd)
}
