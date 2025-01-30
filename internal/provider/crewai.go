package provider

import (
	"github.com/shopmonkeyus/go-common/logger"
)

// CrewAIProvider is the provider implementation for the [CrewAI] framework.
//
// [CrewAI]: https://github.com/crewAIInc/crewAI
type CrewAIProvider struct {
}

var _ Provider = (*CrewAIProvider)(nil)

func (p *CrewAIProvider) Detect(logger logger.Logger, dir string, state map[string]any) (*Detection, error) {
	return detectPyProjectDependency(dir, state, "crewai", "crewai")
}

func (p *CrewAIProvider) RunDev(logger logger.Logger, dir string, env []string, args []string) (Runner, error) {
	return newPythonRunner(logger, dir, env, append([]string{"crewai", "run"}, args...)), nil
}

func init() {
	register("crewai", &CrewAIProvider{})
}
