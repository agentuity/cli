package auth

import (
	"fmt"
	"net/url"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

const (
	loginPath        = "/auth/cli"
	loginWaitMessage = "Waiting for login to complete in the browser..."
)

type LoginResult struct {
	APIKey string
	UserId string
}

// Login will open a browser and wait for the user to login. It will return the token if the user logs in successfully.
// It will return an error if the user cancels the login or if the login fails.
// If the user cancels the login or after a period of 1 minute, the login will fail and return an ErrTimeout error.
func Login(logger logger.Logger, baseUrl string) (*LoginResult, error) {
	var result LoginResult
	callback := func(query url.Values) error {
		apiKey := query.Get("api_key")
		userId := query.Get("user_id")
		if apiKey == "" {
			return fmt.Errorf("no token found")
		}
		if userId == "" {
			return fmt.Errorf("no user_id found")
		}
		result.APIKey = apiKey
		result.UserId = userId
		return nil
	}
	if err := util.BrowserFlow(util.BrowserFlowOptions{
		Logger:      logger,
		BaseUrl:     baseUrl,
		StartPath:   loginPath,
		WaitMessage: loginWaitMessage,
		Callback:    callback,
	}); err != nil {
		return nil, err
	}
	return &result, nil
}
