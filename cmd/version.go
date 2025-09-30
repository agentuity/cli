package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of the Agentuity CLI",
	Long: `Print the version of the Agentuity CLI.

Flags:
  --long    Print the long version including commit hash and build date

Examples:
  agentuity version
  agentuity version --long`,
	Run: func(cmd *cobra.Command, args []string) {
		long, _ := cmd.Flags().GetBool("long")
		if long {
			fmt.Println("Version: " + Version)
			fmt.Println("Commit: " + Commit)
			fmt.Println("Date: " + Date)

			// Example of using feature flags
			if IsPromptsEvalsEnabled() {
				fmt.Println("Prompts Evals: Enabled")
			}
		} else {
			fmt.Println(Version)
		}
	},
}

var versionCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check the latest version of the Agentuity CLI",
	Long: `Check the latest version of the Agentuity CLI.

Examples:
  agentuity version check
  agentuity version check --upgrade`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		upgrade, _ := cmd.Flags().GetBool("upgrade")
		if Version == "dev" {
			tui.ShowWarning("You are using the development version of the Agentuity CLI.")
			return
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if upgrade {
			util.CheckLatestRelease(ctx, logger, true)
		} else {
			release, err := util.GetLatestRelease(ctx)
			if err != nil {
				errsystem.New(errsystem.ErrUpgradeCli, err).ShowErrorAndExit()
			}
			latestVersion := semver.MustParse(release)
			currentVersion := semver.MustParse(Version)
			if latestVersion.GreaterThan(currentVersion) {
				tui.ShowWarning("A new version (%s) of the Agentuity CLI is available. Please upgrade to the latest version.", release)
			} else {
				tui.ShowSuccess("You are using the latest version (%s) of the Agentuity CLI.", release)
			}
		}
	},
}

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"update"},
	Short:   "Upgrade the Agentuity CLI to the latest version",
	Long: func() string {
		baseDesc := `Upgrade the Agentuity CLI to the latest version.

This command will check for the latest version of the Agentuity CLI and upgrade if a newer version is available.
It will create a backup of the current binary before installing the new version.`

		if runtime.GOOS == "darwin" {
			baseDesc += `

On macOS, if the CLI was installed using Homebrew, it will use Homebrew to upgrade.`
		}

		baseDesc += `

Examples:
  agentuity version upgrade
  agentuity version upgrade --force`

		return baseDesc
	}(),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		force, _ := cmd.Flags().GetBool("force")
		if Version == "dev" || strings.HasSuffix(Version, "-next") {
			tui.ShowWarning("You are using the development version of the Agentuity CLI which cannot be upgraded.")
			return
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if err := util.UpgradeCLI(ctx, logger, force); err != nil {
			errsystem.New(errsystem.ErrUpgradeCli, err, errsystem.WithAttributes(map[string]any{"version": Version})).ShowErrorAndExit()
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
	versionCmd.AddCommand(versionCheckCmd)
	versionCmd.AddCommand(upgradeCmd)
	versionCmd.Flags().Bool("long", false, "Print the long version")
	versionCheckCmd.Flags().Bool("upgrade", false, "Upgrade to the latest version if possible")
	upgradeCmd.Flags().Bool("force", false, "Force upgrade even if already on the latest version")
}
