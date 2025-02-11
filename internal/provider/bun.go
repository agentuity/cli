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
	return "Bun"
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

const template = `import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

const res = await generateText({
	model: openai("gpt-4o"),
	system: "You are a friendly assistant!",
	prompt: "Why is the sky blue?",
});

console.log(res.text);
`

func (p *BunProvider) NewProject(logger logger.Logger, dir string, name string) error {
	logger = logger.WithPrefix("[bunjs]")
	bunjs, err := exec.LookPath("bun")
	if err != nil {
		return fmt.Errorf("bun not found in PATH")
	}
	if err := runBunCommand(logger, bunjs, dir, []string{"init", "--yes"}, nil); err != nil {
		return fmt.Errorf("failed to run bun init: %w", err)
	}
	toml := `preload = ["@agentuity/sdk"]`
	if err := os.WriteFile(filepath.Join(dir, "bunfig.toml"), []byte(toml+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write bun.toml: %w", err)
	}
	if err := runBunCommand(logger, bunjs, dir, []string{"add", "@agentuity/sdk", "ai", "@ai-sdk/openai"}, nil); err != nil {
		return fmt.Errorf("failed to add npm modules: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.ts"), []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write index.ts: %w", err)
	}
	return nil
}

func (p *BunProvider) ProjectIgnoreRules() []string {
	return []string{"node_modules/**", "dist/**"}
}

func (p *BunProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "javascript"
	config.Command = []string{"bun", "run", "index.ts"}
	return nil
}

func (p *BunProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	return nil
}

func init() {
	register("bunjs", &BunProvider{})
}
