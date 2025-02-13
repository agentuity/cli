package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

// CrewAIProvider is the provider implementation for the [CrewAI] framework.
//
// [CrewAI]: https://github.com/crewAIInc/crewAI
type CrewAIProvider struct {
}

var _ Provider = (*CrewAIProvider)(nil)

func (p *CrewAIProvider) Name() string {
	return "CrewAI"
}

func (p *CrewAIProvider) Identifier() string {
	return "crewai"
}

func (p *CrewAIProvider) Detect(logger logger.Logger, dir string, state map[string]any) (*Detection, error) {
	return detectPyProjectDependency(dir, state, "crewai", "crewai")
}

func (p *CrewAIProvider) RunDev(logger logger.Logger, dir string, env []string, args []string) (Runner, error) {
	return newPythonRunner(logger, dir, env, append([]string{"crewai", "run"}, args...)), nil
}

func (p *CrewAIProvider) NewProject(logger logger.Logger, dir string, name string) error {
	logger = logger.WithPrefix("[crewai]")
	uv, ok, err := uvExists()
	if err != nil {
		return err
	}
	if ok {
		env, err := createUVNewVirtualEnv(logger, uv, dir, ">=3.10")
		if err != nil {
			return err
		}
		logger.Debug("adding crewai to virtual environment using: %s", strings.Join(env, " "))
		if err := runCommand(logger, uv, dir, []string{"pip", "install", "crewai"}, env); err != nil {
			return fmt.Errorf("failed to install crewai: %w", err)
		}
		dirname := filepath.Base(dir)
		if err := runCommand(logger, uv, dir, []string{"run", "crewai", "create", "crew", dirname}, env); err != nil {
			return fmt.Errorf("failed to create crew: %w", err)
		}
		srcDir := filepath.Join(dir, dirname) // because create nests directories we need to unnest
		if err := util.CopyDir(srcDir, dir); err != nil {
			return fmt.Errorf("failed to copy crew from %s to %s: %w", srcDir, dir, err)
		}
		if err := os.RemoveAll(srcDir); err != nil {
			return fmt.Errorf("failed to remove crew folder: %w", err)
		}
		if err := runCommand(logger, uv, dir, []string{"add", "agentuity"}, env); err != nil {
			return fmt.Errorf("failed to add agentuity: %w", err)
		}
		mainFile := filepath.Join(dir, "src", dirname, "main.py")
		buf, err := os.ReadFile(mainFile)
		if err != nil {
			return fmt.Errorf("failed to read main file: %w", err)
		}
		sbuf, err := patchImport(string(buf), "def run():")
		if err != nil {
			return fmt.Errorf("failed to patch import: %w", err)
		}
		if err := os.WriteFile(mainFile, []byte(sbuf), 0644); err != nil {
			return fmt.Errorf("failed to write main file: %w", err)
		}
	}
	return nil
}

func (p *CrewAIProvider) ProjectIgnoreRules() []string {
	return nil
}

func (p *CrewAIProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "python"
	config.Runtime = "uv"
	config.MinVersion = ">=3.10,<3.13"
	config.Command = []string{"run_crew"}
	return nil
}

func (p *CrewAIProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	if data.Envfile != nil {
		// detect if we have a MODEL set but the model's API Key hasn't been set
		if val, ok := data.Envfile.Lookup("MODEL"); ok && val != "" {
			return validateModelSecretSet(logger, data, val)
		}
	}
	return nil
}

func init() {
	register("crewai", &CrewAIProvider{})
}
