package cmd

import (
	"context"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:    "bundle",
	Short:  "Run the build bundle process",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		started := time.Now()
		projectContext := ensureProject(cmd)
		production, _ := cmd.Flags().GetBool("production")
		if err := bundler.Bundle(bundler.BundleContext{
			Context:    context.Background(),
			Logger:     projectContext.Logger,
			ProjectDir: projectContext.Dir,
			Production: production,
		}); err != nil {
			projectContext.Logger.Fatal("%s", err)
		}
		projectContext.Logger.Debug("bundled in %s", time.Since(started))
	},
}

func init() {
	bundler.Version = Version
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("dir", "d", ".", "The directory to the project")
	bundleCmd.Flags().BoolP("production", "p", false, "Whether to bundle for production")
}
