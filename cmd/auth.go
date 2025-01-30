package cmd

import (
	"github.com/agentuity/cli/internal/auth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication and authorization commands",
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
		viper.Set("auth.token", authResult.Token)
		viper.Set("auth.org_id", authResult.OrgId)
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
		token := viper.GetString("auth.token")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		viper.Set("auth.token", "")
		viper.Set("auth.org_id", "")
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
		token := viper.GetString("auth.token")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		userId := viper.GetString("auth.user_id")
		if userId == "" {
			logger.Fatal("you are not logged in")
		}
		orgId := viper.GetString("auth.org_id")
		if orgId == "" {
			logger.Fatal("you are not logged in")
		}
		logger.Info("You are logged in as user_id: %s and org_id: %s", userId, orgId)
	},
}

func init() {
	authCmd.PersistentFlags().String("app-url", "https://app.agentuity.com", "The base url of the Agentuity Console app")
	authCmd.PersistentFlags().MarkHidden("app-url")
	viper.BindPFlag("overrides.app_url", authCmd.PersistentFlags().Lookup("app-url"))

	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
}
