package dev

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

func KillProjectServer(logger logger.Logger, projectServerCmd *exec.Cmd, pid int) {
	if pid > 0 {
		processes, err := getProcessTree(logger, pid)
		if err != nil {
			logger.Error("error getting process tree for parent (pid: %d): %s", pid, err)
		}
		for _, childPid := range processes {
			logger.Debug("killing child process (pid: %d)", childPid)
			kill(logger, childPid)
		}
	}
	if projectServerCmd == nil || projectServerCmd.ProcessState == nil || projectServerCmd.ProcessState.Exited() {
		logger.Debug("project server already exited (pid: %d)", pid)
		kill(logger, pid)
		return
	}
	ch := make(chan struct{}, 1)
	go func() {
		projectServerCmd.Wait()
		ch <- struct{}{}
	}()

	if projectServerCmd.Process != nil {
		logger.Debug("killing parent process %d", pid)
		if err := terminateProcess(logger, projectServerCmd); err != nil {
			logger.Error("error terminating project server: %s", err)
		}
	}

	// Wait a bit longer for SIGTERM to take effect
	select {
	case <-ch:
		return
	case <-time.After(time.Second * 3):
		// If neither signal worked, use the platform-specific kill
		util.ProcessKill(projectServerCmd)
		close(ch)
	}
}

func isPortAvailable(port int) bool {
	timeout := time.Second
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("0.0.0.0:%d", port), timeout)
	if err != nil {
		return true
	}
	defer conn.Close()
	return false
}

func FindAvailableOpenPort() (int, error) {
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func FindAvailablePort(p project.ProjectContext, tryPort int) (int, error) {
	if tryPort > 0 {
		if isPortAvailable(tryPort) {
			return tryPort, nil
		}
	}
	if v, ok := os.LookupEnv("AGENTUITY_CLOUD_PORT"); ok && v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return 0, err
		}
		if isPortAvailable(p) {
			return p, nil
		}
	}
	if v, ok := os.LookupEnv("PORT"); ok && v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return 0, err
		}
		if isPortAvailable(p) {
			return p, nil
		}
	}
	if isPortAvailable(p.Project.Development.Port) {
		return p.Project.Development.Port, nil
	}
	return FindAvailableOpenPort()
}

func CreateRunProjectCmd(ctx context.Context, log logger.Logger, theproject project.ProjectContext, server *Server, dir string, orgId string, port int, stdout io.Writer, stderr io.Writer) (*exec.Cmd, error) {
	// set the vars
	projectServerCmd := exec.CommandContext(ctx, theproject.Project.Development.Command, theproject.Project.Development.Args...)
	projectServerCmd.Env = os.Environ()[:]
	telemetryURL := server.TelemetryURL()
	telemetryAPIKey := server.TelemetryAPIKey()
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_URL=%s", telemetryURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_BEARER_TOKEN=%s", telemetryAPIKey))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_URL=%s", theproject.APIURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_TRANSPORT_URL=%s", theproject.TransportURL))

	// projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_DEPLOYMENT_ID=%s", server.ID))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PROJECT_ID=%s", theproject.Project.ProjectId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_ORG_ID=%s", orgId))

	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_SDK_DEV_MODE=true")
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_SDK_DIR=%s", dir))
	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_ENV=development")

	if theproject.Project.Bundler.Language == "javascript" {
		projectServerCmd.Env = append(projectServerCmd.Env, "NODE_ENV=development")
	}

	// for nodejs and pnpm, we need to enable source maps directly in the environment.
	// for bun, we need to inject a shim helper to parse the source maps
	if theproject.Project.Bundler.Runtime == "nodejs" {
		nodeOptions := os.Getenv("NODE_OPTIONS")
		if nodeOptions == "" {
			nodeOptions = "--enable-source-maps"
		} else {
			nodeOptions = fmt.Sprintf("%s --enable-source-maps", nodeOptions)
		}
		projectServerCmd.Env = append(projectServerCmd.Env, nodeOptions)
	}

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PORT=%d", port))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("PORT=%d", port))

	projectServerCmd.Stdout = stdout
	projectServerCmd.Stderr = stderr
	projectServerCmd.Dir = dir

	util.ProcessSetup(projectServerCmd)

	return projectServerCmd, nil
}

type Endpoint struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func GetDevModeEndpoint(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, hostname string) (*Endpoint, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[Endpoint]
	body := map[string]string{
		"hostname": hostname,
	}
	if err := client.Do("POST", fmt.Sprintf("/cli/devmode/2/%s", url.PathEscape(projectId)), body, &resp); err != nil {
		return nil, fmt.Errorf("error fetching devmode endpoint: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error fetching devmode endpoint: %s", resp.Message)
	}
	return &resp.Data, nil
}
