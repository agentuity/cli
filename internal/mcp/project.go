package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/agentuity/cli/internal/templates"
	cstr "github.com/agentuity/go-common/string"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

type CreateProjectArguments struct {
	Name             string `json:"name" jsonschema:"required,description=The name of the project which must be unique within the organization"`
	Description      string `json:"description" jsonschema:"required,description=A description of the project"`
	AgentName        string `json:"agentName" jsonschema:"required,description=The name of the agent to create"`
	AgentDescription string `json:"agentDescription" jsonschema:"required,description=A description of the agent and what it does"`
	AuthType         string `json:"authType" jsonschema:"required,description=The type of authentication to use for the agent which can be either 'bearer' for bearer token or 'none' for no authentication"`
	Directory        string `json:"directory" jsonschema:"required,description=The directory to create the project in in the local file system"`
	Provider         string `json:"provider" jsonschema:"required,description=The provider identifier to use for the project. Use the 'list_providers' tool to get a list of available providers"`
	Template         string `json:"template" jsonschema:"required,description=The template name to use for the project. Use the 'list_templates' tool to get a list of available templates"`
	OrganizationId   string `json:"orgId" jsonschema:"description=The organization id to create the project in if the user is a member of more than one organization. Make sure to use whoami tool to get a list of organization ids and ask the user which org to choose."`
}

type ListTemplatesArguments struct {
	Provider string `json:"provider" jsonschema:"required,description=The provider to use for the project which can be either 'bunjs' for BunJS, 'nodejs' for NodeJS, or 'uv' for Python with UV"`
}

func init() {
	register(func(c MCPContext) error {
		return c.Server.RegisterTool("create_project", "this is a tool for creating a new agentuity project using the agentuity cloud platform", func(ctx context.Context, args CreateProjectArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			cmdargs := []string{args.Name, args.Description, args.AgentName, args.AgentDescription, args.AuthType, "--dir", args.Directory, "--provider", args.Provider, "--template", args.Template, "--force", "--format", "json"}
			if args.OrganizationId != "" {
				cmdargs = append(cmdargs, "--org-id", args.OrganizationId)
			}
			result, err := execCommand(ctx, "", "new", cmdargs...)
			cmd := "Ran the command:\n\tagentuity new " + strings.Join(cmdargs, " ") + "\n\nOutput was:\n"
			if err != nil {
				if len(result) > 0 {
					return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(cmd + result)), err
				}
				return nil, fmt.Errorf("error creating project: %w. %s", err, cmd)
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(cmd + result)), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("list_providers", "this is a tool for listing all the available runtime provider for the agentuity cloud platform", func(ctx context.Context, args NoArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			templates, err := templates.LoadTemplates(ctx, c.TemplateDir, false)
			if err != nil {
				return nil, err
			}
			var res []map[string]any
			for _, t := range templates {
				res = append(res, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"identifier":  t.Identifier,
					"language":    t.Language,
				})
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Here are the available runtime providers in JSON format:\n" + cstr.JSONStringify(res))), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("list_templates", "this is a tool for listing all the available templates for the given provider", func(ctx context.Context, args ListTemplatesArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			templates, err := templates.LoadLanguageTemplates(ctx, c.TemplateDir, args.Provider)
			if err != nil {
				return nil, err
			}
			var res []map[string]any
			for _, t := range templates {
				res = append(res, map[string]any{
					"name":        t.Name,
					"description": t.Description,
				})
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Here are the available templates in JSON format:\n" + cstr.JSONStringify(res))), nil
		})
	})

	register(func(c MCPContext) error {
		return c.Server.RegisterTool("list_projects", "this is a tool for listing the user's projects running in the agentuity cloud platform", func(ctx context.Context, args NoArguments) (*mcp_golang.ToolResponse, error) {
			if resp := ensureLoggedIn(&c); resp != nil {
				return resp, nil
			}
			result, err := execCommand(ctx, "", "project", "list", "--format", "json")
			if err != nil {
				return nil, err
			}
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(result)), nil
		})
	})
}
