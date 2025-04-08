package bundler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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
}

func bundleJavascript(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	if ctx.Install || !util.Exists(filepath.Join(dir, "node_modules")) {
		var install *exec.Cmd
		switch theproject.Bundler.Runtime {
		case "nodejs":
			install = exec.CommandContext(ctx.Context, "npm", "install", "--no-save", "--no-audit", "--no-fund", "--include=prod")
		case "bunjs":
			install = exec.CommandContext(ctx.Context, "bun", "install", "--production", "--no-save", "--silent", "--no-progress", "--no-summary")
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
	agentuitypkg := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "package.json")
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
	defines["process.env.AGENTUITY_CLOUD_AGENTS_JSON"] = fmt.Sprintf("'%s'", cstr.JSONStringify(agents))

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
	if len(result.Errors) > 0 {
		fmt.Println("\n" + tui.Warning("Build Failed") + "\n")

		for _, err := range result.Errors {
			formattedError := formatESBuildError(dir, err)
			fmt.Println(formattedError)
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

	if ctx.Install {
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
		agents = append(agents, AgentConfig{
			ID:       agent.ID,
			Name:     agent.Name,
			Filename: filepath.Join(theproject.Bundler.AgentConfig.Dir, util.SafeFilename(agent.Name), filename),
		})
	}
	return agents
}

func Bundle(ctx BundleContext) error {
	theproject := project.NewProject()
	if err := theproject.Load(ctx.ProjectDir); err != nil {
		return err
	}
	dir := ctx.ProjectDir
	outdir := filepath.Join(dir, ".agentuity")
	if sys.Exists(outdir) {
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
