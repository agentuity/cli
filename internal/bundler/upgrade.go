package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/tui"
)

var jsSDKVersion = ">=0.0.106"

type breakingChange struct {
	Title   string
	Message string
	Runtime string
	Version string
}

var jsBreakingChanges = []breakingChange{
	{
		Runtime: "bunjs",
		Version: "<0.0.106",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The JS SDK type signatures for AgentRequest have changed to be async functions. Please see the v0.0.106 Changelog for how to update your code.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00106") + "\n\nPlease bun upgrade @agentuity/sdk, fix your types and ensure your code passes type checking and then re-run this command again.",
	},
	{
		Runtime: "nodejs",
		Version: "<0.0.106",
		Title:   "🚫 JS SDK Breaking Change 🚫",
		Message: "The JS SDK type signatures for AgentRequest have changed to be async functions. Please see the v0.0.106 Changelog for how to update your code.\n\n" + tui.Link("https://agentuity.dev/Changelog/sdk-js#v00106") + "\n\nPlease npm upgrade @agentuity/sdk, fix your types and ensure your code passes type checking and then re-run this command again.",
	},
}

type packageJSON struct {
	Version string `json:"version"`
}

func checkForBreakingChanges(ctx BundleContext, language string, runtime string) error {
	switch language {
	case "javascript":
		pkgjson := filepath.Join(ctx.ProjectDir, "node_modules", "@agentuity", "sdk", "package.json")
		if util.Exists(pkgjson) {
			var pkg packageJSON
			content, err := os.ReadFile(pkgjson)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(content, &pkg); err != nil {
				return err
			}
			currentVersion := semver.MustParse(pkg.Version)
			for _, change := range jsBreakingChanges {
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
		}
	default:
		return nil
	}
	return nil
}
