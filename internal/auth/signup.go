package auth

import (
	"context"
	"fmt"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type VerifySignupOTPResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		UserId  string `json:"userId"`
		APIKey  string `json:"apiKey"`
		Expires int64  `json:"expiresAt"`
	} `json:"data"`
}

func VerifySignupOTP(ctx context.Context, logger logger.Logger, baseUrl string, otp string) (string, string, int64, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, "")

	var resp VerifySignupOTPResponse
	if err := client.Do("GET", "/cli/auth/signup/"+otp, nil, &resp); err != nil {
		return "", "", 0, err
	}
	if !resp.Success {
		return "", "", 0, fmt.Errorf("%s", resp.Message)
	}
	return resp.Data.UserId, resp.Data.APIKey, resp.Data.Expires, nil
}
