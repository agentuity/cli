package dev

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
)

func KillProjectServer(projectServerCmd *exec.Cmd) {
	projectServerCmd.Process.Signal(syscall.SIGTERM)
	ch := make(chan struct{})
	go func() {
		projectServerCmd.Wait()
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		break
	case <-time.After(time.Second * 10):
		projectServerCmd.Process.Kill()
	}
}

func isPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func FindAvailablePort(p project.ProjectContext) (int, error) {
	if v, ok := os.LookupEnv("PORT"); ok {
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

func CreateRunProjectCmd(log logger.Logger, theproject project.ProjectContext, liveDevConnection *Websocket, dir string, orgId string, port int) (*exec.Cmd, error) {
	// set the vars
	projectServerCmd := exec.Command(theproject.Project.Development.Command, theproject.Project.Development.Args...)
	projectServerCmd.Env = os.Environ()
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_BEARER_TOKEN=%s", liveDevConnection.OtelToken))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_OTLP_URL=%s", liveDevConnection.OtelUrl))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_URL=%s", theproject.APIURL))

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_DEPLOYMENT_ID=%s", liveDevConnection.WebSocketId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_PROJECT_ID=%s", theproject.Project.ProjectId))
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_CLOUD_ORG_ID=%s", orgId))

	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_SDK_DEV_MODE=true")
	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("AGENTUITY_SDK_DIR=%s", dir))
	projectServerCmd.Env = append(projectServerCmd.Env, "AGENTUITY_ENV=development")

	if theproject.Project.Bundler.Language == "javascript" {
		projectServerCmd.Env = append(projectServerCmd.Env, "NODE_ENV=development")
	}

	projectServerCmd.Env = append(projectServerCmd.Env, fmt.Sprintf("PORT=%d", port))

	projectServerCmd.Stdout = os.Stdout
	projectServerCmd.Stderr = os.Stderr
	projectServerCmd.Stdin = os.Stdin

	return projectServerCmd, nil
}
