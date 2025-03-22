package mcp

import (
	"context"
	"fmt"

	mcp_golang "github.com/metoro-io/mcp-golang"
)

type HelloArguments struct {
	Submitter string `json:"submitter" jsonschema:"required,description=The name of the thing calling this tool (openai or google or claude etc)"`
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("test", "this is a tool for testing the mcp server and making sure it works", func(ctx context.Context, args HelloArguments) (*mcp_golang.ToolResponse, error) {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Hello, %s!", args.Submitter))), nil
		})
	})
}
