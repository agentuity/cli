package util

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Masterminds/semver"
	"github.com/agentuity/go-common/tui"
	"golang.org/x/term"
)

// GetLatestRelease returns the latest release tag name from the GitHub API
func GetLatestRelease(ctx context.Context) (string, error) {
	if Version == "dev" {
		return Version, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/agentuity/cli/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
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
	return release.TagName, nil
}

func CheckLatestRelease(ctx context.Context) error {
	if Version == "dev" {
		return nil
	}

	release, err := GetLatestRelease(ctx)
	if err != nil {
		return err
	}
	latestVersion := semver.MustParse(release)
	currentVersion := semver.MustParse(Version)
	if latestVersion.GreaterThan(currentVersion) {
		showUpgradeNotice(release)
	}
	return nil
}

func showUpgradeNotice(version string) {
	if runtime.GOOS == "darwin" {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		// if we are running from homebrew, we need to upgrade via homebrew
		if tui.HasTTY && strings.Contains(exe, "/homebrew/Cellar/agentuity/") {
			answer := ask(tui.Bold(fmt.Sprintf("Agentuity version %s is available. Would you like to upgrade? [Y/n] ", version)), "Y")
			fmt.Println()
			if answer == "Y" || answer == "y" {
				action := func() {
					exec.Command("brew", "update").Run()
					exec.Command("brew", "upgrade", "agentuity").Run()
					v, _ := exec.Command(exe, "version").Output()
					if strings.TrimSpace(string(v)) == version {
						tui.ShowSuccess("Upgraded to %s", version)
						return
					}
					tui.ShowWarning("Failed to upgrade. Please run `brew update && brew upgrade agentuity` manually.")
				}
				tui.ShowSpinner("Upgrading...", action)
			}
			return
		}
	}
	tui.ShowBanner("Agentuity Upgrade", fmt.Sprintf("Agentuity version %s is available. See %s for more information.", version, tui.Link("https://agentuity.dev/CLI/installation")), false)
}

func ask(message string, defaultValue string) string {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return ""
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ch := make(chan string, 1)
	go func() {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		ch <- string(buf)
	}()
	fmt.Print(message)
	select {
	case <-ctx.Done():
		return ""
	case answer := <-ch:
		if answer == "\n" || answer == "\r" {
			return defaultValue
		}
		return answer
	}
}
