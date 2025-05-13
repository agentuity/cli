package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/tui"
	"github.com/pelletier/go-toml/v2"
)

type breakingChange struct {
	Title   string
	Message string
	Runtime string
	Version string
}

var breakingChanges = []breakingChange{
	{
		Runtime: "bunjs",
		Version: "<0.0.106",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The JS SDK type signatures for AgentRequest have changed to be async functions. Please see the v0.0.106 Changelog for how to update your code.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00106") + "\n\nPlease bun update @agentuity/sdk --latest, fix your types and ensure your code passes type checking and then re-run this command again.",
	},
	{
		Runtime: "nodejs",
		Version: "<0.0.106",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The JS SDK type signatures for AgentRequest have changed to be async functions. Please see the v0.0.106 Changelog for how to update your code.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00106") + "\n\nPlease npm upgrade @agentuity/sdk, fix your types and ensure your code passes type checking and then re-run this command again.",
	},
	{
		Runtime: "uv",
		Version: "<0.0.82",
		Title:   "🚫 Python SDK Breaking Changes 🚫",
		Message: "The Python SDK type signatures for AgentRequest have changed to be async functions. Please see the v0.0.82 Changelog for how to update your code.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-py#v0082") + "\n\nPlease run `uv add agentuity -U` fix your types and ensure your code passes type checking and then re-run this command again.",
	},
}

type packageJSON struct {
	Version string `json:"version"`
}

type UVPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type UVLockfile struct {
	Packages []UVPackage `toml:"package"`
}

func getSDKVersionJavascript(ctx BundleContext) (*semver.Version, error) {
	var pkg packageJSON
	pkgjson := filepath.Join(ctx.ProjectDir, "node_modules", "@agentuity", "sdk", "package.json")
	if !util.Exists(pkgjson) {
		return nil, fmt.Errorf("package.json not found: %s", pkgjson)
	}
	content, err := os.ReadFile(pkgjson)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, err
	}
	currentVersion := semver.MustParse(pkg.Version)
	return currentVersion, nil
}

func getSDKVersionPython(ctx BundleContext) (*semver.Version, error) {
	uvlock := filepath.Join(ctx.ProjectDir, "uv.lock")
	if !util.Exists(uvlock) {
		return nil, fmt.Errorf("uv.lock not found: %s", uvlock)
	}
	file, err := os.Open(uvlock)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lockfile UVLockfile
	if err := toml.NewDecoder(file).Decode(&lockfile); err != nil {
		return nil, err
	}
	for _, pkg := range lockfile.Packages {
		if pkg.Name == "agentuity" {
			currentVersion := semver.MustParse(pkg.Version)
			return currentVersion, nil
		}
	}
	return nil, fmt.Errorf("agentuity package not found in uv.lock")
}

func GetSDKVersion(language string, ctx BundleContext) (*semver.Version, error) {
	switch language {
	case "python":
		return getSDKVersionPython(ctx)
	case "javascript":
		return getSDKVersionJavascript(ctx)
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
}

func checkForBreakingChanges(ctx BundleContext, language string, runtime string) error {
	ctx.Logger.Trace("Checking for breaking changes in %s, runtime: %s", language, runtime)
	switch language {
	case "python":
		currentVersion, err := getSDKVersionPython(ctx)
		if err != nil {
			return err
		}
		for _, change := range breakingChanges {
			if change.Runtime != runtime {
				continue
			}
			c, err := semver.NewConstraint(change.Version)
			if err != nil {
				return fmt.Errorf("error parsing semver constraint %s: %w", change.Version, err)
			}
			if c.Check(currentVersion) {
				if tui.HasTTY {
					tui.ShowBanner(change.Title, change.Message, true)
					os.Exit(1)
				} else {
					ctx.Logger.Fatal(change.Message)
				}
			}
		}
	case "javascript":
		currentVersion, err := getSDKVersionJavascript(ctx)
		if err != nil {
			return err
		}
		for _, change := range breakingChanges {
			if change.Runtime != runtime {
				continue
			}
			c, err := semver.NewConstraint(change.Version)
			if err != nil {
				return fmt.Errorf("error parsing semver constraint %s: %w", change.Version, err)
			}
			if c.Check(currentVersion) {
				if tui.HasTTY {
					tui.ShowBanner(change.Title, change.Message, true)
					os.Exit(1)
				} else {
					ctx.Logger.Fatal(change.Message)
				}
			}
		}
	default:
		return nil
	}
	return nil
}
