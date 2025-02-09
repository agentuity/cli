package provider

import (
	"fmt"
	"os/exec"

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

func (p *BunProvider) NewProject(logger logger.Logger, dir string, name string) error {
	logger = logger.WithPrefix("[bunjs]")
	bunjs, err := exec.LookPath("bun")
	if err != nil {
		return fmt.Errorf("bun not found in PATH")
	}
	if err := runBunCommand(logger, bunjs, dir, []string{"init", "--yes"}, nil); err != nil {
		return fmt.Errorf("failed to run bun init: %w", err)
	}
	return nil
}

func (p *BunProvider) ProjectIgnoreRules() []string {
	return nil
}

func (p *BunProvider) ConfigureDeploymentConfig(config *project.DeploymentConfig) error {
	config.Language = "javascript"
	config.Command = []string{"bun", "run", "index.ts"}
	return nil
}

func init() {
	register("bunjs", &BunProvider{})
}
