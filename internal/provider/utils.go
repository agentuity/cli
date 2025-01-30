package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/shopmonkeyus/go-common/logger"
)

// PyProject is the structure that is used to parse the pyproject.toml file.
type PyProject struct {
	Name           string   `toml:"name"`
	Description    string   `toml:"description"`
	Version        string   `toml:"version"`
	RequiresPython string   `toml:"requires-python"`
	Dependencies   []string `toml:"dependencies"`
}

// readPyProject will read the pyproject.toml file and return the PyProject structure.
// It will return nil if the file is not found.
func readPyProject(dir string, state map[string]any) (*PyProject, error) {
	if val, ok := state["pyproject"].(*PyProject); ok {
		return val, nil
	}
	fn := filepath.Join(dir, "pyproject.toml")
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return nil, nil
	}
	content, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	var project PyProject
	if err := toml.Unmarshal(content, &project); err != nil {
		return nil, err
	}
	state["pyproject"] = &project
	return &project, nil
}

// detectPyProjectDependency will detect the provider for the given directory.
// It will return the detection if it is found, otherwise it will return nil.
func detectPyProjectDependency(dir string, state map[string]any, dependency string, provider string) (*Detection, error) {
	project, err := readPyProject(dir, state)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, nil
	}
	for _, dep := range project.Dependencies {
		if strings.Contains(dep, dependency) {
			return &Detection{Provider: provider, Name: project.Name, Description: project.Description, Version: project.Version}, nil
		}
	}
	return nil, nil
}

// uvExists will check if the uv command is installed.
// It will return the path to the uv command if it is installed, otherwise it will return an empty string.
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

// getUVCommand will get the uv command for the given directory.
// It will return the command if it is found, otherwise it will return nil.
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

func runUVCommand(uv string, dir string, args []string) error {
	cmd := exec.Command(uv, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runUVNewVirtualEnv(uv string, dir string) error {
	return runUVCommand(uv, dir, []string{"venv", filepath.Join(dir, ".venv")})
}

// PythonRunner is the runner implementation for python projects.
type PythonRunner struct {
	logger  logger.Logger
	dir     string
	env     []string
	args    []string
	cmd     *exec.Cmd
	restart chan struct{}
	done    chan struct{}
	once    sync.Once
}

var _ Runner = (*PythonRunner)(nil)

func (p *PythonRunner) Restart() chan struct{} {
	return p.restart
}

func (p *PythonRunner) Done() chan struct{} {
	return p.done
}

func (p *PythonRunner) Start() error {
	if fn, ok, err := uvExists(); err != nil {
		return err
	} else if ok {
		p.cmd = getUVCommand(p.logger, fn, p.dir, p.args, p.env)
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

func (p *PythonRunner) Stop() error {
	p.once.Do(func() {
		if p.cmd != nil {
			p.logger.Debug("killing process")
			p.cmd.Process.Signal(syscall.SIGTERM)
			p.cmd.Process.Kill()
			p.cmd = nil
		}
	})
	return nil
}

// newPythonRunner will create a new PythonRunner and will start the process using either uv or python.
func newPythonRunner(logger logger.Logger, dir string, env []string, args []string) *PythonRunner {
	return &PythonRunner{
		logger:  logger,
		dir:     dir,
		env:     env,
		args:    args,
		restart: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func patchImport(buf string, token string) (string, error) {
	i := strings.Index(buf, "import ")
	if i < 0 {
		return buf, fmt.Errorf("couldn't find any imports in this file")
	}

	// add our import
	before := buf[:i]
	after := buf[i:]
	buf = before + "import agentuity\n" + after

	i = strings.Index(buf, token)
	if i < 0 {
		return buf, fmt.Errorf("couldn't find %s in this file", token)
	}

	// patch in our init function
	before = buf[:i]
	after = buf[i:]
	buf = before + "agentuity.init()\n\n" + after

	return buf, nil
}
