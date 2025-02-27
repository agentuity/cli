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
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/marcozac/go-jsonc"
)

const (
	MyFirstAgentName        = "MyFirstAgent"
	MyFirstAgentDescription = "A simple agent that can generate text"
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

func runCommandSilent(logger logger.Logger, bin string, dir string, args []string, env []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = append(env, os.Environ()...)
	logger.Debug("running %s with env: %s in directory: %s and args: %s", bin, strings.Join(cmd.Env, " "), dir, strings.Join(args, " "))
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

type tsconfig map[string]any

func (t tsconfig) AddCompilerOption(key string, val any) {
	if co, ok := t["compilerOptions"].(map[string]any); ok {
		co[key] = val
	} else {
		co = make(map[string]any)
		co[key] = val
		t["compilerOptions"] = co
	}
}

func (t tsconfig) AddTypes(vals ...string) {
	if co, ok := t["compilerOptions"].(map[string]any); ok {
		if types, ok := co["types"].([]string); ok {
			co["types"] = append(types, vals...)
		} else {
			co["types"] = vals
		}
	} else {
		co = make(map[string]any)
		co["types"] = vals
		t["compilerOptions"] = co
	}
}

func (t tsconfig) Write(dir string) error {
	fn := filepath.Join(dir, "tsconfig.json")
	content, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fn, content, 0644)
}

func loadTSConfig(dir string) (tsconfig, error) {
	fn := filepath.Join(dir, "tsconfig.json")
	content, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	ts := make(tsconfig)
	if err := jsonc.Unmarshal(content, &ts); err != nil {
		return nil, err
	}
	return ts, nil
}

func searchBackwards(contents string, offset int, val byte) int {
	for i := offset; i >= 0; i-- {
		if contents[i] == val {
			return i
		}
	}
	return -1
}

type patchModule struct {
	Module    string                 `json:"module"`
	Functions map[string]patchAction `json:"functions"`
}

type patchAction struct {
	Before string
	After  string
}

func generateJSArgsPatch(index int, inject string) string {
	return fmt.Sprintf(`const _newargs = [...(_args ?? [])];
_newargs[%[1]d] = {..._newargs[%[1]d], %[2]s};
_args = _newargs;`, index, inject)
}

func generateEnvGuard(name string, inject string) string {
	return fmt.Sprintf(`if (!process.env.%[1]s) {
%[2]s
}`, name, inject)
}

func generateVercelAIProvider(name string) string {
	return generateJSArgsPatch(0, "") + fmt.Sprintf(`const opts = {...(_args[0] ?? {}) };
if (!opts.baseURL) {
	const apikey = process.env.AGENTUITY_API_KEY;
	const url = process.env.AGENTUITY_URL;
	if (url && apikey) {
		opts.apiKey = 'x';
		opts.baseURL = url + '/sdk/gateway/%s';
		opts.headers = {
			...(opts.headers ?? {}),
			Authorization: 'Bearer ' + apikey,
		};
		_args[0] = opts;
	}
}`, name)
}

var vercelTelemetryPatch = generateJSArgsPatch(0, `experimental_telemetry: { isEnabled: true }`)

var patches = map[string]patchModule{
	"@vercel/ai": {
		Module: "ai",
		Functions: map[string]patchAction{
			"generateText": {
				Before: vercelTelemetryPatch,
			},
			"streamText": {
				Before: vercelTelemetryPatch,
			},
			"generateObject": {
				Before: vercelTelemetryPatch,
			},
			"streamObject": {
				Before: vercelTelemetryPatch,
			},
			"embed": {
				Before: vercelTelemetryPatch,
			},
			"embedMany": {
				Before: vercelTelemetryPatch,
			},
		},
	},
	"@vercel/openai": {
		Module: "@ai-sdk/openai",
		Functions: map[string]patchAction{
			"createOpenAI": {
				Before: generateEnvGuard("OPENAI_API_KEY",
					generateVercelAIProvider("openai"),
				),
			},
		},
	},
}

func createPlugin(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "inject-agentuity",
		Setup: func(build api.PluginBuild) {
			for name, mod := range patches {
				build.OnLoad(api.OnLoadOptions{Filter: "node_modules/" + mod.Module + "/.*", Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					logger.Debug("re-writing %s for %s", args.Path, name)
					buf, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, err
					}
					contents := string(buf)
					var suffix strings.Builder
					for fn, mod := range mod.Functions {
						fnname := "function " + fn
						index := strings.Index(contents, fnname)
						if index == -1 {
							continue
						}
						eol := searchBackwards(contents, index, '\n')
						if eol < 0 {
							continue
						}
						prefix := strings.TrimSpace(contents[eol+1 : index])
						isAsync := strings.Contains(prefix, "async")
						newname := "__agentuity_" + fn
						newfnname := "function " + newname
						var fnprefix string
						if isAsync {
							fnprefix = "async "
						}
						contents = strings.Replace(contents, fnname, newfnname, 1)
						suffix.WriteString(fnprefix + fnname + "(...args) {\n")
						suffix.WriteString("\tlet _args = args;\n")
						if mod.Before != "" {
							suffix.WriteString(mod.Before)
							suffix.WriteString("\n")
						}
						suffix.WriteString("\tlet result = " + newname + "(..._args);\n")
						if isAsync {
							suffix.WriteString("\tif (result instanceof Promise) {\n")
							suffix.WriteString("\t\tresult = await result;\n")
							suffix.WriteString("\t}\n")
						}
						if mod.After != "" {
							suffix.WriteString(mod.After)
							suffix.WriteString("\n")
						}
						suffix.WriteString("\treturn result;\n")
						suffix.WriteString("}\n")
						logger.Debug("patched %s -> %s", name, fn)
					}
					contents = contents + "\n" + suffix.String()
					loader := api.LoaderJS
					if strings.HasSuffix(args.Path, ".ts") {
						loader = api.LoaderTS
					}
					return api.OnLoadResult{
						Contents: &contents,
						Loader:   loader,
					}, nil
				})
			}
		},
	}
}

type AgentConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

func addAgentuityBuildToGitignore(dir string) error {
	fn := filepath.Join(dir, ".gitignore")
	var contents string
	if sys.Exists(fn) {
		buf, err := os.ReadFile(fn)
		if err != nil {
			return fmt.Errorf("failed to read .gitignore: %w", err)
		}
		contents = string(buf)
	}
	contents += "\n# don't commit the agentuity build folder\n"
	contents += ".agentuity\n"
	return os.WriteFile(fn, []byte(contents), 0644)
}

func BundleJS(logger logger.Logger, project *project.Project, dir string, production bool) error {
	outdir := filepath.Join(dir, ".agentuity")
	if sys.Exists(outdir) {
		if err := os.RemoveAll(outdir); err != nil {
			return fmt.Errorf("failed to remove .agentuity directory: %w", err)
		}
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create .agentuity directory: %w", err)
	}
	var entryPoints []string
	entryPoints = append(entryPoints, filepath.Join(dir, "index.js"))
	files, err := util.ListDir(project.Bundler.AgentConfig.Dir)
	if err != nil {
		return fmt.Errorf("failed to list src directory: %w", err)
	}
	for _, file := range files {
		if filepath.Base(file) == "index.ts" {
			entryPoints = append(entryPoints, file)
		}
	}
	if len(entryPoints) == 0 {
		return fmt.Errorf("no index.ts files found in %s", project.Bundler.AgentConfig.Dir)
	}
	pkg, err := loadPackageJSON(dir)
	if err != nil {
		return fmt.Errorf("failed to load package.json: %w", err)
	}
	pkg2, err := loadPackageJSON(filepath.Join(dir, "node_modules", "@agentuity", "sdk"))
	if err != nil {
		return fmt.Errorf("failed to load package.json: %w", err)
	}
	defines := map[string]string{
		"process.env.AGENTUITY_CLI_VERSION":     fmt.Sprintf("'%s'", project.Bundler.CLIVersion),
		"process.env.AGENTUITY_SDK_APP_NAME":    fmt.Sprintf("'%s'", pkg["name"]),
		"process.env.AGENTUITY_SDK_APP_VERSION": fmt.Sprintf("'%s'", pkg["version"]),
		"process.env.AGENTUITY_SDK_VERSION":     fmt.Sprintf("'%s'", pkg2["version"]),
	}
	defines["process.env.AGENTUITY_BUNDLER_RUNTIME"] = fmt.Sprintf("'%s'", project.Bundler.Runtime)
	if production {
		defines["process.env.AGENTUITY_SDK_DEV_MODE"] = `"false"`
		defines["process.env.AGENTUITY_ENVIRONMENT"] = fmt.Sprintf("'%s'", "production")
		defines["process.env.NODE_ENV"] = fmt.Sprintf("'%s'", "production")
	} else {
		if val, ok := os.LookupEnv("AGENTUITY_ENVIRONMENT"); ok {
			defines["process.env.AGENTUITY_ENVIRONMENT"] = fmt.Sprintf("'%s'", val)
		} else {
			defines["process.env.AGENTUITY_ENVIRONMENT"] = fmt.Sprintf("'%s'", "development")
		}
	}
	var agents []AgentConfig
	for _, agent := range project.Agents {
		agents = append(agents, AgentConfig{
			ID:       agent.ID,
			Name:     agent.Name,
			Filename: filepath.Join(project.Bundler.AgentConfig.Dir, agent.Name, "index.js"),
		})
	}
	defines["process.env.AGENTUITY_CLOUD_AGENTS_JSON"] = fmt.Sprintf("'%s'", cstr.JSONStringify(agents))

	result := api.Build(api.BuildOptions{
		EntryPoints: entryPoints,
		Bundle:      true,
		Outdir:      outdir,
		Write:       true,
		Splitting:   false,
		Sourcemap:   api.SourceMapExternal,
		Format:      api.FormatESModule,
		Platform:    api.PlatformNode,
		Engines: []api.Engine{
			{Name: api.EngineNode, Version: "22"},
		},
		AbsWorkingDir: dir,
		TreeShaking:   api.TreeShakingTrue,
		Drop:          api.DropDebugger,
		Plugins:       []api.Plugin{createPlugin(logger)},
		Define:        defines,
		LegalComments: api.LegalCommentsNone,
		Banner: map[string]string{
			"js": "/* DO NOT EDIT - GENERATED CODE */",
		},
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
	return nil
}

func detectModelTokens(logger logger.Logger, data DeployPreflightCheckData, baseDir string) error {
	_ = logger
	_ = data
	_ = baseDir
	/*files, err := sys.ListDir(filepath.Join(baseDir, "src"))
	if err != nil {
		return fmt.Errorf("failed to list src directory: %w", err)
	}
	if len(files) > 0 {
		var validated bool
		for _, file := range files {
			if filepath.Ext(file) == ".ts" {
				buf, _ := os.ReadFile(file)
				str := string(buf)
				// TODO: expand this to all models
				if openAICheck.MatchString(str) {
					tok := openAICheck.FindStringSubmatch(str)
					if len(tok) != 2 {
						return fmt.Errorf("failed to find model token in %s", file)
					}
					model := tok[1]
					if err := validateModelSecretSet(logger, data, model); err != nil {
						return fmt.Errorf("failed to validate model secret: %w", err)
					}
					validated = true
				}
			}
			if validated {
				break
			}
		}
	}*/
	return nil
}

func generateJSAgentTemplate(dir string, srcdir string, name string) error {
	srcDir := filepath.Join(dir, srcdir)
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}
	filename := util.SafeFilename(name)
	if err := os.MkdirAll(filepath.Join(srcDir, filename), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Join(srcDir, name), err)
	}
	indexts := filepath.Join(srcDir, filename, "index.ts")
	if err := generateJSAgent(indexts); err != nil {
		return fmt.Errorf("failed to generate file %s: %w", indexts, err)
	}
	return nil
}

func generateJSAgent(filename string) error {
	return os.WriteFile(filename, []byte(jstemplate), 0644)
}

const jstemplate = `import type {
	AgentRequest,
	AgentResponse,
	AgentContext,
} from "@agentuity/sdk";
import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	const res = await generateText({
		model: openai("gpt-4o"),
		system: "You are a friendly assistant!",
		prompt: req.text() ?? "Why is the sky blue?",
	});
	return resp.text(res.text);
}
`

const bootTemplate = `import { runner } from "@agentuity/sdk";

runner(true, import.meta.dirname).catch((err) => {
	console.error(err);
	process.exit(1);
});
`

const nodeJSTSConfig = `{
  "compilerOptions": {
    "allowImportingTsExtensions": true,
    "allowJs": true,
    "esModuleInterop": true,
    "lib": [
      "ESNext",
      "DOM"
    ],
    "module": "ESNext",
    "moduleDetection": "force",
    "moduleResolution": "node",
    "noEmit": true,
    "noFallthroughCasesInSwitch": true,
    "noPropertyAccessFromIndexSignature": false,
    "noUnusedLocals": false,
    "noUnusedParameters": false,
    "skipLibCheck": true,
    "strict": true,
    "target": "ESNext",
    "types": [
      "node",
      "@agentuity/sdk"
    ],
    "verbatimModuleSyntax": true
  }
}`
