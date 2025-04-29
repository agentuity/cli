package dev

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

func KillProjectServer(projectServerCmd *exec.Cmd) {
	ch := make(chan struct{}, 1)
	go func() {
		projectServerCmd.Wait()
		ch <- struct{}{}
	}()

	if projectServerCmd.Process != nil {
		// Try SIGINT first (Ctrl+C equivalent)
		projectServerCmd.Process.Signal(syscall.SIGINT)
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
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func FindAvailablePort(p project.ProjectContext) (int, error) {
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
	return findAvailablePort()
}

func CreateRunProjectCmd(ctx context.Context, log logger.Logger, theproject project.ProjectContext, liveDevConnection *Websocket, dir string, orgId string, port int) (*exec.Cmd, error) {
	// set the vars
	projectServerCmd := exec.CommandContext(ctx, theproject.Project.Development.Command, theproject.Project.Development.Args...)
	projectServerCmd.Env = os.Environ()[:]
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_BEARER_TOKEN=%s", liveDevConnection.OtelToken))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_URL=%s", liveDevConnection.OtelUrl))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_URL=%s", theproject.APIURL))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_TRANSPORT_URL=%s", theproject.TransportURL))

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_DEPLOYMENT_ID=%s", liveDevConnection.webSocketId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PROJECT_ID=%s", theproject.Project.ProjectId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_ORG_ID=%s", orgId))

	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_SDK_DEV_MODE=true")
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_SDK_DIR=%s", dir))
	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_ENV=development")

	if theproject.Project.Bundler.Language == "javascript" {
		projectServerCmd.Env = append(projectServerCmd.Env, "NODE_ENV=development")
	}

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PORT=%d", port))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("PORT=%d", port))

	projectServerCmd.Stdout = os.Stdout
	projectServerCmd.Stderr = os.Stderr
	projectServerCmd.Stdin = os.Stdin
	projectServerCmd.Dir = dir

	util.ProcessSetup(projectServerCmd)

	return projectServerCmd, nil
}
