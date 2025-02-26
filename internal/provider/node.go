package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
)

const minNodeMajorVersion = 22

// NodeJSProvider is the provider implementation a generic Node project.
//
// [Node]: https://nodejs.org
type NodeJSProvider struct {
}

var _ Provider = (*NodeJSProvider)(nil)

func (p *NodeJSProvider) Name() string {
	return "NodeJS with Vercel AI SDK"
}

func (p *NodeJSProvider) Description() string {
	return "NodeJS is a runtime for JavaScript and TypeScript. This provider adds the Vercel AI SDK to a NodeJS project."
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
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("nodejs not found in PATH")
	}
	nv, err := exec.Command(node, "--version").Output()
	if err != nil {
		return fmt.Errorf("failed to get node version: %w", err)
	}
	nvbuf := strings.TrimSpace(string(nv))
	if !strings.HasPrefix(nvbuf, "v") {
		return fmt.Errorf("invalid node version: %s", nvbuf)
	}
	nvbuf = strings.TrimPrefix(nvbuf, "v")
	vertok := strings.Split(nvbuf, ".")
	if len(vertok) == 0 {
		return fmt.Errorf("invalid node version: %s", nvbuf)
	}
	major, err := strconv.Atoi(vertok[0])
	if err != nil {
		return fmt.Errorf("invalid node version: %s", nvbuf)
	}
	if major < minNodeMajorVersion {
		return fmt.Errorf("nodejs version must be %d or higher to use Agentuity. You can install the latest version of NodeJS at https://nodejs.org/en/download/", minNodeMajorVersion)
	}
	projectJSON, err := loadPackageJSON(dir)
	if err != nil {
		return fmt.Errorf("failed to load package.json from %s. %w", dir, err)
	}
	projectJSON.AddScript("build", "agentuity bundle")
	projectJSON.AddScript("prestart", "agentuity bundle")
	projectJSON.AddScript("start", "node --env-file .env .agentuity/index.js")
	projectJSON.SetMain("src/index.js")
	projectJSON.SetType("module")
	projectJSON.SetName(name)
	projectJSON.SetVersion("0.0.1")
	projectJSON.SetDescription("A simple Agentuity Agent project with the Vercel AI SDK")
	projectJSON.SetKeywords([]string{"agent", "agentuity", "ai", "vercel", "ai agent"})
	if err := projectJSON.Write(dir); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}
	if err := runCommandSilent(logger, npm, dir, []string{"install", "@agentuity/sdk", "ai", "@ai-sdk/openai"}, nil); err != nil {
		return fmt.Errorf("failed to add npm modules: %w", err)
	}
	if err := runCommandSilent(logger, npm, dir, []string{"install", "typescript", "@types/node", "-D"}, nil); err != nil {
		return fmt.Errorf("failed to add npm typescript modules: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
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
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(nodeJSTSConfig), 0644); err != nil {
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

func (p *NodeJSProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	if err := detectModelTokens(logger, data, data.Dir); err != nil {
		return fmt.Errorf("failed to detect model tokens: %w", err)
	}
	if err := BundleJS(logger, data.Project, data.Dir, true); err != nil {
		return fmt.Errorf("failed to bundle JS: %w", err)
	}
	return nil
}

func (p *NodeJSProvider) Aliases() []string {
	return []string{"node"}
}

func (p *NodeJSProvider) Language() string {
	return "js"
}

func (p *NodeJSProvider) Framework() string {
	return "nodejs"
}

func (p *NodeJSProvider) Runtime() string {
	return "nodejs"
}

func (p *NodeJSProvider) DefaultSrcDir() string {
	return "src/agents"
}

func init() {
	register("nodejs", &NodeJSProvider{})
}
