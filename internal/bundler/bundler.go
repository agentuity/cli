package bundler

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/bundler/prompts"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/slice"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/evanw/esbuild/pkg/api"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

var Version = "dev"

var ErrBuildFailed = fmt.Errorf("build failed")

type AgentConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
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
		cmd := exec.CommandContext(ctx.Context, "bun", "add", "source-map-js", "--silent", "--no-progress", "--no-summary", "--ignore-scripts")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install source-map-js: %w. %s", err, string(out))
		}
		return nil
	}
	return nil
}

func runTypecheck(ctx BundleContext, dir string, installDir string) error {
	if ctx.Production {
		return nil
	}
	tsc := filepath.Join(installDir, "node_modules", ".bin", "tsc")
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

// WorkspaceConfig represents workspace configuration
type WorkspaceConfig struct {
	Root     string
	Type     string   // "npm", "yarn", or "pnpm"
	Patterns []string // workspace patterns
}

// detectWorkspaceRoot walks up from startDir looking for workspace configuration
func detectWorkspaceRoot(logger logger.Logger, startDir string) (*WorkspaceConfig, error) {
	dir := startDir
	for {
		// Check for npm/yarn workspaces in package.json
		packageJsonPath := filepath.Join(dir, "package.json")
		if util.Exists(packageJsonPath) {
			var pkg struct {
				Workspaces interface{} `json:"workspaces"`
			}
			data, err := os.ReadFile(packageJsonPath)
			if err == nil {
				if json.Unmarshal(data, &pkg) == nil && pkg.Workspaces != nil {
					patterns, err := parseNpmWorkspaces(pkg.Workspaces)
					if err == nil && len(patterns) > 0 {
						logger.Debug("found npm workspace config at %s", packageJsonPath)
						return &WorkspaceConfig{
							Root:     dir,
							Type:     "npm",
							Patterns: patterns,
						}, nil
					}
				}
			}
		}

		// Check for pnpm workspace
		pnpmWorkspacePath := filepath.Join(dir, "pnpm-workspace.yaml")
		if util.Exists(pnpmWorkspacePath) {
			var workspace struct {
				Packages []string `yaml:"packages"`
			}
			data, err := os.ReadFile(pnpmWorkspacePath)
			if err == nil {
				if yaml.Unmarshal(data, &workspace) == nil && len(workspace.Packages) > 0 {
					logger.Debug("found pnpm workspace config at %s", pnpmWorkspacePath)
					return &WorkspaceConfig{
						Root:     dir,
						Type:     "pnpm",
						Patterns: workspace.Packages,
					}, nil
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}
	return nil, nil
}

// parseNpmWorkspaces handles different npm workspaces formats
func parseNpmWorkspaces(workspaces interface{}) ([]string, error) {
	switch v := workspaces.(type) {
	case []interface{}:
		// Array format: ["packages/*", "apps/*"]
		patterns := make([]string, len(v))
		for i, pattern := range v {
			if str, ok := pattern.(string); ok {
				patterns[i] = str
			} else {
				return nil, fmt.Errorf("invalid workspace pattern type")
			}
		}
		return patterns, nil
	case map[string]interface{}:
		// Object format: {"packages": ["packages/*"]}
		if packages, ok := v["packages"].([]interface{}); ok {
			patterns := make([]string, len(packages))
			for i, pattern := range packages {
				if str, ok := pattern.(string); ok {
					patterns[i] = str
				} else {
					return nil, fmt.Errorf("invalid workspace pattern type")
				}
			}
			return patterns, nil
		}
	}
	return nil, fmt.Errorf("unsupported workspace format")
}

// isAgentInWorkspace checks if the agent directory matches any workspace patterns
func isAgentInWorkspace(logger logger.Logger, agentDir string, workspace *WorkspaceConfig) bool {
	// Get relative path from workspace root to agent directory
	relPath, err := filepath.Rel(workspace.Root, agentDir)
	if err != nil {
		logger.Debug("failed to get relative path: %v", err)
		return false
	}

	// Check if agent is outside workspace root
	if strings.HasPrefix(relPath, "..") {
		logger.Debug("agent directory is outside workspace root")
		return false
	}

	// Check each workspace pattern
	for _, pattern := range workspace.Patterns {
		if matchesWorkspacePattern(relPath, pattern) {
			logger.Debug("agent directory matches workspace pattern: %s", pattern)
			return true
		}
	}

	logger.Debug("agent directory doesn't match any workspace patterns")
	return false
}

// matchesWorkspacePattern checks if a path matches a workspace pattern using robust glob matching
// Supports npm-style patterns including "**" for recursive matching and proper cross-platform paths
func matchesWorkspacePattern(path, pattern string) bool {
	// Normalize paths to use forward slashes for cross-platform compatibility
	normalizedPath := filepath.ToSlash(path)
	normalizedPattern := filepath.ToSlash(pattern)

	// Handle negation patterns (e.g., "!excluded")
	if strings.HasPrefix(normalizedPattern, "!") {
		// This is a negation pattern - check if the path matches the pattern without "!"
		innerPattern := strings.TrimPrefix(normalizedPattern, "!")
		matched, err := doublestar.PathMatch(innerPattern, normalizedPath)
		// For negation patterns, we return the inverse of the match
		return err == nil && !matched
	}

	// Use doublestar for robust glob matching that supports "**" and proper npm-style patterns
	matched, err := doublestar.PathMatch(normalizedPattern, normalizedPath)
	return err == nil && matched
}

// findWorkspaceInstallDir determines where to install dependencies
func findWorkspaceInstallDir(logger logger.Logger, agentDir string) string {
	workspace, err := detectWorkspaceRoot(logger, agentDir)
	if err != nil {
		logger.Debug("error detecting workspace: %v", err)
		return agentDir
	}

	if workspace == nil {
		logger.Debug("no workspace detected, using agent directory")
		return agentDir
	}

	if isAgentInWorkspace(logger, agentDir, workspace) {
		logger.Debug("agent is part of %s workspace, using workspace root: %s", workspace.Type, workspace.Root)
		return workspace.Root
	}

	logger.Debug("agent is not part of workspace, using agent directory")
	return agentDir
}

// detectPackageManager detects which package manager to use based on lockfiles
func detectPackageManager(projectDir string) string {
	if util.Exists(filepath.Join(projectDir, "pnpm-lock.yaml")) {
		return "pnpm"
	} else if util.Exists(filepath.Join(projectDir, "bun.lockb")) || util.Exists(filepath.Join(projectDir, "bun.lock")) {
		return "bun"
	} else if util.Exists(filepath.Join(projectDir, "yarn.lock")) {
		return "yarn"
	} else {
		return "npm"
	}
}

// jsInstallCommandSpec returns the base command name and arguments for installing JavaScript dependencies
// This function returns the base command without CI-specific modifications
func jsInstallCommandSpec(projectDir string, isWorkspace bool, production bool) (string, []string, error) {
	packageManager := detectPackageManager(projectDir)

	switch packageManager {
	case "pnpm":
		if isWorkspace && !production {
			// In workspaces during development, install all dependencies including devDependencies
			// This ensures @types packages are available for TypeScript compilation
			return "pnpm", []string{"install", "--ignore-scripts", "--silent"}, nil
		}
		return "pnpm", []string{"install", "--prod", "--ignore-scripts", "--silent"}, nil
	case "bun":
		if isWorkspace && !production {
			return "bun", []string{"install", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"}, nil
		}
		return "bun", []string{"install", "--production", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"}, nil
	case "yarn":
		return "yarn", []string{"install", "--frozen-lockfile"}, nil
	case "npm":
		if isWorkspace && !production {
			return "npm", []string{"install", "--no-audit", "--no-fund", "--ignore-scripts"}, nil
		}
		return "npm", []string{"install", "--no-audit", "--no-fund", "--omit=dev", "--ignore-scripts"}, nil
	default:
		if isWorkspace && !production {
			return "npm", []string{"install", "--no-audit", "--no-fund", "--ignore-scripts"}, nil
		}
		return "npm", []string{"install", "--no-audit", "--no-fund", "--omit=dev", "--ignore-scripts"}, nil
	}
}

func generateBunLockfile(ctx BundleContext, logger logger.Logger, dir string) error {
	args := []string{"install", "--lockfile-only"}
	install := exec.CommandContext(ctx.Context, "bun", args...)
	install.Dir = dir
	out, err := install.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate bun lockfile: %w. %s", err, string(out))
	}
	logger.Debug("re-generated bun lockfile: %s", strings.TrimSpace(string(out)))
	return nil
}

// applyCIModifications applies CI-specific modifications to install command arguments
func applyCIModifications(ctx BundleContext, cmd, runtime string, args []string) []string {
	if !ctx.CI {
		return args
	}

	if cmd == "bun" {
		// Drop quiet flags for CI using a filtered copy
		filtered := make([]string, 0, len(args))
		for _, arg := range args {
			if arg == "--no-progress" || arg == "--no-summary" || arg == "--silent" {
				continue
			}
			filtered = append(filtered, arg)
		}
		return filtered
	} else if cmd == "pnpm" {
		// Remove silent flag and add CI-specific flags
		filtered := make([]string, 0, len(args)+2)
		for _, arg := range args {
			if arg == "--silent" {
				continue
			}
			filtered = append(filtered, arg)
		}
		filtered = append(filtered, "--reporter=append-only", "--frozen-lockfile")
		return filtered
	}

	return args
}

// getJSInstallCommand returns the complete install command with CI modifications applied
func getJSInstallCommand(ctx BundleContext, projectDir, runtime string, isWorkspace bool) (string, []string, error) {
	// For bun, we need to ensure the lockfile is up to date before we can run the install
	// otherwise we'll get an error about the lockfile being out of date
	// Only do this if we have a logger (i.e., not in tests)
	if runtime == "bunjs" && ctx.Logger != nil {
		if err := generateBunLockfile(ctx, ctx.Logger, projectDir); err != nil {
			return "", nil, err
		}
	}

	cmd, args, err := jsInstallCommandSpec(projectDir, isWorkspace, ctx.Production)
	if err != nil {
		return "", nil, err
	}

	// Apply CI-specific modifications
	args = applyCIModifications(ctx, cmd, runtime, args)

	return cmd, args, nil
}

// these are common externals we need to automatically exclude from bundling since they have native modules
var commonExternals = []string{"bun", "fsevents", "chromium-bidi", "sharp"}

// common packages that need automatic externals to be added when detected
// the key is the package in the package.json and then array is the dependencies that
// need to automatically be bundled when detected
var commonExternalsAutoInstalled = map[string][]string{
	"playwright-core": {"chromium-bidi"},
}

func bundleJavascript(ctx BundleContext, dir string, outdir string, theproject *project.Project) error {

	// Generate prompts if prompts.yaml exists (before dependency installation)

	if ctx.PromptsEvalsFF {
		if err := prompts.ProcessPrompts(ctx.Logger, dir); err != nil {
			return fmt.Errorf("failed to process prompts: %w", err)
		}
	}

	// Determine where to install dependencies (workspace root or agent directory)
	installDir := findWorkspaceInstallDir(ctx.Logger, dir)
	isWorkspace := installDir != dir // We're using workspace root if installDir differs from agent dir

	if ctx.Install || !util.Exists(filepath.Join(installDir, "node_modules")) {
		cmd, args, err := getJSInstallCommand(ctx, installDir, theproject.Bundler.Runtime, isWorkspace)
		if err != nil {
			return err
		}

		install := exec.CommandContext(ctx.Context, cmd, args...)
		util.ProcessSetup(install)
		install.Dir = installDir
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
		if err := installSourceMapSupportIfNeeded(ctx, installDir); err != nil {
			return fmt.Errorf("failed to install bun source-map-support: %w", err)
		}
		shimSourceMap = true
	}

	if err := checkForBreakingChanges(ctx, "javascript", theproject.Bundler.Runtime); err != nil {
		return err
	}

	if err := possiblyCreateDeclarationFile(ctx.Logger, dir); err != nil {
		return err
	}

	if err := runTypecheck(ctx, dir, installDir); err != nil {
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

	externals := make([]string, 0)
	copy(externals, commonExternals)
	// check to see if we have any externals explicitly set in package.json so that the
	// project can add additional externals automatically
	if e, ok := pkg.Data["externals"].([]interface{}); ok {
		for _, s := range e {
			if val, ok := s.(string); ok {
				if !slice.Contains(externals, val) {
					externals = append(externals, val)
				}
			}
		}
	}
	ctx.Logger.Debug("resolving agentuity sdk")
	agentuitypkg, err := resolveAgentuity(ctx.Logger, installDir)
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
		External:      externals,
		AbsWorkingDir: dir,
		TreeShaking:   api.TreeShakingTrue,
		Drop:          api.DropDebugger,
		Plugins: []api.Plugin{
			createPlugin(ctx.Logger, dir, shimSourceMap),
			createYAMLImporter(ctx.Logger),
			createJSONImporter(ctx.Logger),
			createTextImporter(ctx.Logger),
			createFileImporter(ctx.Logger),
		},
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
			ctx.Logger.Debug("build failed: %v", result.Errors)
			return ErrBuildFailed
		}

		os.Exit(2)
		return nil // This line will never be reached due to os.Exit
	}

	nodeModulesDir := filepath.Join(dir, "node_modules")

	var nativeInstalls []string

	for _, val := range externals {
		nm := filepath.Join(nodeModulesDir, val)
		if sys.Exists(nm) {
			nativeInstalls = append(nativeInstalls, val)
		}
	}

	for mod, deps := range commonExternalsAutoInstalled {
		nm := filepath.Join(nodeModulesDir, mod)
		if sys.Exists(nm) {
			for _, dep := range deps {
				if !slice.Contains(nativeInstalls, dep) {
					nativeInstalls = append(nativeInstalls, dep)
				}
			}
		}
	}

	// if we get here, we have detected native modules that cannot be bundled and that we need to install
	// natively into the bundle. we are going to move the package.json into the output folder and then automatically
	// install the bundle in the node_modules which can then be picked up at runtime since this folder will be
	// what is packaged and deployed
	if len(nativeInstalls) > 0 {
		// remove keys we just don't need
		for _, key := range []string{"dependencies", "devDependencies", "externals", "scripts", "keywords", "files"} {
			delete(pkg.Data, key)
		}
		buf, err := pkg.ToJSON()
		if err != nil {
			return fmt.Errorf("error serializing modified package.json: %w", err)
		}
		outfile := filepath.Join(outdir, "package.json")
		if err := os.WriteFile(outfile, buf, 0644); err != nil {
			return fmt.Errorf("error generating package.json: %w", err)
		}
		ctx.Logger.Trace("generated %s", outfile)
		npmargs := []string{"install", "--no-audit", "--no-fund", "--ignore-scripts", "--no-bin-links", "--no-package-lock"}
		if ctx.Production {
			// in production, we need to force the native modules to be compatible with our runtime environment
			npmargs = append(npmargs, "--platform=linux", "--arch=amd64", "--omit=dev")
		}
		npmargs = append(npmargs, nativeInstalls...)
		ctx.Logger.Trace("running native install: npm %s", strings.Join(npmargs, " "))
		// note: in this case, we're using npm which should be compatible regardless of main
		// package manager like bun or pnpm so we can get specific arg stability in the npm install
		c := exec.CommandContext(ctx.Context, "npm", npmargs...)
		c.Dir = outdir
		out, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install native dependencies: %w. %s", err, string(out))
		}
		ctx.Logger.Debug("npm installed native dependencies to %s: %s", outdir, string(out))
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
