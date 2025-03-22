package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/agentuity/cli/internal/auth"
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/spf13/viper"
)

func ensureLoggedIn(c MCPContext) *mcp_golang.ToolResponse {
	if !c.LoggedIn {
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("You are not currently logged in or your session has expired. Please login again."))
	}
	return nil
}

func ensureProject(c MCPContext) *mcp_golang.ToolResponse {
	if c.Project == nil {
		cwd, _ := os.Getwd()
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("You are not currently in a project directory (%s). Your current working directory is %s. Your environment variables are %v. Please navigate to an Agentuity project directory and try again.", c.ProjectDir, cwd, os.Environ())))
	}
	return nil
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("login", "this is a tool for generating a login authentication token and url to perform the login to the agentuity cloud platform. present the code and url to the user and ask them to visit the url to complete the login", func(ctx context.Context, args NoArguments) (*mcp_golang.ToolResponse, error) {

			logger := c.Logger

			// Generate OTP
			otp, err := auth.GenerateLoginOTP(ctx, logger, c.APIURL)
			if err != nil {
				return nil, fmt.Errorf("failed to generate login OTP: %w", err)
			}

			// Poll for completion
			go func() {
				authResult, err := auth.PollForLoginCompletion(ctx, logger, c.APIURL, otp)
				if err != nil {
					logger.Error("Login failed: %v", err)
					return
				}

				// Save the auth result
				viper.Set("auth.api_key", authResult.APIKey)
				viper.Set("auth.user_id", authResult.UserId)
				viper.Set("auth.expires", authResult.Expires.UnixMilli())
				if err := viper.WriteConfig(); err != nil {
					logger.Error("Failed to write config: %v", err)
					return
				}
				logger.Info("Successfully logged in!")
			}()

			// Return the OTP and URL to the user
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Please visit %s/auth/cli and enter the code: %s", c.AppURL, otp))), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("whoami", "this is a tool for validating the current user's authentication or logged in status for the agentuity cloud platform", func(ctx context.Context, args NoArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(c); resp != nil {
				return resp, nil
			}

			user, err := auth.GetUser(ctx, c.Logger, c.APIURL, c.APIKey)
			if err != nil {
				return nil, fmt.Errorf("failed to get user: %w", err)
			}

			// Return the OTP and URL to the user
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("You are logged in as %s %s", user.FirstName, user.LastName))), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("logout", "this is a tool for logging out the current user from the agentuity cloud platform", func(ctx context.Context, args NoArguments) (*mcp_golang.ToolResponse, error) {
			auth.Logout()
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("You are logged out!")), nil
		})
	})
}
