package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of the Agentuity CLI",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Agentuity CLI")
		fmt.Println("Version: " + Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
