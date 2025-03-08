package util

import (
	"github.com/agentuity/go-common/logger"
	"github.com/spf13/viper"
)

// GetURLs returns the API URL and app URL based on configuration
func GetURLs(logger logger.Logger) (string, string) {
	apiUrl := viper.GetString("overrides.api_url")
	appUrl := viper.GetString("overrides.app_url")
	logger.Trace("using api_url: %s", apiUrl)
	logger.Trace("using app_url: %s", appUrl)
	return apiUrl, appUrl
}
