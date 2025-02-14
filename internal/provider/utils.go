package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/sys"
	"github.com/evanw/esbuild/pkg/api"
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

type writerLogger struct {
	logger logger.Logger
}

func (w *writerLogger) Write(p []byte) (int, error) {
	w.logger.Info(string(p))
	return len(p), nil
}

func newWriterLogger(logger logger.Logger) io.Writer {
	return &writerLogger{logger: logger}
}

// getUVCommand will get the uv command for the given directory.
// It will return the command if it is found, otherwise it will return nil.
func getUVCommand(logger logger.Logger, uv string, dir string, args []string, env []string) *exec.Cmd {
	cmdargs := []string{"run"}
	cmdargs = append(cmdargs, args...)
	cmd := exec.Command(uv, cmdargs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = newWriterLogger(logger.With(map[string]interface{}{"source": "uv.stdout"}))
	cmd.Stderr = newWriterLogger(logger.With(map[string]interface{}{"source": "uv.stderr"}))
	return cmd
}

func runCommand(logger logger.Logger, bin string, dir string, args []string, env []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(env, os.Environ()...)
	logger.Debug("running %s with env: %s in directory: %s and args: %s", bin, strings.Join(cmd.Env, " "), dir, strings.Join(args, " "))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createUVNewVirtualEnv(logger logger.Logger, uv string, dir string, version string) ([]string, error) {
	venv := filepath.Join(dir, ".venv")
	if err := runCommand(logger, uv, dir, []string{"venv", venv, "--python", version}, nil); err != nil {
		return nil, fmt.Errorf("failed to create virtual environment: %w", err)
	}
	bin := filepath.Join(venv, "bin")
	env := []string{
		"VIRTUAL_ENV=" + venv,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	return env, nil
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

type modelMatcher struct {
	SecretName string
	Matcher    *regexp.Regexp
}

var modelMatcherRules = []modelMatcher{
	{"OPENROUTER_API_KEY", regexp.MustCompile(`openrouter`)},
	{"GITHUB_API_KEY", regexp.MustCompile(`github`)},
	{"LM_STUDIO_API_KEY", regexp.MustCompile(`lm_studio`)},
	{"AZURE_API_KEY", regexp.MustCompile(`azure\/`)},
	{"DATABRICKS_API_KEY", regexp.MustCompile(`databricks`)},
	{"COHERE_API_KEY", regexp.MustCompile(`command-`)},
	{"OPENAI_API_KEY", regexp.MustCompile(`^(gpt|o\d+)-`)},
	{"GEMINI_API_KEY", regexp.MustCompile(`gemini`)},
	{"ANTHROPIC_API_KEY", regexp.MustCompile(`claude`)},
	{"AZURE_AI_API_KEY", regexp.MustCompile(`azure_ai`)},
	{"PERPLEXITYAI_API_KEY", regexp.MustCompile(`perplexity`)},
	{"MISTRAL_API_KEY", regexp.MustCompile(`mistral`)},
	{"HUGGINGFACE_API_KEY", regexp.MustCompile(`huggingface`)},
	{"WATSONX_APIKEY", regexp.MustCompile(`watsonx`)},
	{"NVIDIA_NIM_API_KEY", regexp.MustCompile(`nvidia_nim`)},
	{"XAI_API_KEY", regexp.MustCompile(`xai|grok`)},
	{"GROQ_API_KEY", regexp.MustCompile(`groq`)},
	{"CLOUDFLARE_API_KEY", regexp.MustCompile(`cloudflare`)},
	{"DEEPSEEK_API_KEY", regexp.MustCompile(`deepseek`)},
	{"FIREWORKS_AI_API_KEY", regexp.MustCompile(`fireworks_ai`)},
	{"REPLICATE_API_KEY", regexp.MustCompile(`replicate`)},
	{"TOGETHERAI_API_KEY", regexp.MustCompile(`together_ai`)},
	{"VOYAGE_API_KEY", regexp.MustCompile(`voyage`)},
	{"SAMBANOVA_API_KEY", regexp.MustCompile(`sambanova`)},
	{"BASETEN_API_KEY", regexp.MustCompile(`baseten`)},
	{"ALEPHALPHA_API_KEY", regexp.MustCompile(`luminous`)},
	{"JINA_AI_API_KEY", regexp.MustCompile(`jina_ai`)},
}

func validateModelSecretSet(logger logger.Logger, data DeployPreflightCheckData, modelName string) error {
	for _, m := range modelMatcherRules {
		if m.Matcher.MatchString(modelName) {
			if val, found := data.ProjectData.Secrets[m.SecretName]; !found || val == "" {
				if err := promptToSetSecret(logger, data, m.SecretName, "You are using the model "+modelName+" but you haven't set the "+m.SecretName+" secret to your project. Would you like to set it?"); err != nil {
					return err
				}
			}
			break
		}
	}
	return nil
}

func promptToSetSecret(logger logger.Logger, data DeployPreflightCheckData, secretName string, prompt string) error {
	if _, found := data.ProjectData.Secrets[secretName]; !found {
		var warn bool
		// we haven't set the openai key but we've selected an openai model
		if !data.PromptHelpers.Ask(logger, prompt, true) {
			warn = true
		} else {
			secret := data.PromptHelpers.PromptForEnv(logger, secretName, true, nil, data.OSEnvironment)
			if secret == "" {
				warn = true
			} else {
				pd, err := data.Project.SetProjectEnv(logger, data.APIURL, data.APIKey, nil, map[string]string{secretName: secret})
				if err != nil {
					return err
				}
				data.ProjectData = pd
			}
		}
		if warn {
			data.PromptHelpers.PrintWarning("Your project will likely not run since the %s will not be set in your deployment", secretName)
			return nil
		}
	}
	return nil
}

type packageJSONFile map[string]any

func (p packageJSONFile) AddScript(name string, command string) {
	kv, found := p["scripts"].(map[string]any)
	if !found {
		kv = make(map[string]any)
		p["scripts"] = kv
	}
	kv[name] = command
}

func (p packageJSONFile) RemoveScript(name string) {
	if kv, found := p["scripts"].(map[string]any); found {
		delete(kv, name)
	}
}

func (p packageJSONFile) SetMain(main string) {
	p["main"] = main
}

func (p packageJSONFile) SetType(typ string) {
	p["type"] = typ
}

func (p packageJSONFile) SetDependencies(dependencies []string) {
	p["dependencies"] = dependencies
}

func (p packageJSONFile) SetName(name string) {
	p["name"] = name
}

func (p packageJSONFile) SetVersion(version string) {
	p["version"] = version
}

func (p packageJSONFile) SetDescription(description string) {
	p["description"] = description
}

func (p packageJSONFile) SetKeywords(keywords []string) {
	p["keywords"] = keywords
}

func (p packageJSONFile) Write(dir string) error {
	fn := filepath.Join(dir, "package.json")
	content, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fn, content, 0644)
}

func loadPackageJSON(dir string) (packageJSONFile, error) {
	fn := filepath.Join(dir, "package.json")
	pkg := make(packageJSONFile)
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return pkg, nil
	}
	content, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func BundleJS(logger logger.Logger, dir string, runtime string, production bool) error {
	outdir := filepath.Join(dir, ".agentuity")
	if sys.Exists(outdir) {
		if err := os.RemoveAll(outdir); err != nil {
			return fmt.Errorf("failed to remove .agentuity directory: %w", err)
		}
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create .agentuity directory: %w", err)
	}
	result := api.Build(api.BuildOptions{
		EntryPoints:   []string{"index.ts"},
		Bundle:        true,
		Outdir:        outdir,
		Write:         true,
		Format:        api.FormatESModule,
		Platform:      api.PlatformNode,
		External:      []string{"@agentuity/sdk"},
		AbsWorkingDir: dir,
	})
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			if err.Location != nil {
				logger.Error("failed to bundle %s (line %d): %s", err.Location.File, err.Location.Line, err.Text)
				continue
			}
			logger.Error("failed to bundle: %s", err.Text)
		}
		return fmt.Errorf("failed to bundle JS")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("node not found in PATH")
	}
	if production {
		pkg, err := loadPackageJSON(dir)
		if err != nil {
			return fmt.Errorf("failed to load package.json: %w", err)
		}
		pkg.RemoveScript("build")
		pkg.RemoveScript("prestart")
		var bin string
		var script string
		if runtime == "bunjs" {
			bin = "bun"
			script = `#!/bin/bash
			set -e
			cd .agentuity
			bun install && bun start
			`
		} else {
			bin = "node"
			script = `#!/bin/bash
			set -e
			cd .agentuity
			npm install && node index.js
			`
		}
		pkg.AddScript("start", bin+" index.js")
		pkg.Write(outdir)
		if err := os.WriteFile(filepath.Join(outdir, "run.sh"), []byte(script), 0644); err != nil {
			return fmt.Errorf("failed to write run.sh: %w", err)
		}
		if err := os.Chmod(filepath.Join(outdir, "run.sh"), 0755); err != nil {
			return fmt.Errorf("failed to chmod+x run.sh: %w", err)
		}
	}
	script := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "dist", "instrumentation", "bundler.js")
	if err := runCommand(logger, node, outdir, []string{script}, nil); err != nil {
		return fmt.Errorf("failed to run agentuity-builder: %w", err)
	}
	return nil
}

const jstemplate = `import { getInput, setOutput } from "@agentuity/sdk";
import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

const input = await getInput();

const res = await generateText({
	model: openai("gpt-4o"),
	system: "You are a friendly assistant!",
	prompt: input?.payload ?? "Why is the sky blue?",
});

console.log(res.text);
await setOutput(res.text, "text/plain");
`
