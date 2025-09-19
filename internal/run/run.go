package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type Config struct {
	Context         context.Context
	Logger          logger.Logger
	Project         project.ProjectContext
	TelemetryURL    string
	TelemetryAPIKey string
	APIURL          string
	TransportURL    string
	OrgId           string
	AgentPort       int
	WorkingDir      string
}

func CreateRunProjectCmd(config Config) (*exec.Cmd, error) {
	// set the vars
	projectServerCmd := exec.CommandContext(config.Context, config.Project.Project.Deployment.Command, config.Project.Project.Deployment.Args...)
	projectServerCmd.Env = os.Environ()[:]
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_URL=%s", config.TelemetryURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_BEARER_TOKEN=%s", config.TelemetryAPIKey))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_URL=%s", config.APIURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_TRANSPORT_URL=%s", config.TransportURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PROJECT_ID=%s", config.Project.Project.ProjectId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_ORG_ID=%s", config.OrgId))

	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_ENV=production")

	if config.Project.Project.Bundler.Language == "javascript" {
		projectServerCmd.Env = append(projectServerCmd.Env, "NODE_ENV=production")
	}

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PORT=%d", config.AgentPort))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("PORT=%d", config.AgentPort))

	projectServerCmd.Stdout = os.Stdout
	projectServerCmd.Stderr = os.Stderr
	projectServerCmd.Dir = config.WorkingDir

	util.ProcessSetup(projectServerCmd)

	return projectServerCmd, nil
}
