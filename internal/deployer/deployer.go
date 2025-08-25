package deployer

import (
	"context"
	"os"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
)

type EnvFile struct {
	Filepath string
	Env      []env.EnvLineComment
}

func (e *EnvFile) Lookup(key string) (string, bool) {
	for _, line := range e.Env {
		if line.Key == key {
			return line.Val, true
		}
	}
	return "", false
}

type PromptHelpers struct {
	// ShowSpinner will show a spinner with the title while the action is running
	ShowSpinner func(title string, action func())
	// PrintSuccess will print a check mark and the message provided with optional formatting arguments
	PrintSuccess func(msg string, args ...any)
	// PrintLock will print a lock and the message provided with optional formatting arguments
	PrintLock func(msg string, args ...any)
	// PrintLock will print an X mark and the message provided with optional formatting arguments
	PrintWarning func(msg string, args ...any)
	// CommandString will format a CLI command
	CommandString func(cmd string, args ...string) string
	// LinkString will return a formatted URL string
	LinkString func(cmd string, args ...any) string
	// Ask will ask the user for input and return true (confirm) or false (no!)
	Ask func(logger logger.Logger, title string, defaultValue bool) bool
	// PromptForEnv is a helper for prompting the user to get a environment (or secret) value. You must do something with the result such as save it.
	PromptForEnv func(logger logger.Logger, key string, isSecret bool, localenv map[string]string, osenv map[string]string, defaultValue string, placeholder string) string
}

type DeployPreflightCheckData struct {
	// Dir returns the full path to the project folder
	Dir string
	// APIClient is for communicating with the backend
	APIClient *util.APIClient
	// APIURL is the base url to the API
	APIURL string
	// APIKey is the projects api key
	APIKey string
	// Envfile if the project has a .env file and the parsed contents of that file
	Envfile *EnvFile
	// Project is the project data
	Project *project.Project
	// ProjectData is the project data loaded from the backend
	ProjectData *project.ProjectData
	// Config is the deployment configuration
	Config *project.DeploymentConfig
	// PromptHelpers are a set of funcs to assist in prompting the user on the command line
	PromptHelpers PromptHelpers
	// OS Environment as a map
	OSEnvironment map[string]string
}

func PreflightCheck(ctx context.Context, logger logger.Logger, data DeployPreflightCheckData, noBuild bool) (util.ZipDirCallbackMutator, error) {
	started := time.Now()
	bundleCtx := bundler.BundleContext{
		Context:    context.Background(),
		Logger:     logger,
		ProjectDir: data.Dir,
		Production: true,
		Project:    data.Project,
		Writer:     os.Stderr,
	}
	if !noBuild {
		if err := bundler.Bundle(bundleCtx); err != nil {
			return nil, err
		}
	}
	logger.Debug("bundled in %s", time.Since(started))
	return bundler.CreateDeploymentMutator(bundleCtx), nil
}
