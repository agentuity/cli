package bundler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentuity/go-common/logger"
)

func TestGeneratePromptsInjection(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test prompts.yaml file
	promptsContent := `prompts:
  - slug: "code-review"
    name: "Code Review Assistant"
    description: "Helps review code for quality and best practices"
    system: "You are a senior software engineer."
    prompt: "Review this code: {{code}}"
  
  - slug: "test-generator"
    name: "Test Generator"
    system: "You are a QA engineer."
    prompt: "Generate tests for: {{functionality}}"
`

	promptsFile := filepath.Join(srcDir, "prompts.yaml")
	if err := os.WriteFile(promptsFile, []byte(promptsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a mock logger
	log := &mockLogger{}

	// Generate the injection code
	injection := generatePromptsInjection(log, tmpDir)

	// Verify the injection contains expected content
	if injection == "" {
		t.Fatal("Expected non-empty injection code")
	}

	// Check for expected elements
	expectedElements := []string{
		"const AGENTUITY_PROMPTS",
		"'code-review': {",
		"'test-generator': {",
		"declare module '@agentuity/sdk'",
		"interface PromptService",
		"codeReview(variables?: Record<string, unknown>): PromptDefinition;",
		"testGenerator(variables?: Record<string, unknown>): PromptDefinition;",
		"prompt.codeReview = function(variables = {})",
		"prompt.testGenerator = function(variables = {})",
		"function toCamelCase(slug)",
		"function fillTemplate(template, variables = {})",
		"fillTemplate(promptDef.system, variables)",
		"fillTemplate(promptDef.prompt, variables)",
	}

	for _, element := range expectedElements {
		if !strings.Contains(injection, element) {
			t.Errorf("Injection should contain '%s', but doesn't.\nGenerated:\n%s", element, injection)
		}
	}

	// Verify prompt data is properly escaped
	if !strings.Contains(injection, "slug: 'code-review'") {
		t.Error("Should contain properly quoted slug")
	}
	if !strings.Contains(injection, "name: 'Code Review Assistant'") {
		t.Error("Should contain properly quoted name")
	}
}

func TestUpdatePromptsPatches(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test prompts.yaml file
	promptsContent := `prompts:
  - slug: "test-prompt"
    name: "Test Prompt"
    system: "Test system"
    prompt: "Test prompt"
`

	promptsFile := filepath.Join(srcDir, "prompts.yaml")
	if err := os.WriteFile(promptsFile, []byte(promptsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a mock logger
	log := &mockLogger{}

	// Verify initial state
	if patch, exists := patches["@agentuity/sdk-prompts"]; exists {
		if patch.Body.After != "" {
			t.Error("Patch should start with empty After field")
		}
	}

	// Call updatePromptsPatches
	updatePromptsPatches(log, tmpDir)

	// Verify the patch was updated
	patch, exists := patches["@agentuity/sdk-prompts"]
	if !exists {
		t.Fatal("@agentuity/sdk-prompts patch should exist")
	}

	if patch.Body.After == "" {
		t.Error("Patch After field should be populated after update")
	}

	if !strings.Contains(patch.Body.After, "'test-prompt'") {
		t.Error("Patch should contain the test prompt")
	}
}

func TestEmptyPromptsDirectory(t *testing.T) {
	// Create a temporary directory with no prompts
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a mock logger
	log := &mockLogger{}

	// Generate the injection code
	injection := generatePromptsInjection(log, tmpDir)

	// Should return empty string when no prompts found
	if injection != "" {
		t.Error("Should return empty injection when no prompts found")
	}
}

type mockLogger struct{}

func (m *mockLogger) Debug(format string, args ...interface{}) {}
func (m *mockLogger) Info(format string, args ...interface{})  {}
func (m *mockLogger) Warn(format string, args ...interface{})  {}
func (m *mockLogger) Error(format string, args ...interface{}) {}
func (m *mockLogger) Fatal(format string, args ...interface{}) {}
func (m *mockLogger) Trace(format string, args ...interface{}) {}
func (m *mockLogger) SetLevel(level string)                    {}
func (m *mockLogger) GetLevel() string                         { return "info" }
func (m *mockLogger) WithField(key string, value interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithError(err error) logger.Logger {
	return m
}
func (m *mockLogger) Stack(logger logger.Logger) logger.Logger {
	return m
}
func (m *mockLogger) With(fields map[string]interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithContext(ctx context.Context) logger.Logger {
	return m
}
func (m *mockLogger) WithPrefix(prefix string) logger.Logger {
	return m
}
