package mcp

import (
	"context"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
	"github.com/spf13/cobra"
)

type MCPContext struct {
	Context      context.Context
	Logger       logger.Logger
	Command      *cobra.Command
	Server       *mcp_golang.Server
	APIKey       string
	APIURL       string
	TransportURL string
	AppURL       string
	LoggedIn     bool
	ProjectDir   string
	Project      *project.Project
}

type NoArguments struct {
}

type RegisterCallback func(ctx MCPContext) error

var callbacks []RegisterCallback

func register(callback RegisterCallback) {
	callbacks = append(callbacks, callback)
}

func Register(ctx MCPContext) error {
	for _, callback := range callbacks {
		if err := callback(ctx); err != nil {
			return err
		}
	}
	return nil
}
