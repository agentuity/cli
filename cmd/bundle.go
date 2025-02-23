package cmd

import (
	"github.com/agentuity/cli/internal/provider"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:    "bundle",
	Short:  "Run the build bundle process",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		projectContext := ensureProject(cmd)
		production, _ := cmd.Flags().GetBool("production")
		if err := provider.BundleJS(projectContext.Logger, projectContext.Project, projectContext.Dir, production); err != nil {
			projectContext.Logger.Fatal("failed to bundle JS: %s", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("dir", "d", ".", "The directory to the project")
	bundleCmd.Flags().BoolP("production", "p", false, "Whether to bundle for production")
}
