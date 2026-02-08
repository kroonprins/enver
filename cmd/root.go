package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "enver",
	Short: "A tool for managing environment configuration",
	Long:  `Enver is a CLI tool for reading and managing .enver.yaml configuration files.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
