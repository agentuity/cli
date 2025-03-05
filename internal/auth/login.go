package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

var ErrLoginTimeout = errors.New("timed out")

type LoginResult struct {
	APIKey string
	UserId string
}

type OTPStartResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		OTP string `json:"otp"`
	} `json:"data"`
}

func GenerateLoginOTP(logger logger.Logger, baseUrl string) (string, error) {
	client := util.NewAPIClient(logger, baseUrl, "")

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
		APIKey string `json:"apiKey"`
		UserId string `json:"userId"`
	} `json:"data,omitempty"`
}

func PollForLoginCompletion(logger logger.Logger, baseUrl string, otp string) (*LoginResult, error) {
	client := util.NewAPIClient(logger, baseUrl, "")
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
			time.Sleep(time.Second * 2)
			continue
		}
		return &LoginResult{
			APIKey: resp.Data.APIKey,
			UserId: resp.Data.UserId,
		}, nil
	}
	return nil, ErrLoginTimeout
}
