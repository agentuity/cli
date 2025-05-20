package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

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

	for {
		var resp VerifySignupOTPResponse
		if err := client.Do("GET", "/cli/auth/signup/"+otp, nil, &resp); err != nil {
			var apiErr *util.APIError
			if errors.As(err, &apiErr) {
				if apiErr.Status == 404 {
					select {
					case <-ctx.Done():
						return "", "", 0, ctx.Err()
					case <-time.After(time.Second * 2):
						continue
					}
				}
			}
			return "", "", 0, err
		}
		if !resp.Success {
			return "", "", 0, fmt.Errorf("%s", resp.Message)
		}
		return resp.Data.UserId, resp.Data.APIKey, resp.Data.Expires, nil
	}
}
