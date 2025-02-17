package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
)

// NodeJSProvider is the provider implementation a generic Node project.
//
// [Node]: https://nodejs.org
type NodeJSProvider struct {
}

var _ Provider = (*NodeJSProvider)(nil)

func (p *NodeJSProvider) Name() string {
	return "NodeJS with Vercel AI SDK"
}

func (p *NodeJSProvider) Identifier() string {
	return "nodejs"
}

func (p *NodeJSProvider) Detect(logger logger.Logger, dir string, state map[string]any) (*Detection, error) {
	return nil, nil
}

func (p *NodeJSProvider) RunDev(logger logger.Logger, dir string, env []string, args []string) (Runner, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *NodeJSProvider) NewProject(logger logger.Logger, dir string, name string) error {
	logger = logger.WithPrefix("[nodejs]")
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found in PATH")
	}
	projectJSON, err := loadPackageJSON(dir)
	if err != nil {
		return fmt.Errorf("failed to load package.json from %s. %w", dir, err)
	}
	projectJSON.AddScript("build", "agentuity bundle -r node")
	projectJSON.AddScript("prestart", "agentuity bundle -r node")
	projectJSON.AddScript("start", "node .agentuity/index.js")
	projectJSON.SetMain("src/index.js")
	projectJSON.SetType("module")
	projectJSON.SetName(name)
	projectJSON.SetVersion("0.0.1")
	projectJSON.SetDescription("A simple Agentuity Agent project with the Vercel AI SDK")
	projectJSON.SetKeywords([]string{"agent", "agentuity", "ai", "vercel"})
	if err := projectJSON.Write(dir); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}
	if err := runCommandSilent(logger, npm, dir, []string{"install", "@agentuity/sdk", "ai", "@ai-sdk/openai"}, nil); err != nil {
		return fmt.Errorf("failed to add npm modules: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte(jstemplate), 0644); err != nil {
		return fmt.Errorf("failed to write index.ts: %w", err)
	}
	ts, err := loadTSConfig(dir)
	if err != nil {
		return fmt.Errorf("failed to load tsconfig.json: %w", err)
	}
	ts.AddTypes("node", "@agentuity/sdk")
	ts.AddCompilerOption("esModuleInterop", true)
	if err := ts.Write(dir); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %w", err)
	}
	if err := addAgentuityBuildToGitignore(dir); err != nil {
		return fmt.Errorf("failed to add agentuity build to .gitignore: %w", err)
	}
	return nil
}

func (p *NodeJSProvider) ProjectIgnoreRules() []string {
	return []string{"node_modules/**", "dist/**", "src/**"}
}

func (p *NodeJSProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "javascript"
	config.Runtime = "nodejs"
	config.Command = []string{"sh", "/app/.agentuity/run.sh"}
	return nil
}

var openAICheck = regexp.MustCompile(`openai\("([\w-]+)"\)`) // TODO: need to expand this

func (p *NodeJSProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	buf, _ := os.ReadFile(filepath.Join(data.Dir, "index.ts"))
	str := string(buf)
	if openAICheck.MatchString(str) {
		tok := openAICheck.FindStringSubmatch(str)
		if len(tok) != 2 {
			return fmt.Errorf("failed to find openai token in index.ts")
		}
		model := tok[1]
		if err := validateModelSecretSet(logger, data, model); err != nil {
			return fmt.Errorf("failed to validate model secret: %w", err)
		}
	}

	if err := BundleJS(logger, data.Dir, "node", true); err != nil {
		return fmt.Errorf("failed to bundle JS: %w", err)
	}

	return nil
}

func (p *NodeJSProvider) Aliases() []string {
	return []string{"node"}
}

func init() {
	register("nodejs", &NodeJSProvider{})
}
