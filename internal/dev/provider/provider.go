package provider

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/shopmonkeyus/go-common/logger"
)

type Provider interface {
	Start() error
	Stop() error
	Restart() chan struct{}
	Done() chan struct{}
}

type providerFactory func(logger logger.Logger, dir string, env []string, args []string) (Provider, error)

var providers = map[string]providerFactory{}

func register(name string, factory providerFactory) {
	providers[name] = factory
}

func Get(name string, logger logger.Logger, dir string, env []string, args []string) (Provider, error) {
	factory, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return factory(logger, dir, env, args)
}

func uvExists() (string, bool, error) {
	fn, err := exec.LookPath("uv")
	if err != nil {
		return "", false, err
	}
	if fn == "" {
		return "", false, nil
	}
	return fn, true, nil
}

func getUVCommand(logger logger.Logger, uv string, dir string, args []string, env []string) *exec.Cmd {
	_ = logger
	cmdargs := []string{"run"}
	cmdargs = append(cmdargs, args...)
	cmd := exec.Command(uv, cmdargs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	// TODO: use logger
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
