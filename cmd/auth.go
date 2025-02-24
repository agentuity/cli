package cmd

import (
	"github.com/agentuity/cli/internal/auth"
	"github.com/agentuity/go-common/env"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication and authorization related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to the Agentuity Cloud Platform",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		appUrl := viper.GetString("overrides.app_url")
		apiUrl := viper.GetString("overrides.api_url")
		if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
			logger.Debug("switching app url to production since the api url is production")
			appUrl = "https://app.agentuity.com"
		} else if apiUrl == "https://api.agentuity.div" && appUrl == "https://app.agentuity.com" {
			logger.Debug("switching app url to dev since the api url is dev")
			appUrl = "http://localhost:3000"
		}
		initScreenWithLogo()
		authResult, err := auth.Login(logger, appUrl)
		if err != nil {
			logger.Fatal("failed to login: %s", err)
		}
		viper.Set("auth.api_key", authResult.APIKey)
		viper.Set("auth.user_id", authResult.UserId)
		if err := viper.WriteConfig(); err != nil {
			logger.Fatal("failed to write config: %s", err)
		}
		printSuccess("You are now logged in")
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of the Agentuity Cloud Platform",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		appUrl := viper.GetString("overrides.app_url")
		token := viper.GetString("auth.api_key")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		viper.Set("auth.api_key", "")
		viper.Set("auth.user_id", "")
		if err := viper.WriteConfig(); err != nil {
			logger.Fatal("failed to write config: %s", err)
		}
		initScreenWithLogo()
		if err := auth.Logout(logger, appUrl, token); err != nil {
			logger.Fatal("failed to logout: %s", err)
		}
		printSuccess("You have been logged out")
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the current logged in user details",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		userId := viper.GetString("auth.user_id")
		if userId == "" {
			logger.Fatal("you are not logged in")
		}
		logger.Info("You are logged in with user id: %s", userId)
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	rootCmd.AddCommand(authLoginCmd)
}
