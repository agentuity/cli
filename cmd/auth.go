package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/99designs/keyring"
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

func showLogin() {
	fmt.Println(tui.Warning("You are not currently logged in or your session has expired."))
	tui.ShowBanner("Login", tui.Text("Use ")+tui.Command("login")+tui.Text(" to login to Agentuity"), false)
}

func ensureKeyring(profile string) keyring.Keyring {
	keyring, err := keyring.Open(keyring.Config{
		ServiceName:                    "agentuity-" + profile,
		KeychainTrustApplication:       true,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: false,
		FileDir:                        filepath.Dir(cfgFile),
	})
	if err != nil {
		errsystem.New(errsystem.ErrAuthenticateUser, err,
			errsystem.WithContextMessage("Failed to open keyring")).ShowErrorAndExit()
	}
	return keyring
}

func getProfileName() string {
	profile := viper.GetString("name")
	if profile == "" {
		profile = "default"
		viper.Set("name", profile)
		if err := viper.WriteConfig(); err != nil {
			errsystem.New(errsystem.ErrWriteConfigurationFile, err,
				errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
		}
	}
	return profile
}

func deleteAPIKey() {
	profile := getProfileName()
	ring := ensureKeyring(profile)
	err := ring.Remove("apikey")
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return
		}
		errsystem.New(errsystem.ErrAuthenticateUser, err,
			errsystem.WithContextMessage("Failed to delete apikey from keyring")).ShowErrorAndExit()
	}
}

func saveAPIKey(apikey string) {
	profile := getProfileName()
	ring := ensureKeyring(profile)
	err := ring.Set(keyring.Item{
		Key:  "apikey",
		Data: []byte(apikey),
	})
	if err != nil {
		errsystem.New(errsystem.ErrAuthenticateUser, err,
			errsystem.WithContextMessage("Failed to save apikey to keyring")).ShowErrorAndExit()
	}
}

func ensureLoggedIn() (string, string) {
	profile := getProfileName()
	ring := ensureKeyring(profile)
	apikey, err := ring.Get("apikey")
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			showLogin()
			os.Exit(1)
		}
		errsystem.New(errsystem.ErrAuthenticateUser, err,
			errsystem.WithContextMessage("Failed to get apikey from keyring")).ShowErrorAndExit()
	}
	userId := viper.GetString("auth.user_id")
	if userId == "" {
		showLogin()
		os.Exit(1)
	}
	expires := viper.GetInt64("auth.expires")
	if expires < time.Now().UnixMilli() {
		showLogin()
		os.Exit(1)
	}
	return string(apikey.Data), userId
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to the Agentuity Platform",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, appUrl := getURLs(logger)
		var otp string
		loginaction := func() {
			var err error
			otp, err = auth.GenerateLoginOTP(logger, apiUrl)
			if err != nil {
				errsystem.New(errsystem.ErrAuthenticateUser, err,
					errsystem.WithContextMessage("Failed to generate login OTP")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Generating login OTP...", loginaction)

		body := tui.Paragraph(
			"Please open the url in your browser:",
			tui.Link("%s/auth/cli", appUrl),
			"And enter the following code:",
			tui.Bold(otp),
			tui.Muted("This code will expire in 60 seconds"),
		)

		tui.ShowBanner("Login to Agentuity", body, false)

		tui.ShowSpinner("Waiting for login to complete...", func() {
			authResult, err := auth.PollForLoginCompletion(logger, apiUrl, otp)
			if err != nil {
				if errors.Is(err, auth.ErrLoginTimeout) {
					tui.ShowWarning("Login timed out. Please try again.")
					os.Exit(1)
				}
				errsystem.New(errsystem.ErrAuthenticateUser, err,
					errsystem.WithContextMessage("Failed to login")).ShowErrorAndExit()
			}
			saveAPIKey(authResult.APIKey)
			viper.Set("auth.user_id", authResult.UserId)
			viper.Set("auth.expires", authResult.Expires.UnixMilli())
			if err := viper.WriteConfig(); err != nil {
				errsystem.New(errsystem.ErrWriteConfigurationFile, err,
					errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
			}
		})

		tui.ClearScreen()
		initScreenWithLogo()
		tui.ShowLock("Your API key has been saved to your machine's secure storage")
		tui.ShowSuccess("Welcome to Agentuity! You are now logged in")
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout of the Agentuity Cloud Platform",
	Run: func(cmd *cobra.Command, args []string) {
		deleteAPIKey()
		viper.Set("auth.user_id", "")
		viper.Set("auth.expires", time.Now().UnixMilli())
		if err := viper.WriteConfig(); err != nil {
			errsystem.New(errsystem.ErrWriteConfigurationFile, err,
				errsystem.WithContextMessage("Failed to write viper config")).ShowErrorAndExit()
		}
		tui.ShowSuccess("You have been logged out")
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the current logged in user details",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _ := getURLs(logger)
		apiKey, userId := ensureLoggedIn()
		user, err := auth.GetUser(logger, apiUrl, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrAuthenticateUser, err,
				errsystem.WithContextMessage("Failed to get user")).ShowErrorAndExit()
		}
		body := tui.Paragraph(
			tui.PadRight("Name:", 15, " ")+" "+tui.Bold(tui.PadRight(user.FirstName+" "+user.LastName, 30, " "))+" "+tui.Muted(userId),
			tui.PadRight("Organization:", 15, " ")+" "+tui.Bold(tui.PadRight(user.OrgName, 31, " "))+" "+tui.Muted(user.OrgId),
		)
		tui.ShowBanner(tui.Muted("Currently logged in as"), body, false)
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authWhoamiCmd)
	rootCmd.AddCommand(authLoginCmd)
	rootCmd.AddCommand(authLogoutCmd)
}
