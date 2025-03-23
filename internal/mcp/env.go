package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/tui"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

type EnvSetArguments struct {
	Key       string `json:"key" jsonschema:"required,description=The name of the environment variable or secret to set"`
	Value     string `json:"value" jsonschema:"required,description=The value to set the environment variable or secret to set"`
	IsSecret  bool   `json:"isSecret" jsonschema:"required,description=Set to true if the environment variable is a secret or looks at all like a secret, api key, password, etc"`
	Directory string `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

type EnvDeleteArguments struct {
	Keys      []string `json:"keys" jsonschema:"required,description=The names of the environment variables or secrets to delete"`
	Directory string   `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

type EnvListArguments struct {
	Directory string `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("set_project_environment", "this is a tool for setting the environment variable or secret for the current agentuity project", func(ctx context.Context, args EnvSetArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			if strings.HasPrefix(args.Key, "AGENTUITY_") {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("You cannot set a project environment variable that starts with AGENTUITY_")), nil
			}
			var err error
			if args.IsSecret {
				_, err = c.Project.SetProjectEnv(ctx, c.Logger, c.APIURL, c.APIKey, map[string]string{}, map[string]string{args.Key: args.Value})
			} else {
				_, err = c.Project.SetProjectEnv(ctx, c.Logger, c.APIURL, c.APIKey, map[string]string{args.Key: args.Value}, map[string]string{})
			}
			if err != nil {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Error setting environment variable: %s", err))), nil
			}
			project.SaveEnvValue(ctx, c.Logger, c.ProjectDir, map[string]string{args.Key: args.Value})
			if args.IsSecret {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("%s was set as a secret", args.Key))), nil
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("%s was set", args.Key))), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("list_project_environment", "this is a tool for showing the environment variable or secrets configured for the current agentuity project", func(ctx context.Context, args EnvListArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			projectData, err := c.Project.GetProject(ctx, c.Logger, c.APIURL, c.APIKey)
			if err != nil {
				return nil, err
			}

			var output bytes.Buffer

			for key, value := range projectData.Env {
				if !tui.HasTTY {
					io.WriteString(&output, fmt.Sprintf("%s=%s\n", key, value))
				} else {
					io.WriteString(&output, fmt.Sprintf("%s=%s\n", tui.Title(key), tui.Body(value)))
				}
			}
			for key, value := range projectData.Secrets {
				if !tui.HasTTY {
					io.WriteString(&output, fmt.Sprintf("%s=%s\n", key, value))
				} else {
					io.WriteString(&output, fmt.Sprintf("%s=%s\n", tui.Title(key), tui.Muted(value)))
				}
			}
			if len(projectData.Env) == 0 && len(projectData.Secrets) == 0 {
				io.WriteString(&output, "No environment variables or secrets set for this project")
				io.WriteString(&output, "\n")
				io.WriteString(&output, fmt.Sprintf("You can set environment variables with %s", tui.Command("env", "set", "<key>", "<value>")))
				io.WriteString(&output, "\n")
			}

			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(output.String())), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("delete_project_environment", "this is a tool for deleting one or more environment variables or secrets configured for the current agentuity project", func(ctx context.Context, args EnvDeleteArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			if err := c.Project.DeleteProjectEnv(ctx, c.Logger, c.APIURL, c.APIKey, args.Keys, args.Keys); err != nil {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Error deleting environment variable: %s", err))), nil
			}
			if err := project.RemoveEnvValues(ctx, c.Logger, c.ProjectDir, args.Keys...); err != nil {
				return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Error deleting environment variable from .env file: %s", err))), nil
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Environment variables and secrets deleted")), nil
		})
	})
}
