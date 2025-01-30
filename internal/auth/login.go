package auth

import (
	"fmt"
	"net/url"

	"github.com/agentuity/cli/internal/util"
	"github.com/shopmonkeyus/go-common/logger"
)

const (
	loginPath        = "/auth/login"
	successLoginPath = "/auth/login/success"
	loginWaitMessage = "Waiting for login to complete in the browser..."
)

type LoginResult struct {
	Token  string
	OrgId  string
	UserId string
}

// Login will open a browser and wait for the user to login. It will return the token if the user logs in successfully.
// It will return an error if the user cancels the login or if the login fails.
// If the user cancels the login or after a period of 1 minute, the login will fail and return an ErrTimeout error.
func Login(logger logger.Logger, baseUrl string) (*LoginResult, error) {
	var result LoginResult
	callback := func(query url.Values) error {
		token := query.Get("token")
		orgId := query.Get("org_id")
		userId := query.Get("user_id")
		if token == "" {
			return fmt.Errorf("no token found")
		}
		if orgId == "" {
			return fmt.Errorf("no org_id found")
		}
		if userId == "" {
			return fmt.Errorf("no user_id found")
		}
		result.Token = token
		result.OrgId = orgId
		result.UserId = userId
		return nil
	}
	if err := util.BrowserFlow(util.BrowserFlowOptions{
		Logger:      logger,
		BaseUrl:     baseUrl,
		StartPath:   loginPath,
		SuccessPath: successLoginPath,
		WaitMessage: loginWaitMessage,
		Callback:    callback,
	}); err != nil {
		return nil, err
	}
	return &result, nil
}
