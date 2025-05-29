package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

const (
	agentuityToolName    = "agentuity"
	agentuityToolCommand = "agentuity"
)

var agentuityToolArgs = []any{"mcp", "run"}
var agentuityToolEnv = map[string]any{}

type MCPClientApplicationConfig struct {
	MacOS   []string
	Windows []string
	Linux   []string
}

type MCPClientConfig struct {
	Name           string
	ConfigLocation string
	Command        string
	Application    *MCPClientApplicationConfig
	Transport      string
	Config         *MCPConfig `json:"-"`
	Detected       bool       `json:"-"` // if the agentuity mcp server is detected in the config file
	Installed      bool       `json:"-"` // if this client is installed on this machine
}

type MCPServerConfig struct {
	Command string         `json:"command"`
	Args    []any          `json:"args,omitempty"`
	Env     map[string]any `json:"env,omitempty"`
}

type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	filename   string
}

func (c *MCPConfig) AddIfNotExists(name string, command string, args []any, env map[string]any) bool {
	if _, ok := c.MCPServers[name]; ok {
		return false
	}
	c.MCPServers[name] = MCPServerConfig{
		Command: command,
		Args:    args,
		Env:     env,
	}
	return true
}

func (c *MCPConfig) Save() error {
	if c.filename == "" {
		return errors.New("filename is not set")
	}
	if len(c.MCPServers) == 0 {
		os.Remove(c.filename) // if no more MCP servers, remove the config file
		return nil
	}
	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filename, content, 0644)
}

func loadConfig(path string) (*MCPConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config MCPConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}
	config.filename = path
	return &config, nil
}

var mcpClientConfigs []MCPClientConfig

// Detect detects the MCP clients that are installed and returns an array of MCP client names found.
func Detect(logger logger.Logger, all bool) ([]MCPClientConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var found []MCPClientConfig
	for _, config := range mcpClientConfigs {
		var exists bool
		if config.Command != "" {
			_, err := exec.LookPath(config.Command)
			if err != nil {
				if !errors.Is(err, exec.ErrNotFound) {
					return nil, err
				}
			} else {
				exists = true
			}
		}
		if !exists && config.Application != nil {
			var filepaths []string
			switch runtime.GOOS {
			case "darwin":
				filepaths = config.Application.MacOS
			case "windows":
				filepaths = config.Application.Windows
			case "linux":
				filepaths = config.Application.Linux
			}

			for _, filepath := range filepaths {
				if filepath == "$PATH" {
					if config.Command != "" {
						_, err := exec.LookPath(config.Command)
						if err == nil {
							exists = true
							break
						}
					}
				} else if util.Exists(filepath) {
					exists = true
					break
				}
			}
		}
		if !exists {
			if all {
				found = append(found, config)
			}
			continue
		}
		config.Installed = true
		config.ConfigLocation = strings.Replace(config.ConfigLocation, "$HOME", home, 1)
		var mcpconfig *MCPConfig
		if util.Exists(config.ConfigLocation) {
			mcpconfig, err = loadConfig(config.ConfigLocation)
			if err != nil {
				logger.Error("failed to load MCP config for %s: %s", config.Name, err)
				return nil, nil
			}
			if _, ok := mcpconfig.MCPServers[agentuityToolName]; ok {
				config.Detected = true
			}
			config.Config = mcpconfig
		}
		found = append(found, config)
	}
	return found, nil
}

// Install installs the agentuity tool for the given command and args.
// It will install the tool for each MCP client config that is detected and not already installed.
func Install(ctx context.Context, logger logger.Logger) error {
	detected, err := Detect(logger, false)
	if err != nil {
		return err
	}
	if len(detected) == 0 {
		return nil
	}
	var executable string
	if runtime.GOOS == "windows" {
		executable = "agentuity.exe" // already on the PATH in windows
	} else {
		executable, err = exec.LookPath(agentuityToolCommand)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				bin, err := os.Executable()
				if err != nil {
					return fmt.Errorf("failed to get executable path: %w", err)
				}
				executable = bin
			} else {
				return fmt.Errorf("failed to find %s: %w", agentuityToolCommand, err)
			}
		}
		if executable == "" {
			return fmt.Errorf("failed to find %s", agentuityToolCommand)
		}
	}
	var installed int
	for _, config := range detected {
		if config.Detected {
			continue
		}
		if config.Config == nil {
			config.Config = &MCPConfig{
				MCPServers: make(map[string]MCPServerConfig),
				filename:   config.ConfigLocation,
			}
			dir := filepath.Dir(config.ConfigLocation)
			if !util.Exists(dir) {
				logger.Debug("creating directory %s", dir)
				if err := os.MkdirAll(dir, 0700); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}
		}
		if config.Transport == "" {
			config.Transport = "stdio"
		}
		if config.Config.AddIfNotExists(agentuityToolName, executable, append(agentuityToolArgs, "--"+config.Transport), agentuityToolEnv) {
			if err := config.Config.Save(); err != nil {
				return fmt.Errorf("failed to save config for %s: %w", config.Name, err)
			}
			logger.Debug("added %s config for %s at %s", agentuityToolName, config.Name, config.ConfigLocation)
			tui.ShowSuccess("Installed Agentuity MCP server for %s", config.Name)
		} else {
			logger.Debug("config for %s already exists at %s", agentuityToolName, config.ConfigLocation)
			tui.ShowSuccess("Agentuity MCP server already installed for %s", config.Name)
		}
		installed++
	}
	if installed == 0 {
		tui.ShowSuccess("All MCP clients are up-to-date")
	}

	return nil
}

// Uninstall uninstalls the agentuity tool for the given command and args.
// It will uninstall the tool for each MCP client config that is detected and not already uninstalled.
func Uninstall(ctx context.Context, logger logger.Logger) error {
	detected, err := Detect(logger, false)
	if err != nil {
		return err
	}
	if len(detected) == 0 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var uninstalled int
	for _, config := range detected {
		config.ConfigLocation = strings.Replace(config.ConfigLocation, "$HOME", home, 1)
		if util.Exists(config.ConfigLocation) {
			mcpconfig, err := loadConfig(config.ConfigLocation)
			if err != nil {
				return err
			}
			if _, ok := mcpconfig.MCPServers[agentuityToolName]; !ok {
				logger.Debug("config for %s not found in %s, skipping", config.Name, config.ConfigLocation)
				continue
			}
			delete(mcpconfig.MCPServers, agentuityToolName)
			if err := mcpconfig.Save(); err != nil {
				return fmt.Errorf("failed to save config for %s: %w", config.Name, err)
			}
			logger.Debug("removed %s config for %s at %s", agentuityToolName, config.Name, config.ConfigLocation)
			tui.ShowSuccess("Uninstalled Agentuity MCP server for %s", config.Name)
			uninstalled++
		}
	}
	if uninstalled == 0 {
		tui.ShowWarning("Agentuity MCP server not installed for any clients")
	}
	return nil
}

func init() {
	// Add MCP client configs for various tools we want to support automagically
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Cursor",
		ConfigLocation: "$HOME/.cursor/mcp.json",
		Command:        "cursor",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Cursor.app/Contents/MacOS/Cursor", "/usr/local/bin/cursor", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "cursor")), "Cursor.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/cursor", "/usr/local/bin/cursor", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Windsurf",
		ConfigLocation: "$HOME/.codeium/windsurf/mcp_config.json",
		Command:        "windsurf",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Windsurf.app/Contents/MacOS/Electron", "/usr/local/bin/windsurf", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "Windsurf")), "Windsurf.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/windsurf", "/usr/local/bin/windsurf", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Claude Desktop",
		ConfigLocation: filepath.Join(util.GetAppSupportDir("Claude"), "claude_desktop_config.json"),
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Claude.app/Contents/MacOS/Claude", "/usr/local/bin/claude-desktop", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir("Claude Desktop"), "Claude Desktop.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/claude-desktop", "/usr/local/bin/claude-desktop", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Claude Code",
		ConfigLocation: filepath.Join(util.GetAppSupportDir("Claude Code"), "mcp_config.json"),
		Command:        "claude",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Claude Code.app/Contents/MacOS/Claude Code", "/opt/homebrew/bin/claude", "/usr/local/bin/claude", "~/.npm-global/bin/claude", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir("Claude Code"), "Claude Code.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/claude", "/usr/local/bin/claude", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Cline",
		ConfigLocation: "$HOME/.config/cline/mcp.json",
		Command:        "cline",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Cline.app/Contents/MacOS/Cline", "/usr/local/bin/cline", "/opt/homebrew/bin/cline", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "Cline")), "Cline.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/cline", "/usr/local/bin/cline", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Augment Code",
		ConfigLocation: "$HOME/.config/augment/mcp.json",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Augment Code.app/Contents/MacOS/Augment Code", "/usr/local/bin/augment", "/opt/homebrew/bin/augment", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir("Augment Code"), "Augment Code.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/augment", "/usr/local/bin/augment", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "VSCode",
		ConfigLocation: filepath.Join(util.GetAppSupportDir("Code"), "User", "mcp_config.json"),
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Visual Studio Code.app/Contents/MacOS/Electron", "/usr/local/bin/code", "/opt/homebrew/bin/code", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "Microsoft VS Code")), "Code.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/code", "/usr/local/bin/code", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Zed",
		ConfigLocation: "$HOME/.config/zed/mcp.json",
		Command:        "zed",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/Applications/Zed.app/Contents/MacOS/Zed", "/usr/local/bin/zed", "/opt/homebrew/bin/zed", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "Zed")), "Zed.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/zed", "/usr/local/bin/zed", "$PATH"},
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Anthropic Terminal",
		ConfigLocation: "$HOME/.config/anthropic/terminal/mcp.json",
		Command:        "anthropic",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS:   []string{"/usr/local/bin/anthropic", "/opt/homebrew/bin/anthropic", "$PATH"},
			Windows: []string{filepath.Join(util.GetAppSupportDir(filepath.Join("Programs", "Anthropic")), "anthropic.exe"), "$PATH"},
			Linux:   []string{"/usr/bin/anthropic", "/usr/local/bin/anthropic", "$PATH"},
		},
	})
}
