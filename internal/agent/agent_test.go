package agent

import (
	"testing"
)

func TestSourceResolver_Resolve(t *testing.T) {
	resolver := NewSourceResolver()

	tests := []struct {
		name     string
		input    string
		expected SourceType
		wantErr  bool
	}{
		{
			name:     "catalog reference",
			input:    "memory/vector-store",
			expected: SourceTypeCatalog,
			wantErr:  false,
		},
		{
			name:     "git repository",
			input:    "github.com/user/repo#main agent-name",
			expected: SourceTypeGit,
			wantErr:  false,
		},
		{
			name:     "https url",
			input:    "https://example.com/agent.zip",
			expected: SourceTypeURL,
			wantErr:  false,
		},
		{
			name:     "local path",
			input:    "./local/agent",
			expected: SourceTypeLocal,
			wantErr:  false,
		},
		{
			name:     "empty source",
			input:    "",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "invalid catalog reference",
			input:    "invalid-format",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.Type != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, result.Type)
			}
		})
	}
}

func TestSourceResolver_GetCacheKey(t *testing.T) {
	resolver := NewSourceResolver()

	tests := []struct {
		name     string
		source   *AgentSource
		expected string
	}{
		{
			name: "catalog source",
			source: &AgentSource{
				Type:     SourceTypeCatalog,
				Location: "https://github.com/agentuity/agents",
				Branch:   "main",
				Path:     "memory/vector-store",
			},
			expected: "catalog_https://github.com/agentuity/agents_main_memory/vector-store",
		},
		{
			name: "git source",
			source: &AgentSource{
				Type:     SourceTypeGit,
				Location: "https://github.com/user/repo",
				Branch:   "main",
				Path:     "agent-path",
			},
			expected: "git_https://github.com/user/repo_main_agent-path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.GetCacheKey(tt.source)
			if result != tt.expected {
				t.Errorf("Expected cache key %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAgentValidator_ValidateMetadata(t *testing.T) {
	validator := NewAgentValidator(false)

	tests := []struct {
		name     string
		metadata *AgentMetadata
		wantErr  bool
	}{
		{
			name: "valid metadata",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				Version:     "1.0.0",
				Description: "Test agent",
				Language:    "typescript",
				Files:       []string{"index.ts"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			metadata: &AgentMetadata{
				Version:     "1.0.0",
				Description: "Test agent",
				Language:    "typescript",
				Files:       []string{"index.ts"},
			},
			wantErr: true,
		},
		{
			name: "invalid language",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				Version:     "1.0.0",
				Description: "Test agent",
				Language:    "rust",
				Files:       []string{"index.ts"},
			},
			wantErr: true,
		},
		{
			name: "no files",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				Version:     "1.0.0",
				Description: "Test agent",
				Language:    "typescript",
				Files:       []string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true, Errors: []string{}}
			validator.validateMetadata(tt.metadata, result)

			hasErrors := len(result.Errors) > 0
			if hasErrors != tt.wantErr {
				t.Errorf("Expected error: %v, got errors: %v", tt.wantErr, hasErrors)
			}
		})
	}
}
