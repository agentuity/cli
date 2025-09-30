package cmd

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Run the build bundle process",
	Long: `Run the build bundle process to prepare your project for deployment.

This command bundles your project code and dependencies for deployment. You generally should not need to call this command directly as it is automatically called when you run the project.

Flags:
  --production    Bundle for production deployment
  --install       Install dependencies before bundling
  --deploy        Deploy after bundling

Examples:
  agentuity bundle --production
  agentuity bundle --install --deploy`,
	Args:    cobra.NoArgs,
	Aliases: []string{"build"},
	Hidden:  true,
	Run: func(cmd *cobra.Command, args []string) {
		started := time.Now()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		projectContext := project.EnsureProject(ctx, cmd)

		// Check for prompts evals feature flag
		if CheckFeatureFlag(cmd, FeaturePromptsEvals, "enable-prompts-evals") {
			projectContext.Logger.Info("Prompts evaluations feature is enabled")
		}

		production, _ := cmd.Flags().GetBool("production")
		install, _ := cmd.Flags().GetBool("install")
		deploy, _ := cmd.Flags().GetBool("deploy")
		ci, _ := cmd.Flags().GetBool("ci")
		tags, _ := cmd.Flags().GetStringArray("tag")
		description, _ := cmd.Flags().GetString("description")

		if err := bundler.Bundle(bundler.BundleContext{
			Context:    ctx,
			Logger:     projectContext.Logger,
			Project:    projectContext.Project,
			ProjectDir: projectContext.Dir,
			Production: production,
			Install:    install,
			CI:         ci,
			Writer:     os.Stderr,
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
			if len(tags) > 0 {
				for _, tag := range tags {
					args = append(args, "--tag", tag)
				}
			}

			// if no tags are provided, set the latest tag
			if len(tags) == 0 {
				args = append(args, "--tag", "latest")
			}

			if description != "" {
				args = append(args, "--description", description)
			}
			flags := []string{
				"log-level",
				"api-url",
				"api-key",
				"dir",
				"ci",
				"ci-remote-url",
				"ci-branch",
				"ci-commit",
				"ci-message",
				"ci-logs-url",
				"ci-git-provider",
				"ci-logs-url",
				"tag",
			}

			f := cmd.Flags()
			for _, flag := range flags {
				projectContext.Logger.Debug("flag: %s", flag)
				if f.Changed(flag) {
					switch f.Lookup(flag).Value.Type() {
					case "string":
						val, _ := f.GetString(flag)
						args = append(args, "--"+flag, val)
					case "bool":
						args = append(args, "--"+flag)
					case "stringArray":
						val, _ := f.GetStringArray(flag)
						for _, v := range val {
							projectContext.Logger.Debug("v: %s", v)
							args = append(args, "--"+flag, v)
						}
					}
				}
			}

			if ci {
				if ciMessage, _ := f.GetString("ci-message"); ciMessage != "" {
					args = append(args, "--message", ciMessage)
				}
			}

			started = time.Now()
			projectContext.Logger.Trace("deploying to cloud with %s and args: %v", bin, args)
			cwd, err := os.Getwd()
			if err != nil {
				projectContext.Logger.Fatal("Failed to get current working directory: %s", err)
			}
			c := exec.CommandContext(ctx, bin, args...)
			util.ProcessSetup(c)
			c.Dir = cwd
			c.Stdin = nil
			c.Env = os.Environ()
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				projectContext.Logger.Fatal("Failed to deploy to cloud: %s", err)
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
	bundleCmd.Flags().StringArray("tag", nil, "Tag(s) to associate with this deployment (can be specified multiple times)")
	bundleCmd.Flags().String("description", "", "Used to set the description of the deployment")
	bundleCmd.Flags().MarkHidden("deploymentId")
	bundleCmd.Flags().Bool("ci", false, "Used to track a specific CI job")
	bundleCmd.Flags().MarkHidden("ci")
	bundleCmd.Flags().String("ci-remote-url", "", "Used to set the remote repository URL for your deployment metadata")
	bundleCmd.Flags().String("ci-branch", "", "Used to set the branch name for your deployment metadata")
	bundleCmd.Flags().String("ci-commit", "", "Used to set the commit hash for your deployment metadata")
	bundleCmd.Flags().String("ci-message", "", "Used to set the commit message for your deployment metadata")
	bundleCmd.Flags().String("ci-git-provider", "", "Used to set the git provider for your deployment metadata")
	bundleCmd.Flags().String("ci-logs-url", "", "Used to set the CI logs URL for your deployment metadata")

	bundleCmd.Flags().MarkHidden("ci-remote-url")
	bundleCmd.Flags().MarkHidden("ci-branch")
	bundleCmd.Flags().MarkHidden("ci-commit")
	bundleCmd.Flags().MarkHidden("ci-message")
	bundleCmd.Flags().MarkHidden("ci-git-provider")
	bundleCmd.Flags().MarkHidden("ci-logs-url")
}
