package mcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/project"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

type CreateAgentArguments struct {
	Name        string `json:"name" jsonschema:"required,description=The name of the agent which must be unique within the project"`
	Description string `json:"description" jsonschema:"required,description=A description of the agent and what it does"`
	AuthType    string `json:"authType" jsonschema:"required,description=The type of authentication to use for the agent which can be either 'bearer' for bearer token or 'none' for no authentication"`
	Directory   string `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

type ListAgentsArguments struct {
	Directory string `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

type DeleteAgentArguments struct {
	AgentIds  []string `json:"agentIds" jsonschema:"required,description=The IDs of the agents to delete"`
	Directory string   `json:"directory" jsonschema:"required,description=The directory where the project is located"`
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("create_agent", "this is a tool for creating an Agent using the agentuity cloud platform", func(ctx context.Context, args CreateAgentArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			result, err := execCommand(ctx, c.ProjectDir, "agent", "create", args.Name, args.Description, args.AuthType, "--force", "--dir", c.ProjectDir, "--format", "json")
			if err != nil {
				return nil, err
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("list_agents", "this is a tool for listing information about the Agents in this agentuity cloud platform project", func(ctx context.Context, args ListAgentsArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			result, err := execCommand(ctx, c.ProjectDir, "agent", "list", "--dir", c.ProjectDir, "--format", "json")
			if err != nil {
				return nil, err
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("delete_agent", "this is a tool for deleting one or more Agents from the agentuity cloud platform", func(ctx context.Context, args DeleteAgentArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			if args.Directory != "" {
				c.ProjectDir = args.Directory
			}
			if resp := ensureProject(&c); resp != nil {
				return resp, nil
			}
			if len(args.AgentIds) == 0 {
				return nil, fmt.Errorf("no agents to delete")
			}
			deleted, err := agent.DeleteAgents(ctx, c.Logger, c.APIURL, c.APIKey, c.Project.ProjectId, args.AgentIds)
			if err != nil {
				return nil, err
			}

			var agents []project.AgentConfig
			for _, agent := range c.Project.Agents {
				if !slices.Contains(deleted, agent.ID) {
					agents = append(agents, agent)
				}
			}
			c.Project.Agents = agents

			if err := c.Project.Save(c.ProjectDir); err != nil {
				return nil, fmt.Errorf("failed to save project after agent delete: %w", err)
			}

			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("Successfully deleted %d agent(s): %v", len(deleted), deleted))), nil
		})
	})
}
