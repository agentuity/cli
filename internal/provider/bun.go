package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
)

// BunProvider is the provider implementation a generic Bun project.
//
// [Bun]: https://bun.sh
type BunProvider struct {
}

var _ Provider = (*BunProvider)(nil)

func (p *BunProvider) Name() string {
	return "Bun with Vercel AI SDK"
}

func (p *BunProvider) Identifier() string {
	return "bunjs"
}

func (p *BunProvider) Detect(logger logger.Logger, dir string, state map[string]any) (*Detection, error) {
	return nil, nil
}

func (p *BunProvider) RunDev(logger logger.Logger, dir string, env []string, args []string) (Runner, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *BunProvider) NewProject(logger logger.Logger, dir string, name string) error {
	logger = logger.WithPrefix("[bunjs]")
	bunjs, err := exec.LookPath("bun")
	if err != nil {
		return fmt.Errorf("bun not found in PATH")
	}
	if err := runCommandSilent(logger, bunjs, dir, []string{"init", "--yes"}, nil); err != nil {
		return fmt.Errorf("failed to run bun init: %w", err)
	}
	if err := runCommandSilent(logger, bunjs, dir, []string{"add", "@agentuity/sdk", "ai", "@ai-sdk/openai"}, nil); err != nil {
		return fmt.Errorf("failed to add npm modules: %w", err)
	}
	srcDir := filepath.Join(dir, p.DefaultSrcDir())
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "myfirstagent"), 0755); err != nil {
		return fmt.Errorf("failed to create src/myfirstagent directory: %w", err)
	}
	indexts := filepath.Join(srcDir, "myfirstagent", "index.ts")
	if err := os.WriteFile(indexts, []byte(jstemplate), 0644); err != nil {
		return fmt.Errorf("failed to write index.ts to %s: %w", indexts, err)
	}
	os.Remove(filepath.Join(dir, "index.ts"))
	boot := filepath.Join(dir, "index.js")
	if err := os.WriteFile(boot, []byte(bootTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", boot, err)
	}
	projectJSON, err := loadPackageJSON(dir)
	if err != nil {
		return fmt.Errorf("failed to load package.json from %s. %w", dir, err)
	}
	projectJSON.AddScript("build", "agentuity bundle")
	projectJSON.AddScript("prestart", "agentuity bundle")
	projectJSON.AddScript("start", "bun run .agentuity/index.js")
	projectJSON.SetMain("index.js")
	projectJSON.SetType("module")
	projectJSON.SetName(name)
	projectJSON.SetVersion("0.0.1")
	projectJSON.SetDescription("A simple Agentuity Agent project with the Vercel AI SDK")
	projectJSON.SetKeywords([]string{"agent", "agentuity", "ai", "vercel", "bun", "ai agent"})
	if err := projectJSON.Write(dir); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}
	ts, err := loadTSConfig(dir)
	if err != nil {
		return fmt.Errorf("failed to load tsconfig.json: %w", err)
	}
	ts.AddTypes("bun", "@agentuity/sdk")
	ts.AddCompilerOption("esModuleInterop", true)
	if err := ts.Write(dir); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %w", err)
	}
	if err := addAgentuityBuildToGitignore(dir); err != nil {
		return fmt.Errorf("failed to add agentuity build to .gitignore: %w", err)
	}
	return nil
}

func (p *BunProvider) ProjectIgnoreRules() []string {
	return []string{"node_modules/**", "dist/**", "src/**"}
}

func (p *BunProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "javascript"
	config.Runtime = "bunjs"
	config.Command = []string{"sh", "/app/.agentuity/run.sh"}
	return nil
}

func (p *BunProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	if err := detectModelTokens(logger, data, data.Dir); err != nil {
		return fmt.Errorf("failed to detect model tokens: %w", err)
	}
	if err := BundleJS(logger, data.Project, data.Dir, true); err != nil {
		return fmt.Errorf("failed to bundle JS: %w", err)
	}
	return nil
}

func (p *BunProvider) Aliases() []string {
	return []string{"bun", "javascript", "typescript", "js"}
}

func (p *BunProvider) Language() string {
	return "js"
}

func (p *BunProvider) Framework() string {
	return "bunjs"
}

func (p *BunProvider) Runtime() string {
	return "bunjs"
}

func (p *BunProvider) DefaultSrcDir() string {
	return "src/agents"
}

func init() {
	register("bunjs", &BunProvider{})
}
