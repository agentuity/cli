// Package provider provides the interfaces for implementing providers.
package provider

import (
	"fmt"
	"path/filepath"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
)

// Detection is the structure that is returned by the Detect function.
type Detection struct {
	Provider    string `json:"provider"`              // the name of the provider
	Name        string `json:"name,omitempty"`        // the optional name of the project
	Description string `json:"description,omitempty"` // the optional name of the description
	Version     string `json:"version,omitempty"`     // the optional version of the project
}

// Runner is the interface that is implemented by the provider for running the project.
type Runner interface {
	// Start will start the runner.
	Start() error
	// Stop will stop the runner.
	Stop() error
	// Restart will restart the runner.
	Restart() chan struct{}
	// Done will return a channel that is closed when the runner is done.
	Done() chan struct{}
}

type EnvFile struct {
	Filepath string
	Env      []env.EnvLine
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
	ShowSpinner func(logger logger.Logger, title string, action func())
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
	PromptForEnv func(logger logger.Logger, key string, isSecret bool, localenv map[string]string, osenv map[string]string) string
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

// Provider is the interface that is implemented by the provider to perform implementation specific logic.
type Provider interface {
	// Name will return the name of the provider in a format that is easy to use in a CLI.
	Name() string

	// Identifier will return the identifier of the provider in a format that is easy to use in a CLI.
	Identifier() string

	// Aliases will return any aliases for the provider for identifier alternatives.
	Aliases() []string

	// Detect will detect the provider for the given directory.
	// It will return the detection if it is found, otherwise it will return nil.
	Detect(logger logger.Logger, dir string, state map[string]any) (*Detection, error)

	// NewProject will create a new project for the given provider.
	NewProject(logger logger.Logger, dir string, name string) error

	// RunDev will run the development mode for the given provider.
	// It will return the runner if it is found, otherwise it will return nil.
	RunDev(logger logger.Logger, dir string, env []string, args []string) (Runner, error)

	// ProjectIgnoreRules should return any additional project specific deployment ignore rules.
	ProjectIgnoreRules() []string

	// DeployPreflightCheck is called before cloud deployment to allow the provider to perform any preflight checks.
	DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error

	// ConfigureDeploymentConfig will configure the deployment config for the given provider.
	ConfigureDeploymentConfig(config *project.DeploymentConfig) error
}

var providers = map[string]Provider{}

func register(name string, provider Provider) {
	providers[name] = provider
}

// GetProviders will return the registered providers.
func GetProviders() map[string]Provider {
	return providers
}

// GetProviderForName returns a provider registered as name or returns an error
func GetProviderForName(name string) (Provider, error) {
	if p, ok := providers[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("no provider registered: %s", name)
}

// Detect will detect the provider for the given directory.
// It will return the detection if it is found, otherwise it will return nil.
func Detect(logger logger.Logger, dir string) (*Detection, error) {
	state := map[string]any{}
	for _, provider := range providers {
		detection, err := provider.Detect(logger, dir, state)
		if err != nil {
			return nil, err
		}
		if detection != nil {
			return detection, nil
		}
	}
	return nil, nil
}

// NewRunner will create a new runner for the given provider.
// It will return the runner if it is found, otherwise it will return nil.
func NewRunner(logger logger.Logger, dir string, apiUrl string, eventLogFile string, args []string) (Runner, error) {
	project := project.NewProject()
	if err := project.Load(dir); err != nil {
		return nil, err
	}
	if project.Provider == "" {
		return nil, fmt.Errorf("no provider found in the agentuity.yaml file")
	}
	provider, ok := providers[project.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %s not registered", project.Provider)
	}
	envlines, err := env.ParseEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		return nil, err
	}
	var envs []string
	var apiFound bool
	for _, line := range envlines {
		envs = append(envs, env.EncodeOSEnv(line.Key, line.Val))
		if line.Key == "AGENTUITY_URL" {
			apiFound = true
		}
	}
	if !apiFound {
		envs = append(envs, env.EncodeOSEnv("AGENTUITY_URL", apiUrl))
	}
	if eventLogFile != "" {
		envs = append(envs, env.EncodeOSEnv("AGENTUITY_TRACE_LOG", eventLogFile))
	}
	envs = append(envs, env.EncodeOSEnv("AGENTUITY_LOG_LEVEL", "error"))

	return provider.RunDev(logger, dir, envs, args)
}
