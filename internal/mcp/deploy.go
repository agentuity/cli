package mcp

import (
	"context"

	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

type DeployArguments struct {
	Directory string `json:"directory" jsonschema:"required,description=The directory where the project is located"`
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
			result, err := execCommand(ctx, c.ProjectDir, "deploy", "--format", "json", "--dir", c.ProjectDir)
			if err != nil {
				return nil, err
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
		})
	})
}
