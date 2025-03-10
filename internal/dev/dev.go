package dev

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
)

func CreateRunProjectCmd(log logger.Logger, theproject project.ProjectContext, liveDevConnection *LiveDevConnection, dir string, orgId string) (*exec.Cmd, error) {
	// set the vars
	projectServerCmd := exec.Command(theproject.Project.Development.Command, theproject.Project.Development.Args...)
	projectServerCmd.Env = os.Environ()
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_BEARER_TOKEN=%s", liveDevConnection.OtelToken))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_URL=%s", liveDevConnection.OtelUrl))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_SDK_DIR=%s", dir))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_DEPLOYMENT_ID=%s", liveDevConnection.WebSocketId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_ORG_ID=%s", orgId))
	projectServerCmd.Stdout = os.Stdout
	projectServerCmd.Stderr = os.Stderr
	projectServerCmd.Stdin = os.Stdin
	return projectServerCmd, nil
}
