package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Source struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Type      string `yaml:"type"`
}

type Config struct {
	KubeContexts []string `yaml:"kube-contexts"`
	Sources      []Source `yaml:"sources"`
}

var kubeContext string
var outputPath string

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

		// Collect all env vars with their source info
		type envEntry struct {
			key       string
			value     string
			sourceType string
			name      string
			namespace string
		}
		var envData []envEntry

		// Get each source and collect its data
		for _, source := range config.Sources {
			namespace := source.Namespace
			if namespace == "" {
				namespace = "default"
			}

			sourceType := source.Type
			if sourceType == "" {
				return fmt.Errorf("type is required for source %q in namespace %q", source.Name, namespace)
			}

			switch sourceType {
			case "ConfigMap":
				cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("failed to get configmap %s/%s: %w", namespace, source.Name, err)
				}
				for key, value := range cm.Data {
					if value != "" {
						envData = append(envData, envEntry{
							key:        key,
							value:      value,
							sourceType: sourceType,
							name:       source.Name,
							namespace:  namespace,
						})
					}
				}
			case "Secret":
				secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), source.Name, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("failed to get secret %s/%s: %w", namespace, source.Name, err)
				}
				for key, value := range secret.Data {
					if len(value) > 0 {
						envData = append(envData, envEntry{
							key:        key,
							value:      strings.TrimRight(string(value), "\n\r"),
							sourceType: sourceType,
							name:       source.Name,
							namespace:  namespace,
						})
					}
				}
			default:
				return fmt.Errorf("unknown source type %q for %s/%s", sourceType, namespace, source.Name)
			}
		}

		// Create output directory if it doesn't exist
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		// Write to output file with comments
		output := ""
		for _, entry := range envData {
			output += fmt.Sprintf("# %s %s/%s\n", entry.sourceType, entry.namespace, entry.name)
			output += fmt.Sprintf("%s=%s\n", entry.key, entry.value)
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
	rootCmd.AddCommand(readCmd)
}
