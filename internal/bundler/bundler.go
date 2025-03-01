package bundler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/evanw/esbuild/pkg/api"
)

type AgentConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type BundleJSContext struct {
	Context    context.Context
	Logger     logger.Logger
	ProjectDir string
	Production bool
}

func BundleJS(ctx BundleJSContext) error {
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
	var entryPoints []string
	entryPoints = append(entryPoints, filepath.Join(dir, "index.js"))
	files, err := util.ListDir(theproject.Bundler.AgentConfig.Dir)
	if err != nil {
		return fmt.Errorf("failed to list src directory: %w", err)
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
	defines := map[string]string{
		"process.env.AGENTUITY_CLI_VERSION":     fmt.Sprintf("'%s'", theproject.Bundler.CLIVersion),
		"process.env.AGENTUITY_SDK_APP_NAME":    fmt.Sprintf("'%s'", pkg.Data["name"]),
		"process.env.AGENTUITY_SDK_APP_VERSION": fmt.Sprintf("'%s'", pkg.Data["version"]),
		"process.env.AGENTUITY_SDK_VERSION":     fmt.Sprintf("'%s'", pkg2.Data["version"]),
	}
	defines["process.env.AGENTUITY_BUNDLER_RUNTIME"] = fmt.Sprintf("'%s'", theproject.Bundler.Runtime)
	if ctx.Production {
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
	for _, agent := range theproject.Agents {
		agents = append(agents, AgentConfig{
			ID:       agent.ID,
			Name:     agent.Name,
			Filename: filepath.Join(theproject.Bundler.AgentConfig.Dir, util.SafeFilename(agent.Name), "index.js"),
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
		Plugins:       []api.Plugin{createPlugin(ctx.Logger)},
		Define:        defines,
		LegalComments: api.LegalCommentsNone,
		Banner: map[string]string{
			"js": jsheader + jsshim,
		},
	})
	if len(result.Errors) > 0 {
		var errs []error
		for _, err := range result.Errors {
			if err.Location != nil {
				errs = append(errs, fmt.Errorf("failed to bundle %s (line %d): %s", err.Location.File, err.Location.Line, err.Text))
			} else {
				errs = append(errs, fmt.Errorf("failed to bundle: %s", err.Text))
			}
		}
		return errors.Join(errs...)
	}
	return nil
}
