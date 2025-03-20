package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

var ErrLoginTimeout = errors.New("timed out")

type LoginResult struct {
	APIKey  string
	UserId  string
	Expires time.Time
}

type OTPStartResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		OTP string `json:"otp"`
	} `json:"data"`
}

func GenerateLoginOTP(ctx context.Context, logger logger.Logger, baseUrl string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, "")

	var resp OTPStartResponse
	if err := client.Do("GET", "/cli/auth/start", nil, &resp); err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("%s", resp.Message)
	}
	return resp.Data.OTP, nil
}

type OTPCompleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    *struct {
		APIKey  string `json:"apiKey"`
		UserId  string `json:"userId"`
		Expires int64  `json:"expires"`
	} `json:"data,omitempty"`
}

func PollForLoginCompletion(ctx context.Context, logger logger.Logger, baseUrl string, otp string) (*LoginResult, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, "")
	body := map[string]string{"otp": otp}
	started := time.Now()
	for time.Since(started) < time.Minute {
		var resp OTPCompleteResponse
		if err := client.Do("POST", "/cli/auth/check", body, &resp); err != nil {
			return nil, err
		}
		if !resp.Success {
			return nil, fmt.Errorf("%s", resp.Message)
		}
		if resp.Data == nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * 2):
			}
			continue
		}
		return &LoginResult{
			APIKey:  resp.Data.APIKey,
			UserId:  resp.Data.UserId,
			Expires: time.UnixMilli(resp.Data.Expires),
		}, nil
	}
	return nil, ErrLoginTimeout
}

type Organization struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	FirstName     string         `json:"firstName"`
	LastName      string         `json:"lastName"`
	Organizations []Organization `json:"organizations"`
}

type UserResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    *User  `json:"data"`
}

func GetUser(ctx context.Context, logger logger.Logger, baseUrl string, apiKey string) (*User, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, apiKey)

	var resp UserResponse
	if err := client.Do("GET", "/cli/auth/user", nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Message)
	}
	return resp.Data, nil
}
