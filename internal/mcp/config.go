package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

type MCPClientConfig struct {
	Name           string
	ConfigLocation string
	Command        string
	Transport      string
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

// Install installs the agentuity tool for the given command and args.
// It will install the tool for each MCP client config that is detected and not already installed.
func Install(ctx context.Context, logger logger.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	executable, err := exec.LookPath(agentuityToolCommand)
	if err != nil {
		return fmt.Errorf("failed to find %s: %w", agentuityToolCommand, err)
	}
	if executable == "" {
		return fmt.Errorf("failed to find %s", agentuityToolCommand)
	}
	for _, config := range mcpClientConfigs {
		_, err := exec.LookPath(config.Command)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				continue
			} else {
				return err
			}
		}
		config.ConfigLocation = strings.Replace(config.ConfigLocation, "$HOME", home, 1)
		var mcpconfig *MCPConfig
		if util.Exists(config.ConfigLocation) {
			logger.Debug("config already exists at %s, will load...", config.ConfigLocation)
			mcpconfig, err = loadConfig(config.ConfigLocation)
			if err != nil {
				return err
			}
		} else {
			logger.Debug("creating config for %s at %s", config.Name, config.ConfigLocation)
			mcpconfig = &MCPConfig{
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
			config.Transport = "cli"
		}
		if mcpconfig.AddIfNotExists(agentuityToolName, executable, append(agentuityToolArgs, "--"+config.Transport), agentuityToolEnv) {
			if err := mcpconfig.Save(); err != nil {
				return fmt.Errorf("failed to save config for %s: %w", config.Name, err)
			}
			logger.Debug("added %s config for %s at %s", agentuityToolName, config.Name, config.ConfigLocation)
			tui.ShowSuccess("Installed Agentuity MCP server for %s", config.Name)
		} else {
			logger.Debug("config for %s already exists at %s", agentuityToolName, config.ConfigLocation)
			tui.ShowSuccess("Agentuity MCP server already installed for %s", config.Name)
		}
	}
	return nil
}

// Uninstall uninstalls the agentuity tool for the given command and args.
// It will uninstall the tool for each MCP client config that is detected and not already uninstalled.
func Uninstall(ctx context.Context, logger logger.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var uninstalled bool
	for _, config := range mcpClientConfigs {
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
			uninstalled = true
		}
	}
	if !uninstalled {
		tui.ShowWarning("No Agentuity MCP servers found")
	}
	return nil
}

func init() {
	// Add MCP client configs for various tools we want to support automagically
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Cursor",
		ConfigLocation: "$HOME/.cursor/mcp.json",
		Command:        "cursor",
		Transport:      "cli",
	})
	mcpClientConfigs = append(mcpClientConfigs, MCPClientConfig{
		Name:           "Codeium",
		ConfigLocation: "$HOME/.codeium/windsurf/mcp_config.json",
		Command:        "codeium",
		Transport:      "cli",
	})
}
