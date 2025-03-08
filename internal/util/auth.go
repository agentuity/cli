package util

import (
	"fmt"
	"os"
	"time"

	"github.com/agentuity/cli/internal/tui"
	"github.com/spf13/viper"
)

// ShowLogin displays a login prompt to the user
func ShowLogin() {
	fmt.Println(tui.Warning("You are not currently logged in or your session has expired."))
	tui.ShowBanner("Login", tui.Text("Use ")+tui.Command("login")+tui.Text(" to login to Agentuity"), false)
}

// EnsureLoggedIn checks if the user is logged in and returns the API key and user ID
// If not logged in, it shows a login prompt and exits
func EnsureLoggedIn() (string, string) {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		ShowLogin()
		os.Exit(1)
	}
	userId := viper.GetString("auth.user_id")
	if userId == "" {
		ShowLogin()
		os.Exit(1)
	}
	expires := viper.GetInt64("auth.expires")
	if expires < time.Now().UnixMilli() {
		ShowLogin()
		os.Exit(1)
	}
	return apikey, userId
}
