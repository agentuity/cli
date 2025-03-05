package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/go-common/logger"
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

	viper.SetDefault("overrides.app_url", "https://app.agentuity.com")
	viper.SetDefault("overrides.api_url", "https://api.agentuity.com")

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

func resolveProjectDir(cmd *cobra.Command) string {
	cwd, err := os.Getwd()
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage(fmt.Sprintf("Failed to get current directory: %s", err))).ShowErrorAndExit()
	}
	dir := cwd
	dirFlag, _ := cmd.Flags().GetString("dir")
	if dirFlag != "" {
		dir = dirFlag
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage(fmt.Sprintf("Failed to get absolute path: %s", err))).ShowErrorAndExit()
	}
	if !project.ProjectExists(abs) {
		errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("no agentuity.yaml file found"),
			errsystem.WithUserMessage("No agentuity.yaml file found in the current directory")).ShowErrorAndExit()
	}
	return abs
}

func getURLs(logger logger.Logger) (string, string) {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.div" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "http://localhost:3000"
	}
	return apiUrl, appUrl
}
