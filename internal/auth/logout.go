package auth

import (
	"time"

	"github.com/spf13/viper"
)

func Logout() {
	viper.Set("auth.api_key", "")
	viper.Set("auth.user_id", "")
	viper.Set("auth.expires", time.Now().UnixMilli())
	viper.WriteConfig()
}
