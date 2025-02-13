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

const template = `import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

const res = await generateText({
	model: openai("gpt-4o"),
	system: "You are a friendly assistant!",
	prompt: "Why is the sky blue?",
});


console.log(res.text);
`

const runner = `#!/bin/sh
set -e
bun install && bun start
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
	if err := runBunCommand(logger, bunjs, dir, []string{"add", "@agentuity/sdk", "ai", "@ai-sdk/openai"}, nil); err != nil {
		return fmt.Errorf("failed to add npm modules: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.ts"), []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write index.ts: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentuity_runner.sh"), []byte(runner), 0644); err != nil {
		return fmt.Errorf("failed to write .agentuity_runner.sh: %w", err)
	}
	if err := os.Chmod(filepath.Join(dir, ".agentuity_runner.sh"), 0755); err != nil {
		return fmt.Errorf("failed to chmod .agentuity_runner.sh: %w", err)
	}
	projectJSON, err := loadPackageJSON(dir)
	if err != nil {
		return fmt.Errorf("failed to load package.json from %s. %w", dir, err)
	}
	projectJSON.AddScript("build", "agentuity-builder")
	projectJSON.AddScript("prestart", "bun run build")
	projectJSON.AddScript("start", "bun run .agentuity/index.js")
	if err := projectJSON.Write(dir); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}
	return nil
}

func (p *BunProvider) ProjectIgnoreRules() []string {
	return []string{"node_modules/**", "dist/**"}
}

func (p *BunProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "javascript"
	config.Runtime = "bunjs"
	config.Command = []string{".agentuity_runner.sh"}
	return nil
}

var openAICheck = regexp.MustCompile(`openai\("([\w-]+)"\)`) // TODO: need to expand this

func (p *BunProvider) DeployPreflightCheck(logger logger.Logger, data DeployPreflightCheckData) error {
	buf, _ := os.ReadFile(filepath.Join(data.Dir, "index.ts"))
	str := string(buf)
	if openAICheck.MatchString(str) {
		tok := openAICheck.FindStringSubmatch(str)
		if len(tok) != 2 {
			return fmt.Errorf("failed to find openai token in index.ts")
		}
		model := tok[1]
		return validateModelSecretSet(logger, data, model)
	}
	return nil
}

func init() {
	register("bunjs", &BunProvider{})
}
