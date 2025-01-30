package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var devRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("dev called")
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devRunCmd)
}
