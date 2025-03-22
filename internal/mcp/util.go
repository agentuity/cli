package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	mcp_golang "github.com/agentuity/mcp-golang/v2"
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

func execCommand(ctx context.Context, dir string, command string, args ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	args = append([]string{command}, args...)
	args = append(args, "--log-level", "warn")
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AGENTUITY_MCP_SESSION=1")
	output, err := cmd.Output()
	return string(output), err
}
