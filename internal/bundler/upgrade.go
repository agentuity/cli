package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/tui"
	"github.com/pelletier/go-toml/v2"
)

type breakingChange struct {
	Title    string
	Message  string
	Runtime  string
	Version  string
	Callback func(ctx BundleContext) error
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
	{
		Runtime: "bunjs",
		Version: "<0.0.115",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The environment variable and code reference for your Agentuity API key has changed from AGENTUITY_API_KEY to AGENTUITY_SDK_KEY. Update all occurrences in your .env files and codebase. See the v0.0.115 Changelog for details.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00115") + "\n\nAfter migrated, please run bun update @agentuity/sdk --latest and then re-run this command again.",
		Callback: func(ctx BundleContext) error {
			files := []string{
				filepath.Join(ctx.ProjectDir, "index.ts"),
				filepath.Join(ctx.ProjectDir, ".env"),
			}
			for _, file := range files {
				if !util.Exists(file) {
					continue
				}
				content, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				updated := strings.ReplaceAll(string(content), `AGENTUITY_API_KEY`, `AGENTUITY_SDK_KEY`)
				if updated != string(content) {
					err = os.WriteFile(file, []byte(updated), 0644)
					if err != nil {
						return err
					}
				}
			}

			return nil
		},
	},
	{
		Runtime: "nodejs",
		Version: "<0.0.115 ",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The environment variable and code reference for your Agentuity API key has changed from AGENTUITY_API_KEY to AGENTUITY_SDK_KEY. Update all occurrences in your .env files and codebase. See the v0.0.115 Changelog for details.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00115") + "\n\nAfter migrated, please run npm upgrade @agentuity/sdk aand then re-run this command again.",
		Callback: func(ctx BundleContext) error {
			files := []string{
				filepath.Join(ctx.ProjectDir, "index.ts"),
				filepath.Join(ctx.ProjectDir, ".env"),
			}
			for _, file := range files {
				if !util.Exists(file) {
					continue
				}
				content, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				updated := strings.ReplaceAll(string(content), `AGENTUITY_API_KEY`, `AGENTUITY_SDK_KEY`)
				if updated != string(content) {
					err = os.WriteFile(file, []byte(updated), 0644)
					if err != nil {
						return err
					}
				}
			}

			return nil
		},
	},
	{
		Runtime: "uv",
		Version: "<0.0.84",
		Title:   "🚫 Python SDK Breaking Changes 🚫",
		Message: "The environment variable and code reference for your Agentuity API key has changed from AGENTUITY_API_KEY to AGENTUITY_SDK_KEY. Update all occurrences in your .env files and codebase. See the v0.0.84 Changelog for details.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-py#v0084") + "\n\nAfter migrated, please run `uv add agentuity -U` --latest and then re-run this command again.",
		Callback: func(ctx BundleContext) error {
			files := []string{
				filepath.Join(ctx.ProjectDir, "server.py"),
				filepath.Join(ctx.ProjectDir, ".env"),
			}
			for _, file := range files {
				if !util.Exists(file) {
					continue
				}
				content, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				updated := strings.ReplaceAll(string(content), `AGENTUITY_API_KEY`, `AGENTUITY_SDK_KEY`)
				if updated != string(content) {
					err = os.WriteFile(file, []byte(updated), 0644)
					if err != nil {
						return err
					}
				}
			}

			return nil
		},
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
		if pkg.Name == "agentuity" && !strings.Contains(pkg.Version, "+") {
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
	currentVersion, err := GetSDKVersion(language, ctx)
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
		if strings.Contains(currentVersion.String(), "-pre") {
			return nil
		}
		if c.Check(currentVersion) {
			if change.Callback != nil {
				var proceed bool
				if tui.HasTTY && !ctx.DevMode {
					tui.ShowBanner(change.Title, change.Message, true)
				} else {
					return fmt.Errorf("migration required: %s. %s", change.Title, change.Message)
				}
				proceed = tui.AskForConfirm("Would you like to migrate your project now?", 'y') == 'y'
				if proceed {
					if err := change.Callback(ctx); err != nil {
						return err
					}
					os.Exit(1)
				} else {
					return fmt.Errorf("migration required")
				}
			} else {
				if tui.HasTTY && !ctx.DevMode {
					tui.ShowBanner(change.Title, change.Message, true)
					os.Exit(1)
				} else {
					ctx.Logger.Fatal(change.Message)
				}
			}
		}
	}

	return nil
}
