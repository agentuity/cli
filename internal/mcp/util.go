package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	mcp_golang "github.com/agentuity/mcp-golang/v2"
)

func ensureLoggedIn(c *MCPContext) *mcp_golang.ToolResponse {
	if !c.LoggedIn {
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("You are not currently logged in or your session has expired. Please login again."))
	}
	return nil
}

func ensureProject(c *MCPContext) *mcp_golang.ToolResponse {
	if c.Project == nil {
		if c.ProjectDir != "" {
			p := project.LoadProject(c.Logger, c.ProjectDir, c.APIURL, c.AppURL, c.TransportURL, c.AppURL)
			if p.Project != nil {
				c.Project = p.Project
				return nil
			}
		}
		cwd, _ := os.Getwd()
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(fmt.Sprintf("You are not currently in a project directory (%s). Your current working directory is %s. Please navigate to a Agentuity project directory and try again or pass in the project directory", c.ProjectDir, cwd)))
	}
	return nil
}

func execCommand(ctx context.Context, dir string, command string, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	args = append([]string{command}, args...)
	args = append(args, "--log-level", "warn")
	cmd := exec.CommandContext(ctx, exe, args...)
	util.ProcessSetup(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "AGENTUITY_MCP_SESSION=1")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error executing command: %w. %s", err, string(output))
	}
	return string(output), nil
}
