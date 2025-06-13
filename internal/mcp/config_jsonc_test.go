package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONCCommentSupport(t *testing.T) {
	tests := []struct {
		name        string
		jsonContent string
		expectError bool
	}{
		{
			name: "User's exact case: /* test */ at top",
			jsonContent: `/* test */
{
  "editor.fontSize": 14,
  "workbench.colorTheme": "Default Dark+",
  "mcpServers": {
    "agentuity": {
      "command": "npx",
      "args": ["-y", "@agentuity/mcp-server"],
      "env": {
        "AGENTUITY_API_KEY": "${AGENTUITY_API_KEY}"
      }
    }
  }
}`,
			expectError: false,
		},
		{
			name: "Single line comments",
			jsonContent: `{
  "editor.fontSize": 14,
  "mcpServers": {
    "agentuity": {
      "command": "npx", // inline comment
      "args": ["-y", "@agentuity/mcp-server"]
    }
  }
}`,
			expectError: false,
		},
		{
			name: "Multi-line comments",
			jsonContent: `{
  /* This is a 
     multi-line comment */
  "mcpServers": {
    "test": {
      "command": "test"
    }
  },
  /* Another comment */
  "ampMcpServers": {
    "amp": {
      "command": "amp"
    }
  }
}`,
			expectError: false,
		},
		{
			name: "Mixed comments comprehensive",
			jsonContent: `{
  /* test comment at start */
  "editor.fontSize": 14,
  "workbench.colorTheme": "Default Dark+",
  "mcpServers": {
    "agentuity": {
      "command": "npx", // inline comment
      "args": ["-y", "@agentuity/mcp-server"],
      "env": {
        "AGENTUITY_API_KEY": "${AGENTUITY_API_KEY}" /* env var comment */
      }
    }
  },
  /* Extension settings */
  "extensions.autoUpdate": false,
  "ampMcpServers": {
    "test": {
      "command": "test"
    }
  }
}`,
			expectError: false,
		},
		{
			name: "Empty JSON with comment",
			jsonContent: `/* test */
{
}`,
			expectError: false,
		},
		{
			name: "Comment at end",
			jsonContent: `{
  "mcpServers": {
    "test": {
      "command": "test"
    }
  }
}
/* end comment */`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test_config.json")

			err := os.WriteFile(configPath, []byte(tt.jsonContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			config, err := loadConfig(configPath)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if config == nil {
				t.Errorf("Config should not be nil")
				return
			}

			t.Logf("Successfully parsed config with %d mcpServers, %d ampMcpServers, %d extra fields",
				len(config.MCPServers), len(config.AMPMCPServers), len(config.Extra))
		})
	}
}

func TestCompleteLoadSaveCycleWithComments(t *testing.T) {
	jsonWithComments := `/* test comment */
{
  "editor.fontSize": 14,
  "workbench.colorTheme": "Default Dark+",
  "mcpServers": {
    "agentuity": {
      "command": "npx",
      "args": ["-y", "@agentuity/mcp-server"],
      "env": {
        "AGENTUITY_API_KEY": "${AGENTUITY_API_KEY}"
      }
    }
  },
  /* Extension settings */
  "extensions.autoUpdate": false
}`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")

	err := os.WriteFile(configPath, []byte(jsonWithComments), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(config.MCPServers) != 1 {
		t.Errorf("Expected 1 mcpServer, got %d", len(config.MCPServers))
	}

	if len(config.Extra) != 3 {
		t.Errorf("Expected 3 extra fields, got %d", len(config.Extra))
	}

	marshaledData, err := config.MarshalJSON()
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if len(marshaledData) == 0 {
		t.Errorf("Marshaled data should not be empty")
	}

	t.Logf("Successfully completed load-save cycle with comments")
}

func TestCursorSettingsWithComments(t *testing.T) {
	cursorSettings := `/* test */
{
  "editor.fontSize": 14,
  "workbench.colorTheme": "Default Dark+",
  "mcpServers": {
    "agentuity": {
      "command": "npx",
      "args": ["-y", "@agentuity/mcp-server"],
      "env": {
        "AGENTUITY_API_KEY": "${AGENTUITY_API_KEY}"
      }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/files"]
    }
  },
  "extensions.autoUpdate": false
}`

	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	err := os.WriteFile(settingsPath, []byte(cursorSettings), 0644)
	if err != nil {
		t.Fatalf("Failed to write Cursor settings file: %v", err)
	}

	config, err := loadConfig(settingsPath)
	if err != nil {
		t.Fatalf("Failed to load Cursor settings with comments: %v", err)
	}

	if len(config.MCPServers) != 2 {
		t.Errorf("Expected 2 mcpServers in Cursor settings, got %d", len(config.MCPServers))
	}

	if _, exists := config.MCPServers["agentuity"]; !exists {
		t.Errorf("Expected 'agentuity' mcpServer in Cursor settings")
	}

	if _, exists := config.MCPServers["filesystem"]; !exists {
		t.Errorf("Expected 'filesystem' mcpServer in Cursor settings")
	}

	marshaledData, err := config.MarshalJSON()
	if err != nil {
		t.Fatalf("Failed to marshal Cursor settings: %v", err)
	}

	if len(marshaledData) == 0 {
		t.Errorf("Marshaled Cursor settings should not be empty")
	}

	t.Logf("Successfully processed Cursor settings with /* test */ comment")
}
