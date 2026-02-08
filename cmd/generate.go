package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"enver/sources"

	"github.com/AlecAivazis/survey/v2"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	Contexts []string         `yaml:"contexts"`
	Sources  []sources.Source `yaml:"sources"`
}

var kubeContext string
var outputPath string
var contextFlags []string

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate .env file from ConfigMaps, Secrets and EnvFiles",
	Long:  `Reads the .enver.yaml file, selects a kubectl context if needed, and generates a .env file from ConfigMaps, Secrets and EnvFiles defined in sources.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := os.ReadFile(".enver.yaml")
		if err != nil {
			return fmt.Errorf("failed to read .enver.yaml: %w", err)
		}

		var config Config
		if err := yaml.Unmarshal(content, &config); err != nil {
			return fmt.Errorf("failed to parse .enver.yaml: %w", err)
		}

		if len(config.Sources) == 0 {
			return fmt.Errorf("no sources found in .enver.yaml")
		}

		// Select contexts for filtering sources
		selectedContexts := contextFlags
		if len(selectedContexts) == 0 && len(config.Contexts) > 0 {
			prompt := &survey.MultiSelect{
				Message: "Select contexts (press Enter for none, Space to select):",
				Options: config.Contexts,
			}

			err := survey.AskOne(prompt, &selectedContexts)
			if err != nil {
				return fmt.Errorf("context selection failed: %w", err)
			}
		}

		// Filter sources based on selected contexts and check if any require Kubernetes
		var filteredSources []sources.Source
		needsKubernetes := false
		for _, source := range config.Sources {
			if !source.ShouldInclude(selectedContexts) {
				continue
			}
			filteredSources = append(filteredSources, source)
			if source.Type == "ConfigMap" || source.Type == "Secret" {
				needsKubernetes = true
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
			selectedKubeContext := kubeContext
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

		// Collect all env vars with their source info
		var envData []sources.EnvEntry

		// Get each source and collect its data
		for _, source := range filteredSources {
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
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		// Write to output file with comments
		output := ""
		for _, entry := range envData {
			output += fmt.Sprintf("# %s %s/%s\n", entry.SourceType, entry.Namespace, entry.Name)
			output += fmt.Sprintf("%s=%s\n", entry.Key, entry.Value)
		}
		if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

		fmt.Printf("Wrote %d environment variables to %s\n", len(envData), outputPath)
		return nil
	},
}

func init() {
	generateCmd.Flags().StringVar(&kubeContext, "kube-context", "", "kubectl context to use (prompts if needed and not provided)")
	generateCmd.Flags().StringVarP(&outputPath, "output", "o", "generated/.env", "output file path for the .env file")
	generateCmd.Flags().StringArrayVarP(&contextFlags, "context", "c", []string{}, "context for filtering sources (can be repeated, prompts if not provided and contexts are defined)")
	rootCmd.AddCommand(generateCmd)
}
