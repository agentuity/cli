package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of the Agentuity CLI",
	Run: func(cmd *cobra.Command, args []string) {
		long, _ := cmd.Flags().GetBool("long")
		if long {
			fmt.Println("Version: " + Version)
			fmt.Println("Commit: " + Commit)
			fmt.Println("Date: " + Date)
		} else {
			fmt.Println(Version)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().Bool("long", false, "Print the long version")
}
