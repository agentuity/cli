package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/auth"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication and authorization related commands",
	Long: `Authentication and authorization related commands for managing your Agentuity account.

Use the subcommands to login, logout, and check your authentication status.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to the Agentuity Platform",
	Long: `Login to the Agentuity Platform using a browser-based authentication flow.

This command will generate a one-time password (OTP) and print a link to a URL
where you can complete the authentication process.

Examples:
  agentuity login
  agentuity auth login`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, appUrl, _ := util.GetURLs(logger)
		var otp string
		var upgrade bool
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		loginaction := func() {
			var err error
			otp, upgrade, err = auth.GenerateLoginOTP(ctx, logger, apiUrl)
			if upgrade {
				return
			}
			if err != nil {
				if isCancelled(ctx) {
					os.Exit(1)
				}
				errsystem.New(errsystem.ErrAuthenticateUser, err,
					errsystem.WithContextMessage("Failed to generate login OTP")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Generating login OTP...", loginaction)
		if upgrade {
			tui.ShowWarning("A new version of the CLI is required, will automatically attempt to upgrade...")
			if err := util.UpgradeCLI(ctx, logger, true); err != nil {
				errsystem.New(errsystem.ErrUpgradeCli, err, errsystem.WithAttributes(map[string]any{"version": Version})).ShowErrorAndExit()
			}
			tui.ShowWarning("Please re-run the login command to continue")
			os.Exit(1)
		}

		body := tui.Paragraph(
			"Copy the following code:",
			tui.Bold(otp),
			"Then open the url in your browser and paste the code:",
			tui.Link("%s/auth/cli", appUrl),
			tui.Muted("This code will expire in 60 seconds"),
		)

		tui.ShowBanner("Login to Agentuity", body, false)

		tui.ShowSpinner("Waiting for login to complete...", func() {
			authResult, err := auth.PollForLoginCompletion(ctx, logger, apiUrl, otp)
			if err != nil {
				if isCancelled(ctx) {
					os.Exit(1)
				}
				if errors.Is(err, auth.ErrLoginTimeout) {
					tui.ShowWarning("Login timed out. Please try again.")
					os.Exit(1)
				}
				errsystem.New(errsystem.ErrAuthenticateUser, err,
					errsystem.WithContextMessage("Failed to login")).ShowErrorAndExit()
			}
			viper.Set("auth.api_key", authResult.APIKey)
			viper.Set("auth.user_id", authResult.UserId)
			viper.Set("auth.expires", authResult.Expires.UnixMilli())
			viper.Set("preferences.orgId", "")
			if err := viper.WriteConfig(); err != nil {
				errsystem.New(errsystem.ErrWriteConfigurationFile, err,
					errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
			}
		})

		tui.ClearScreen()
		initScreenWithLogo()
		tui.ShowSuccess("Welcome to Agentuity! You are now logged in")
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of the Agentuity Cloud Platform",
	Long: `Logout of the Agentuity Cloud Platform.

This command will remove your authentication credentials from the local configuration.

Examples:
  agentuity logout
  agentuity auth logout`,
	Run: func(cmd *cobra.Command, args []string) {
		auth.Logout()
		tui.ShowSuccess("You have been logged out")
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the current logged in user details",
	Long: `Print the current logged in user details.

This command displays information about the currently authenticated user,
including name, organization, and IDs.

Examples:
  agentuity auth whoami`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apiKey, userId := util.EnsureLoggedIn(ctx, logger, cmd)
		user, err := auth.GetUser(ctx, logger, apiUrl, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrAuthenticateUser, err,
				errsystem.WithContextMessage("Failed to get user")).ShowErrorAndExit()
		}
		if user == nil {
			auth.Logout()
			util.ShowLogin(ctx, logger, cmd)
			os.Exit(1)
		}
		var orgs []string
		orgs = append(orgs, tui.Bold(tui.Muted("You are a member of the following organizations:")))
		for _, org := range user.Organizations {
			orgs = append(orgs, tui.PadRight("Organization:", 15, " ")+" "+tui.Bold(tui.PadRight(org.Name, 31, " "))+" "+tui.Muted(org.Id))
		}
		body := tui.Paragraph(
			tui.PadRight("Name:", 15, " ")+" "+tui.Bold(tui.PadRight(user.FirstName+" "+user.LastName, 30, " "))+" "+tui.Muted(userId),
			orgs...,
		)
		if strings.Contains(apiUrl, "agentuity.dev") {
			body += "\n\n" + tui.Warning("Logged in to development, not production")
		}
		tui.ShowBanner(tui.Muted("Currently logged in as:"), body, false)
	},
}

var authSignupCmd = &cobra.Command{
	Use:   "signup",
	Short: "Create a new Agentuity Cloud Platform account",
	Long: `Create a new Agentuity Cloud Platform account.

Examples:
  agentuity auth signup`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, appUrl, _ := util.GetURLs(logger)
		var otp string
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		otp = util.RandStringBytes(5)

		body := tui.Paragraph(
			"Please open the url in your browser:",
			tui.Link("%s/sign-up?code=%s", appUrl, otp),
			tui.Muted("Once you have completed the signup process, you will be given a one-time password to complete the signup process."),
		)

		tui.ShowBanner("Signup for Agentuity", body, false)
		fmt.Println()

		action := func() {
			userId, apiKey, expires, err := auth.VerifySignupOTP(ctx, logger, apiUrl, otp)
			if err != nil {
				errsystem.New(errsystem.ErrAuthenticateUser, err,
					errsystem.WithContextMessage("Failed to verify signup OTP")).ShowErrorAndExit()
			}
			viper.Set("auth.api_key", apiKey)
			viper.Set("auth.user_id", userId)
			viper.Set("auth.expires", expires)
			viper.Set("preferences.orgId", "")
			if err := viper.WriteConfig(); err != nil {
				errsystem.New(errsystem.ErrWriteConfigurationFile, err,
					errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Waiting for signup to complete...", action)

		tui.ClearScreen()
		initScreenWithLogo()
		tui.ShowSuccess("Welcome to Agentuity! You are now logged in")
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	authCmd.AddCommand(authSignupCmd)
	rootCmd.AddCommand(authLoginCmd)
	rootCmd.AddCommand(authLogoutCmd)
}
