package mcp

import (
	"context"

	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

type DeployArguments struct {
	Directory   string   `json:"directory" jsonschema:"required,description=The directory where the project is located"`
	Tags        []string `json:"tags,omitempty" jsonschema:"description=Tags to associate with this deployment"`
	Description string   `json:"description,omitempty" jsonschema:"description=Description for the deployment tag(s)"`
	Message     string   `json:"message,omitempty" jsonschema:"description=Message for the deployment tag(s)"`
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("deploy", "this is a tool for deploying the agent project to the agentuity cloud platform", func(ctx context.Context, args DeployArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			argsList := []string{"deploy", "--format", "json", "--dir", c.ProjectDir}
			for _, tag := range args.Tags {
				argsList = append(argsList, "--tag", tag)
			}
			if args.Description != "" {
				argsList = append(argsList, "--description", args.Description)
			}
			if args.Message != "" {
				argsList = append(argsList, "--message", args.Message)
			}
			result, err := execCommand(ctx, c.ProjectDir, argsList[0], argsList[1:]...)
			if err != nil {
				return nil, err
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
		})
	})
}
