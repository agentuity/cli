package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Cloud related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("deploy called")
	},
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
}
