package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Masterminds/semver"
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
		upgrade, _ := cmd.Flags().GetBool("upgrade")
		if Version == "dev" {
			tui.ShowWarning("You are using the development version of the Agentuity CLI.")
			return
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if upgrade {
			util.CheckLatestRelease(ctx)
		} else {
			release, err := util.GetLatestRelease(ctx)
			if err != nil {
				logger := env.NewLogger(cmd)
				logger.Fatal("%s", err)
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

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.AddCommand(versionCheckCmd)
	versionCmd.Flags().Bool("long", false, "Print the long version")
	versionCheckCmd.Flags().Bool("upgrade", false, "Upgrade to the latest version if possible")
}
