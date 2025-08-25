package bundler

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"math"
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
	"k8s.io/apimachinery/pkg/api/resource"
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
	Project    *project.Project
	ProjectDir string
	Production bool
	Install    bool
	CI         bool
	DevMode    bool
	Writer     io.Writer
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func validateDiskRequest(ctx BundleContext, dir string) error {
	if !ctx.DevMode {
		if !util.Exists(dir) {
			return fmt.Errorf("%s not found", dir)
		}
		size, err := dirSize(dir)
		if err != nil {
			return fmt.Errorf("error calculating size of %s: %w", dir, err)
		}
		diskSize := resource.NewQuantity(size, resource.DecimalSI)
		val, ok := diskSize.AsInt64()
		if ok {
			millisValue := fmt.Sprintf("%.0fMi", math.Round(float64(val)/1000/1000))
			askSize, err := resource.ParseQuantity(ctx.Project.Deployment.Resources.Disk)
			if err != nil {
				return fmt.Errorf("error parsing disk requirement: %w", err)
			}
			askVal, ok := askSize.AsInt64()
			if ok {
				if askVal < val {
					if tui.HasTTY {
						fmt.Println(tui.Warning(fmt.Sprintf("Warning: The deployment is larger (%s) than the requested disk size for the deployment (%s).", millisValue, ctx.Project.Deployment.Resources.Disk)))
						if tui.AskForConfirm("Would you like to adjust the disk requirement?", 'y') != 'y' {
							fmt.Println()
							return fmt.Errorf("Disk request is too small. %s required but %s requested", millisValue, ctx.Project.Deployment.Resources.Disk)
						}
						fmt.Println()
						ctx.Project.Deployment.Resources.Disk = millisValue
						if err := ctx.Project.Save(ctx.ProjectDir); err != nil {
							return fmt.Errorf("error saving project: %w", err)
						}
						tui.ShowSuccess("Disk requirement adjusted to %s", millisValue)
					} else {
						return fmt.Errorf("The deployment is larger (%s) than the requested disk size for the deployment (%s)", millisValue, ctx.Project.Deployment.Resources.Disk)
					}
				}
			}
		}
	}
	return nil
}

func installSourceMapSupportIfNeeded(ctx BundleContext, dir string) error {
	// only bun needs to install this library to aide in parsing the source maps
	path := filepath.Join(dir, "node_modules", "source-map-js", "package.json")
	if !util.Exists(path) {
		cmd := exec.CommandContext(ctx.Context, "bun", "add", "source-map-js", "--no-save", "--silent", "--no-progress", "--no-summary", "--ignore-scripts")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install source-map-js: %w. %s", err, string(out))
		}
		return nil
	}
	return nil
}

func runTypecheck(ctx BundleContext, dir string) error {
	if ctx.Production {
		return nil
	}
	tsc := filepath.Join(dir, "node_modules", ".bin", "tsc")
	if !util.Exists(tsc) {
		ctx.Logger.Warn("no tsc found at %s, skipping typecheck", tsc)
		return nil
	}
	cmd := exec.CommandContext(ctx.Context, tsc, "--noEmit")
	cmd.Dir = dir
	cmd.Stdout = ctx.Writer
	cmd.Stderr = ctx.Writer
	if err := cmd.Run(); err != nil {
		if ctx.DevMode {
			ctx.Logger.Error("🚫 TypeScript check failed")
			return ErrBuildFailed // output goes to the console so we don't need to show it
		}
		os.Exit(2)
	}
	ctx.Logger.Debug("✅ TypeScript passed")
	return nil
}

// jsInstallCommandSpec returns the base command name and arguments for installing JavaScript dependencies
// This function returns the base command without CI-specific modifications
func jsInstallCommandSpec(ctx context.Context, projectDir, runtime string) (string, []string, error) {
	switch runtime {
	case "nodejs":
		if util.Exists(filepath.Join(projectDir, "yarn.lock")) {
			return "yarn", []string{"install", "--frozen-lockfile"}, nil
		} else {
			return "npm", []string{"install", "--no-audit", "--no-fund", "--include=prod", "--ignore-scripts"}, nil
		}
	case "bunjs":
		return "bun", []string{"install", "--production", "--no-save", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"}, nil
	case "pnpm":
		return "pnpm", []string{"install", "--prod", "--ignore-scripts", "--silent"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}
}

// getJSInstallCommand returns the complete install command with CI modifications applied
func getJSInstallCommand(ctx BundleContext, projectDir, runtime string) (string, []string, error) {
	cmd, args, err := jsInstallCommandSpec(ctx.Context, projectDir, runtime)
	if err != nil {
		return "", nil, err
	}
	
	// Apply CI-specific modifications
	if ctx.CI {
		if runtime == "bunjs" {
			// Replace silent flags with verbose for CI
			for i, arg := range args {
				if arg == "--no-progress" || arg == "--no-summary" || arg == "--silent" {
					args = append(args[:i], args[i+1:]...)
					i--
				}
			}
			args = append(args, "--verbose", "--no-cache")
		} else if runtime == "pnpm" {
			// Remove silent flag and add CI-specific flags
			for i, arg := range args {
				if arg == "--silent" {
					args = append(args[:i], args[i+1:]...)
					break
				}
			}
			args = append(args, "--reporter=append-only", "--frozen-lockfile")
		}
	}
	
	return cmd, args, nil
}

func bundleJavascript(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	if ctx.Install || !util.Exists(filepath.Join(dir, "node_modules")) {
	cmd, args, err := getJSInstallCommand(ctx, dir, theproject.Bundler.Runtime)
	if err != nil {
	return err
	}
	
	install := exec.CommandContext(ctx.Context, cmd, args...)
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

	var shimSourceMap bool

	if theproject.Bundler.Runtime == "bunjs" {
		if err := installSourceMapSupportIfNeeded(ctx, dir); err != nil {
			return fmt.Errorf("failed to install bun source-map-support: %w", err)
		}
		shimSourceMap = true
	}

	if err := checkForBreakingChanges(ctx, "javascript", theproject.Bundler.Runtime); err != nil {
		return err
	}

	if err := runTypecheck(ctx, dir); err != nil {
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
	ctx.Logger.Debug("resolving agentuity sdk")
	agentuitypkg, err := resolveAgentuity(ctx.Logger, dir)
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
		Sourcemap:      api.SourceMapLinked,
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
		Plugins:       []api.Plugin{createPlugin(ctx.Logger, dir, shimSourceMap)},
		Define:        defines,
		LegalComments: api.LegalCommentsNone,
		Banner: map[string]string{
			"js": strings.Join([]string{jsheader, jsshim}, "\n"),
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

	if err := validateDiskRequest(ctx, outdir); err != nil {
		return err
	}

	return nil
}

var (
	pyProjectNameRegex    = regexp.MustCompile(`name\s+=\s+"(.*?)"`)
	pyProjectVersionRegex = regexp.MustCompile(`version\s+=\s+"(.*?)"`)
)

/* NOTE: leaving this here for now but we don't need it for now but this will allow you to run uv python commands with virtual env
func runUVPython(ctx BundleContext, dir string, args ...string) ([]byte, error) {
	venvPath := ".venv"
	// python3 -m pip install --upgrade pip
	pythonPath := filepath.Join(dir, venvPath, "bin", "python3")
	fmt.Println(pythonPath, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx.Context, pythonPath, args...)
	env := os.Environ()
	env = append(env, "VIRTUAL_ENV="+venvPath)
	env = append(env, "PATH="+filepath.Join(venvPath, "bin")+":"+os.Getenv("PATH"))
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run uv: %w. %s", err, string(out))
	}
	return out, nil
}*/

func bundlePython(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	if ctx.Install || !util.Exists(filepath.Join(dir, ".venv", "lib")) {
		var install *exec.Cmd
		switch theproject.Bundler.Runtime {
		case "uv":
			install = exec.CommandContext(ctx.Context, "uv", "sync", "--no-dev", "--frozen", "--quiet", "--no-progress")
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

	if err := validateDiskRequest(ctx, filepath.Join(dir, ".venv")); err != nil {
		return err
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
		agentfilename = util.SafeProjectFilename(agent.Name, theproject.IsPython())
		agents = append(agents, AgentConfig{
			ID:       agent.ID,
			Name:     agent.Name,
			Filename: filepath.Join(theproject.Bundler.AgentConfig.Dir, agentfilename, filename),
		})
	}
	return agents
}

func CreateDeploymentMutator(ctx BundleContext) util.ZipDirCallbackMutator {
	return func(writer *zip.Writer) error {
		// NOTE: for now we don't need to do anything here
		// but this is a hook for future use where we can add files to the zip
		// before it is uploaded to the cloud
		return nil
	}
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
	switch theproject.Bundler.Language {
	case "javascript":
		return bundleJavascript(ctx, dir, outdir, theproject)
	case "python":
		return bundlePython(ctx, dir, outdir, theproject)
	}
	return fmt.Errorf("unsupported runtime: %s", theproject.Bundler.Runtime)
}

// resolveAgentuity walks up from startDir looking for node_modules/@agentuity/sdk/package.json
func resolveAgentuity(logger logger.Logger, startDir string) (string, error) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "package.json")
		if util.Exists(candidate) {
			logger.Debug("found @agentuity/sdk/package.json in %s", candidate)
			return candidate, nil
		}
		logger.Debug("did not find @agentuity/sdk/package.json in %s", dir)
		parent := filepath.Dir(dir)
		logger.Debug("checking parent directory: %s", parent)
		if parent == dir {
			logger.Debug("reached root directory, stopping search")
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find @agentuity/sdk/package.json in any parent directory of %s", startDir)
}
