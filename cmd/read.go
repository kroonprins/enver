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
	KubeContexts []string         `yaml:"kube-contexts"`
	Contexts     []string         `yaml:"contexts"`
	Sources      []sources.Source `yaml:"sources"`
}

var kubeContext string
var outputPath string
var contextFlags []string

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read data from ConfigMaps and Secrets",
	Long:  `Reads the .enver.yaml file, selects a kubectl context, and writes all data from ConfigMaps and Secrets defined in sources to a .env file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := os.ReadFile(".enver.yaml")
		if err != nil {
			return fmt.Errorf("failed to read .enver.yaml: %w", err)
		}

		var config Config
		if err := yaml.Unmarshal(content, &config); err != nil {
			return fmt.Errorf("failed to parse .enver.yaml: %w", err)
		}

		if len(config.KubeContexts) == 0 {
			return fmt.Errorf("no kube-contexts found in .enver.yaml")
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

		selected := kubeContext
		if selected == "" {
			prompt := promptui.Select{
				Label: "Select kubectl context",
				Items: config.KubeContexts,
			}

			_, selected, err = prompt.Run()
			if err != nil {
				return fmt.Errorf("selection failed: %w", err)
			}
		}

		// Build kubeconfig path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		kubeconfigPath := filepath.Join(homeDir, ".kube", "config")

		// Load kubeconfig with the selected context
		kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
			&clientcmd.ConfigOverrides{CurrentContext: selected},
		).ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to load kubeconfig: %w", err)
		}

		// Create Kubernetes client
		clientset, err := kubernetes.NewForConfig(kubeconfig)
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", err)
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
		for _, source := range config.Sources {
			// Check if source should be included based on contexts
			if !source.ShouldInclude(selectedContexts) {
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
	readCmd.Flags().StringVar(&kubeContext, "kube-context", "", "kubectl context to use (prompts if not provided)")
	readCmd.Flags().StringVarP(&outputPath, "output", "o", "generated/.env", "output file path for the .env file")
	readCmd.Flags().StringArrayVarP(&contextFlags, "context", "c", []string{}, "context for filtering sources (can be repeated, prompts if not provided and contexts are defined)")
	rootCmd.AddCommand(readCmd)
}
