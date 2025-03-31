package cmd

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	Version string = "dev"
	Commit  string = "dev"
	Date    string
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "agentuity",
	Long: `Agentuity CLI is a command-line tool for building, managing, and deploying AI agents.

Use the various commands to create projects, manage agents, set environment variables,
and deploy your agents to the Agentuity Cloud Platform.

Run 'agentuity help <command>' for more information about a specific command.`,
	Run: func(cmd *cobra.Command, args []string) {
		if version, _ := cmd.Flags().GetBool("version"); version {
			fmt.Println(Version)
			return
		}
		tui.Logo()
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func loadTemplates(ctx context.Context, cmd *cobra.Command) templates.Templates {
	tmplDir, err := getConfigTemplateDir(cmd)
	if err != nil {
		errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
	}

	var tmpls templates.Templates

	tui.ShowSpinner("Loading templates...", func() {
		tmpls, err = templates.LoadTemplates(ctx, tmplDir)
		if err != nil {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates")).ShowErrorAndExit()
		}

		if len(tmpls) == 0 {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("No templates returned from load templates")).ShowErrorAndExit()
		}
	})
	return tmpls
}

func init() {

	// NOTE: this is not a persistent flag is hidden but since it's a unix default for most
	// commands its a natural flag to expect
	rootCmd.Flags().BoolP("version", "v", false, "print out the version")
	rootCmd.Flags().MarkHidden("version")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/agentuity/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "The log level to use")

	rootCmd.PersistentFlags().String("app-url", "https://app.agentuity.com", "The base url of the Agentuity Console app")
	rootCmd.PersistentFlags().MarkHidden("app-url")
	viper.BindPFlag("overrides.app_url", rootCmd.PersistentFlags().Lookup("app-url"))

	rootCmd.PersistentFlags().String("api-url", "https://api.agentuity.com", "The base url of the Agentuity API")
	rootCmd.PersistentFlags().MarkHidden("api-url")
	viper.BindPFlag("overrides.api_url", rootCmd.PersistentFlags().Lookup("api-url"))

	rootCmd.PersistentFlags().String("websocket-url", "wss://api.agentuity.com", "The base url of the Agentuity WebSocket API")
	rootCmd.PersistentFlags().MarkHidden("websocket-url")
	viper.BindPFlag("overrides.websocket_url", rootCmd.PersistentFlags().Lookup("websocket-url"))

	rootCmd.PersistentFlags().String("transport-url", "https://agentuity.ai", "The base url of the Agentuity Transport API")
	rootCmd.PersistentFlags().MarkHidden("transport-url")
	viper.BindPFlag("overrides.transport_url", rootCmd.PersistentFlags().Lookup("transport-url"))

	rootCmd.PersistentFlags().String("api-key", "", "The API key to use for authentication")
	rootCmd.PersistentFlags().MarkHidden("api-key")
	viper.BindPFlag("auth.api_key", rootCmd.PersistentFlags().Lookup("api-key"))

	viper.SetDefault("overrides.app_url", "https://app.agentuity.com")
	viper.SetDefault("overrides.api_url", "https://api.agentuity.com")
	viper.SetDefault("overrides.transport_url", "https://agentuity.ai")

	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		dir := filepath.Join(home, ".config", "agentuity")
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Fatalf("failed to create config directory (%s): %s", dir, err)
			}
		}
		cfgFile = filepath.Join(dir, "config.yaml")
		cfgFile = getProfile()
		viper.SetConfigFile(cfgFile)
	}

	viper.AutomaticEnv() // read in environment variables that match

	// Finally read the config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(*fs.PathError); !ok {
			log.Fatalf("Error reading config file: %s\n", err)
		}
	}
}

func maxString(val string, max int) string {
	if len(val) > max {
		return val[:max] + "..."
	}
	return val
}

func initScreenWithLogo() {
	tui.ClearScreen()
	tui.Logo()
}

func createPromptHelper() deployer.PromptHelpers {
	return deployer.PromptHelpers{
		ShowSpinner:   tui.ShowSpinner,
		PrintSuccess:  tui.ShowSuccess,
		CommandString: tui.Command,
		LinkString:    tui.Link,
		PrintLock:     tui.ShowLock,
		PrintWarning:  tui.ShowWarning,
		Ask:           tui.Ask,
		PromptForEnv:  promptForEnv,
	}
}

func isCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func checkForUpgrade(ctx context.Context, logger logger.Logger) {
	v := viper.GetInt64("preferences.last_update_check")
	var check bool
	if v == 0 {
		check = true
	} else {
		n := time.Unix(v, 0)
		if time.Since(n) >= 24*time.Hour {
			check = true
		}
	}
	if check {
		viper.Set("preferences.last_update_check", time.Now().Unix())
		viper.WriteConfig()
		util.CheckLatestRelease(ctx, logger)
	}
}

func getAgentuityCommand() string {
	exe, _ := os.Executable()
	if !strings.Contains(exe, "agentuity") {
		exe, _ = exec.LookPath("agentuity")
	}
	return exe
}
