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

var agentuityToolArgs = []string{"mcp", "run"}
var agentuityToolEnv = map[string]string{}

type MCPClientApplicationConfig struct {
	MacOS   string
	Windows string
	Linux   string
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
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	filename   string
}

func (c *MCPConfig) AddIfNotExists(name string, command string, args []string, env map[string]string) bool {
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
func Detect(all bool) ([]MCPClientConfig, error) {
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
			var filepath string
			switch runtime.GOOS {
			case "darwin":
				if config.Application.MacOS != "" {
					filepath = config.Application.MacOS
				}
			case "windows":
				if config.Application.Windows != "" {
					filepath = config.Application.Windows
				}
			case "linux":
				if config.Application.Linux != "" {
					filepath = config.Application.Linux
				}
			}
			if util.Exists(filepath) {
				exists = true
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
				return nil, err
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
	detected, err := Detect(false)
	if err != nil {
		return err
	}
	if len(detected) == 0 {
		return nil
	}
	executable, err := exec.LookPath(agentuityToolCommand)
	if err != nil {
		return fmt.Errorf("failed to find %s: %w", agentuityToolCommand, err)
	}
	if executable == "" {
		return fmt.Errorf("failed to find %s", agentuityToolCommand)
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
	detected, err := Detect(false)
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
			MacOS: "/Applications/Cursor.app/Contents/MacOS/Cursor",
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Windsurf",
		ConfigLocation: "$HOME/.codeium/windsurf/mcp_config.json",
		Command:        "windsurf",
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS: "/Applications/Windsurf.app/Contents/MacOS/Electron",
		},
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Claude Desktop",
		ConfigLocation: filepath.Join(util.GetAppSupportDir("Claude"), "claude_desktop_config.json"),
		Transport:      "stdio",
		Application: &MCPClientApplicationConfig{
			MacOS: "/Applications/Claude.app/Contents/MacOS/Claude",
		},
	})
}
