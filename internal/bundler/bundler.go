package bundler

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/evanw/esbuild/pkg/api"
)

var Version = "dev"

var ErrBuildFailed = fmt.Errorf("build failed")

type AgentConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type BundleContext struct {
	Context    context.Context
	Logger     logger.Logger
	ProjectDir string
	Production bool
	Install    bool
	CI         bool
	DevMode    bool
	Writer     io.Writer
}

func bundleJavascript(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	if ctx.Install || !util.Exists(filepath.Join(dir, "node_modules")) {
		var install *exec.Cmd
		switch theproject.Bundler.Runtime {
		case "nodejs":
			install = exec.CommandContext(ctx.Context, "npm", "install", "--no-save", "--no-audit", "--no-fund", "--include=prod", "--ignore-scripts")
		case "bunjs":
			args := []string{"install", "--production", "--no-save", "--ignore-scripts"}
			if ctx.CI {
				args = append(args, "--verbose", "--no-cache")
			} else {
				args = append(args, "--no-progress", "--no-summary", "--silent")
			}
			install = exec.CommandContext(ctx.Context, "bun", args...)
		default:
			return fmt.Errorf("unsupported runtime: %s", theproject.Bundler.Runtime)
		}
		util.ProcessSetup(install)
		install.Dir = dir
		out, err := install.CombinedOutput()
		var ec int
		if install.ProcessState != nil {
			ec = install.ProcessState.ExitCode()
		}
		ctx.Logger.Trace("install command: %s returned: %s, err: %s, exit code: %d", strings.Join(install.Args, " "), strings.TrimSpace(string(out)), err, ec)
		if err != nil {
			if install.ProcessState == nil || install.ProcessState.ExitCode() != 0 {
				if install.ProcessState != nil {
					return fmt.Errorf("failed to install dependencies (exit code %d): %w. %s", install.ProcessState.ExitCode(), err, string(out))
				}
				return fmt.Errorf("failed to install dependencies: %w. %s", err, string(out))
			}
		}
		ctx.Logger.Debug("installed dependencies: %s", strings.TrimSpace(string(out)))
	}

	if err := checkForBreakingChanges(ctx, "javascript", theproject.Bundler.Runtime); err != nil {
		return err
	}

	var entryPoints []string
	entryPoints = append(entryPoints, filepath.Join(dir, "index.js"))
	files, err := util.ListDir(filepath.Join(dir, theproject.Bundler.AgentConfig.Dir))
	if err != nil {
		errsystem.New(errsystem.ErrListFilesAndDirectories, err).ShowErrorAndExit()
	}
	for _, file := range files {
		if filepath.Base(file) == "index.ts" {
			entryPoints = append(entryPoints, file)
		}
	}
	if len(entryPoints) == 0 {
		return fmt.Errorf("no index.ts files found in %s", theproject.Bundler.AgentConfig.Dir)
	}
	pkgjson := filepath.Join(dir, "package.json")
	pkg, err := util.NewOrderedMapFromFile(util.PackageJsonKeysOrder, pkgjson)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", pkgjson, err)
	}
	agentuitypkg, err := resolveAgentuity(dir)
	if err != nil {
		return err
	}
	pkg2, err := util.NewOrderedMapFromFile(util.PackageJsonKeysOrder, agentuitypkg)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", agentuitypkg, err)
	}
	agents := getAgents(theproject, "index.js")
	defines := map[string]string{
		"process.env.AGENTUITY_CLI_VERSION":     fmt.Sprintf("'%s'", Version),
		"process.env.AGENTUITY_SDK_APP_NAME":    fmt.Sprintf("'%s'", pkg.Data["name"]),
		"process.env.AGENTUITY_SDK_APP_VERSION": fmt.Sprintf("'%s'", pkg.Data["version"]),
		"process.env.AGENTUITY_SDK_VERSION":     fmt.Sprintf("'%s'", pkg2.Data["version"]),
	}
	defines["process.env.AGENTUITY_BUNDLER_RUNTIME"] = fmt.Sprintf("'%s'", theproject.Bundler.Runtime)
	if ctx.Production {
		defines["process.env.AGENTUITY_SDK_DEV_MODE"] = `"false"`
		defines["process.env.NODE_ENV"] = fmt.Sprintf("'%s'", "production")
	} else {
		if val, ok := os.LookupEnv("AGENTUITY_ENVIRONMENT"); ok {
			defines["process.env.AGENTUITY_ENVIRONMENT"] = fmt.Sprintf("'%s'", val)
		} else {
			defines["process.env.AGENTUITY_ENVIRONMENT"] = fmt.Sprintf("'%s'", "development")
		}
	}
	defines["process.env.AGENTUITY_CLOUD_AGENTS_JSON"] = cstr.JSONStringify(cstr.JSONStringify(agents))

	ctx.Logger.Debug("starting build")
	started := time.Now()
	result := api.Build(api.BuildOptions{
		EntryPoints:    entryPoints,
		Bundle:         true,
		Outdir:         outdir,
		Write:          true,
		Splitting:      false,
		Sourcemap:      api.SourceMapExternal,
		SourcesContent: api.SourcesContentInclude,
		Format:         api.FormatESModule,
		Platform:       api.PlatformNode,
		Engines: []api.Engine{
			{Name: api.EngineNode, Version: "22"},
		},
		External:      []string{"bun"},
		AbsWorkingDir: dir,
		TreeShaking:   api.TreeShakingTrue,
		Drop:          api.DropDebugger,
		Plugins:       []api.Plugin{createPlugin(ctx.Logger)},
		Define:        defines,
		LegalComments: api.LegalCommentsNone,
		Banner: map[string]string{
			"js": jsheader + jsshim,
		},
	})
	ctx.Logger.Debug("finished build in %v", time.Since(started))
	if len(result.Errors) > 0 {
		fmt.Fprintln(ctx.Writer, "\n"+tui.Warning("Build Failed")+"\n")

		for _, err := range result.Errors {
			formattedError := formatESBuildError(dir, err)
			fmt.Fprintln(ctx.Writer, formattedError)
		}

		if ctx.DevMode {
			return ErrBuildFailed
		}

		os.Exit(2)
		return nil // This line will never be reached due to os.Exit
	}
	return nil
}

var (
	pyProjectNameRegex    = regexp.MustCompile(`name\s+=\s+"(.*?)"`)
	pyProjectVersionRegex = regexp.MustCompile(`version\s+=\s+"(.*?)"`)
)

func bundlePython(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	if ctx.Install || !util.Exists(filepath.Join(dir, ".venv", "lib")) {
		var install *exec.Cmd
		switch theproject.Bundler.Runtime {
		case "uv":
			install = exec.CommandContext(ctx.Context, "uv", "sync", "--no-dev", "--frozen", "--quiet", "--no-progress")
		case "pip":
			install = exec.CommandContext(ctx.Context, "uv", "pip", "install", "--quiet", "--no-progress")
		case "poetry":
			return fmt.Errorf("poetry is not supported yet")
		default:
			return fmt.Errorf("unsupported runtime: %s", theproject.Bundler.Runtime)
		}
		util.ProcessSetup(install)
		install.Dir = dir
		out, err := install.CombinedOutput()
		if err != nil {
			if install.ProcessState == nil || install.ProcessState.ExitCode() != 0 {
				if install.ProcessState != nil {
					return fmt.Errorf("failed to install dependencies (exit code %d): %w. %s", install.ProcessState.ExitCode(), err, string(out))
				}
				return fmt.Errorf("failed to install dependencies: %w. %s", err, string(out))
			}
		}
		ctx.Logger.Debug("installed dependencies: %s", strings.TrimSpace(string(out)))
	}

	if err := checkForBreakingChanges(ctx, "python", theproject.Bundler.Runtime); err != nil {
		return err
	}

	config := map[string]any{
		"agents":      getAgents(theproject, "agent.py"),
		"cli_version": Version,
		"environment": "development",
	}
	if ctx.Production {
		config["environment"] = "production"
	}

	pyproject := filepath.Join(dir, "pyproject.toml")
	if sys.Exists(pyproject) {
		pyprojectData, err := os.ReadFile(pyproject)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", pyproject, err)
		}
		buf := string(pyprojectData)
		app := map[string]string{}
		if m := pyProjectNameRegex.FindStringSubmatch(buf); len(m) == 2 {
			app["name"] = m[1]
		}
		if m := pyProjectVersionRegex.FindStringSubmatch(buf); len(m) == 2 {
			app["version"] = m[1]
		}
		config["app"] = app
	}
	return os.WriteFile(filepath.Join(outdir, "config.json"), []byte(cstr.JSONStringify(config)), 0644)
}

func getAgents(theproject *project.Project, filename string) []AgentConfig {
	var agents []AgentConfig
	for _, agent := range theproject.Agents {
		var agentfilename string
		if theproject.Bundler.Language == "python" {
			agentfilename = util.SafePythonFilename(agent.Name)
		} else {
			agentfilename = util.SafeFilename(agent.Name)
		}
		agents = append(agents, AgentConfig{
			ID:       agent.ID,
			Name:     agent.Name,
			Filename: filepath.Join(theproject.Bundler.AgentConfig.Dir, agentfilename, filename),
		})
	}
	return agents
}

func Bundle(ctx BundleContext) error {
	theproject := project.NewProject()
	if err := theproject.Load(ctx.ProjectDir); err != nil {
		return fmt.Errorf("failed to load project from %s: %w", ctx.ProjectDir, err)
	}
	if theproject.ProjectId == "" {
		return fmt.Errorf("project in the directory %s is not a valid agentuity project", ctx.ProjectDir)
	}
	dir := ctx.ProjectDir
	outdir := filepath.Join(dir, ".agentuity")
	ctx.Logger.Debug("bundling project %s to %s", dir, outdir)
	if sys.Exists(outdir) {
		ctx.Logger.Debug("removing existing directory: %s", outdir)
		if err := os.RemoveAll(outdir); err != nil {
			return fmt.Errorf("failed to remove .agentuity directory: %w", err)
		}
	}
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create .agentuity directory: %w", err)
	}
	ctx.Logger.Debug("bundling project %s to %s", dir, outdir)
	switch theproject.Bundler.Language {
	case "javascript":
		return bundleJavascript(ctx, dir, outdir, theproject)
	case "python":
		return bundlePython(ctx, dir, outdir, theproject)
	}
	return fmt.Errorf("unsupported runtime: %s", theproject.Bundler.Runtime)
}

// resolveAgentuity walks up from startDir looking for node_modules/@agentuity/sdk/package.json
func resolveAgentuity(startDir string) (string, error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "package.json")
		if util.Exists(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find @agentuity/sdk/package.json in any parent directory of %s", startDir)
}
