package dev

import (
	"fmt"
	"path/filepath"

	"github.com/agentuity/cli/internal/dev/provider"
	"github.com/agentuity/cli/internal/env"
	"github.com/agentuity/cli/internal/project"
	"github.com/shopmonkeyus/go-common/logger"
)

func NewProvider(logger logger.Logger, dir string, args []string, apiUrl string) (provider.Provider, error) {
	project := project.NewProject()
	if err := project.Load(dir); err != nil {
		return nil, err
	}
	if project.Provider == "" {
		return nil, fmt.Errorf("no provider found in the agentuity.yaml file")
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
	provider, err := provider.Get(project.Provider, logger, dir, envs, args)
	if err != nil {
		return nil, err
	}
	return provider, nil
}
