package cmd

import (
	"github.com/agentuity/cli/internal/auth"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/tui"
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
		_, appUrl := getURLs(logger)
		initScreenWithLogo()
		authResult, err := auth.Login(logger, appUrl)
		if err != nil {
			errsystem.New(errsystem.ErrAuthenticateUser, err,
				errsystem.WithContextMessage("Failed to login")).ShowErrorAndExit()
		}
		viper.Set("auth.api_key", authResult.APIKey)
		viper.Set("auth.user_id", authResult.UserId)
		if err := viper.WriteConfig(); err != nil {
			errsystem.New(errsystem.ErrWriteConfigurationFile, err,
				errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
		}
		tui.ShowSuccess("You are now logged in")
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of the Agentuity Cloud Platform",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		_, appUrl := getURLs(logger)
		token := viper.GetString("auth.api_key")
		if token == "" {
			logger.Fatal("You are not logged in. Please run `agentuity login` to login.")
		}
		viper.Set("auth.api_key", "")
		viper.Set("auth.user_id", "")
		if err := viper.WriteConfig(); err != nil {
			errsystem.New(errsystem.ErrWriteConfigurationFile, err,
				errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
		}
		initScreenWithLogo()
		if err := auth.Logout(logger, appUrl, token); err != nil {
			errsystem.New(errsystem.ErrApiRequest, err,
				errsystem.WithContextMessage("Failed to logout")).ShowErrorAndExit()
		}
		tui.ShowSuccess("You have been logged out")
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the current logged in user details",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("You are not logged in. Please run `agentuity login` to login.")
		}
		userId := viper.GetString("auth.user_id")
		if userId == "" {
			logger.Fatal("You are not logged in. Please run `agentuity login` to login.")
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
