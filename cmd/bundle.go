package cmd

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:     "bundle",
	Short:   "Run the build bundle process",
	Args:    cobra.NoArgs,
	Aliases: []string{"build"},
	Hidden:  true,
	Run: func(cmd *cobra.Command, args []string) {
		started := time.Now()
		projectContext := project.EnsureProject(cmd)
		production, _ := cmd.Flags().GetBool("production")
		install, _ := cmd.Flags().GetBool("install")
		deploy, _ := cmd.Flags().GetBool("deploy")
		if err := bundler.Bundle(bundler.BundleContext{
			Context:    context.Background(),
			Logger:     projectContext.Logger,
			ProjectDir: projectContext.Dir,
			Production: production,
			Install:    install,
		}); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to bundle project")).ShowErrorAndExit()
		}
		if !deploy {
			projectContext.Logger.Debug("bundled in %s", time.Since(started))
			return
		}
		if deploy {
			projectContext.Logger.Info("bundled in %s", time.Since(started))
			bin, err := os.Executable()
			if err != nil {
				bin = os.Args[0]
				projectContext.Logger.Error("Failed to get executable path: %s. using %s", err, bin)
			}
			deploymentId, _ := cmd.Flags().GetString("deploymentId")
			args := []string{"cloud", "deploy"}
			if deploymentId != "" {
				args = append(args, "--deploymentId", deploymentId)
			}
			flags := []string{"log-level", "api-url", "api-key"}
			for _, flag := range flags {
				if cmd.Flags().Changed(flag) {
					level, _ := cmd.Flags().GetString(flag)
					args = append(args, "--"+flag, level)
				}
			}
			started = time.Now()
			projectContext.Logger.Trace("deploying to cloud with args: %v", args)
			c := exec.Command(bin, args...)
			c.Stdin = nil
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				projectContext.Logger.Error("Failed to deploy to cloud: %s", err)
				os.Exit(1)
			}
			projectContext.Logger.Info("deployment completed in %s", time.Since(started))
		}
	},
}

func init() {
	bundler.Version = Version
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("dir", "d", ".", "The directory to the project")
	bundleCmd.Flags().BoolP("production", "p", false, "Whether to bundle for production")
	bundleCmd.Flags().BoolP("install", "i", false, "Whether to install dependencies before bundling")
	bundleCmd.Flags().Bool("deploy", false, "Whether to deploy after bundling")
	bundleCmd.Flags().String("deploymentId", "", "Used to track a specific deployment")
	bundleCmd.Flags().MarkHidden("deploymentId")
}
