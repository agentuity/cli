package util

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

// GetLatestRelease returns the latest release tag name from the GitHub API
func GetLatestRelease(ctx context.Context) (string, error) {
	if Version == "dev" {
		return Version, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://agentuity.sh/release/cli", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var release struct {
		TagName string `json:"tag_name"`
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to check for latest release: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

func CheckLatestRelease(ctx context.Context, logger logger.Logger, force bool) (bool, error) {
	if Version == "dev" {
		return false, nil
	}
	release, err := GetLatestRelease(ctx)
	if err != nil {
		return false, err
	}
	latestVersion := semver.MustParse(release)
	currentVersion := semver.MustParse(Version)
	if latestVersion.GreaterThan(currentVersion) {
		return showUpgradeNotice(ctx, logger, release, force), nil
	}
	return false, nil
}

func showUpgradeNotice(ctx context.Context, logger logger.Logger, version string, force bool) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// if we are running from homebrew, we need to upgrade via homebrew
	if tui.HasTTY {
		answer := tui.Ask(logger, tui.Bold(fmt.Sprintf("Agentuity version %s is available. Would you like to upgrade? [Y/n] ", version)), true)
		fmt.Println()
		if answer {
			var success bool
			action := func() {
				c := exec.CommandContext(ctx, exe, "update", "--force")
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				c.Stdin = os.Stdin
				if err := c.Run(); err != nil {
					return
				}
				success = true
			}
			action()
			if !success {
				tui.ShowWarning("Please see https://agentuity.dev/CLI/installation for instructions to upgrade manually.")
			}
			return success
		}
	}
	return false
}
