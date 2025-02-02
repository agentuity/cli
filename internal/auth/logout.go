package auth

import (
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

const (
	logoutPath        = "/auth/logout"
	successLogoutPath = "/auth/logout/success"
	logoutWaitMessage = "Waiting for logout to complete in the browser..."
)

// Logout will open a browser and wait for the application to logout.
func Logout(logger logger.Logger, baseUrl string, authToken string) error {
	return util.BrowserFlow(util.BrowserFlowOptions{
		Logger:      logger,
		BaseUrl:     baseUrl,
		StartPath:   logoutPath,
		SuccessPath: successLogoutPath,
		WaitMessage: logoutWaitMessage,
	})
}
