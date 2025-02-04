package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/cli/internal/util"
	"github.com/shopmonkeyus/go-common/logger"
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
	uv, ok, err := uvExists()
	if err != nil {
		return err
	}
	if ok {
		if err := runUVNewVirtualEnv(uv, dir); err != nil {
			return err
		}
		env := []string{
			"VIRTUAL_ENV=" + filepath.Join(dir, ".venv"),
			"PATH=" + filepath.Join(dir, ".venv", "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		}
		logger.Debug("adding crewai to virtual environment using: %s", strings.Join(env, " "))
		if err := runUVCommand(uv, dir, []string{"pip", "install", "crewai"}, env); err != nil {
			return fmt.Errorf("failed to install crewai: %w", err)
		}
		if err := runUVCommand(uv, dir, []string{"run", "crewai", "create", "crew", name}, env); err != nil {
			return fmt.Errorf("failed to create crew: %w", err)
		}
		srcDir := filepath.Join(dir, name) // because create nests directories we need to unnest
		if err := util.CopyDir(srcDir, dir); err != nil {
			return fmt.Errorf("failed to copy crew from %s to %s: %w", srcDir, dir, err)
		}
		if err := os.RemoveAll(srcDir); err != nil {
			return fmt.Errorf("failed to remove crew folder: %w", err)
		}
		if err := runUVCommand(uv, dir, []string{"add", "agentuity"}, env); err != nil {
			return fmt.Errorf("failed to add agentuity: %w", err)
		}
		mainFile := filepath.Join(dir, "src", name, "main.py")
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

func init() {
	register("crewai", &CrewAIProvider{})
}
