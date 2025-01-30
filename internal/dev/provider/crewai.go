package provider

import (
	"os/exec"
	"sync"
	"syscall"

	"github.com/shopmonkeyus/go-common/logger"
)

type CrewAIProvider struct {
	logger  logger.Logger
	dir     string
	env     []string
	args    []string
	cmd     *exec.Cmd
	restart chan struct{}
	done    chan struct{}
	once    sync.Once
}

var _ Provider = (*CrewAIProvider)(nil)

func (p *CrewAIProvider) Restart() chan struct{} {
	return p.restart
}

func (p *CrewAIProvider) Done() chan struct{} {
	return p.done
}

func (p *CrewAIProvider) Start() error {
	if fn, ok, err := uvExists(); err != nil {
		return err
	} else if ok {
		cmdargs := []string{"crewai", "run"}
		cmdargs = append(cmdargs, p.args...)
		p.cmd = getUVCommand(p.logger, fn, p.dir, cmdargs, p.env)
		if err := p.cmd.Start(); err != nil {
			return err
		}
	}
	if p.cmd != nil {
		go func() {
			p.cmd.Wait()
			p.done <- struct{}{}
		}()
	}
	// FIXME: fallback to python
	return nil
}

func (p *CrewAIProvider) Stop() error {
	p.once.Do(func() {
		if p.cmd != nil {
			p.logger.Debug("killing crewai process")
			p.cmd.Process.Signal(syscall.SIGTERM)
			p.cmd.Process.Kill()
			p.cmd = nil
		}
	})
	return nil
}

func init() {
	register("crewai", func(logger logger.Logger, dir string, env []string, args []string) (Provider, error) {
		return &CrewAIProvider{
			logger:  logger,
			dir:     dir,
			env:     env,
			args:    args,
			restart: make(chan struct{}),
			done:    make(chan struct{}),
		}, nil
	})
}
