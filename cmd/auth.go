package cmd

import (
	"github.com/agentuity/cli/internal/auth"
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
		logger := newLogger(cmd)
		appUrl := viper.GetString("overrides.app_url")
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
		logger := newLogger(cmd)
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
		logger := newLogger(cmd)
		token := viper.GetString("auth.api_key")
		if token == "" {
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
	authCmd.PersistentFlags().String("app-url", "https://app.agentuity.com", "The base url of the Agentuity Console app")
	authCmd.PersistentFlags().MarkHidden("app-url")
	viper.BindPFlag("overrides.app_url", authCmd.PersistentFlags().Lookup("app-url"))

	authCmd.PersistentFlags().String("api-url", "https://api.agentuity.com", "The base url of the Agentuity API")
	authCmd.PersistentFlags().MarkHidden("api-url")
	viper.BindPFlag("overrides.api_url", authCmd.PersistentFlags().Lookup("api-url"))

	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
}
