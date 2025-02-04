// Package provider provides the interfaces for implementing providers.
package provider

import (
	"fmt"
	"path/filepath"

	"github.com/agentuity/cli/internal/project"
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

// Provider is the interface that is implemented by the provider to perform implementation specific logic.
type Provider interface {
	// Name will return the name of the provider in a format that is easy to use in a CLI.
	Name() string

	// Identifier will return the identifier of the provider in a format that is easy to use in a CLI.
	Identifier() string

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

// RunDev will run the development mode for the given provider.
// It will return the runner if it is found, otherwise it will return nil.
func RunDev(logger logger.Logger, dir string, apiUrl string, args []string) (Runner, error) {
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
	return provider.RunDev(logger, dir, envs, args)
}
