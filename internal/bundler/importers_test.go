package bundler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentuity/go-common/logger"
)

// Mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Trace(format string, args ...interface{}) {}
func (m *mockLogger) Debug(format string, args ...interface{}) {}
func (m *mockLogger) Info(format string, args ...interface{})  {}
func (m *mockLogger) Warn(format string, args ...interface{})  {}
func (m *mockLogger) Error(format string, args ...interface{}) {}
func (m *mockLogger) Fatal(format string, args ...interface{}) {}
func (m *mockLogger) IsTraceEnabled() bool                     { return false }
func (m *mockLogger) IsDebugEnabled() bool                     { return false }
func (m *mockLogger) IsInfoEnabled() bool                      { return false }
func (m *mockLogger) IsWarnEnabled() bool                      { return false }
func (m *mockLogger) IsErrorEnabled() bool                     { return false }
func (m *mockLogger) IsFatalEnabled() bool                     { return false }
func (m *mockLogger) WithField(key string, value interface{}) logger.Logger { return m }
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger { return m }
func (m *mockLogger) WithError(err error) logger.Logger                      { return m }
func (m *mockLogger) Stack(logger logger.Logger) logger.Logger               { return m }
func (m *mockLogger) With(fields map[string]interface{}) logger.Logger       { return m }
func (m *mockLogger) WithContext(ctx context.Context) logger.Logger          { return m }
func (m *mockLogger) WithPrefix(prefix string) logger.Logger                 { return m }

func TestImportInsertion(t *testing.T) {
	tests := []struct {
		name           string
		inputContent   string
		expectedOutput string
		shouldInsert   bool
	}{
		{
			name: "insert after export with relative import",
			inputContent: `export { Tool } from './tool';
export { Agent } from './agent';
declare module '@agentuity/sdk' {
  export interface Config {}
}`,
			expectedOutput: `export { Tool } from './tool';
import './file_types';
export { Agent } from './agent';
declare module '@agentuity/sdk' {
  export interface Config {}
}`,
			shouldInsert: true,
		},
		{
			name: "insert after last export when no relative imports",
			inputContent: `export interface Tool {}
export class Agent {}
declare module '@agentuity/sdk' {
  export interface Config {}
}`,
			expectedOutput: `export interface Tool {}
export class Agent {}
import './file_types';
declare module '@agentuity/sdk' {
  export interface Config {}
}`,
			shouldInsert: true,
		},
		{
			name: "don't insert when import already exists with single quotes",
			inputContent: `export { Tool } from './tool';
import './file_types';
export { Agent } from './agent';`,
			expectedOutput: `export { Tool } from './tool';
import './file_types';
export { Agent } from './agent';`,
			shouldInsert: false,
		},
		{
			name: "don't insert when import already exists with double quotes",
			inputContent: `export { Tool } from "./tool";
import "./file_types";
export { Agent } from "./agent";`,
			expectedOutput: `export { Tool } from "./tool";
import "./file_types";
export { Agent } from "./agent";`,
			shouldInsert: false,
		},
		{
			name: "append at end when only exports exist",
			inputContent: `export interface Tool {}
export class Agent {}`,
			expectedOutput: `export interface Tool {}
export class Agent {}
import './file_types';`,
			shouldInsert: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory and files
			tmpDir, err := os.MkdirTemp("", "bundler_test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create SDK directory structure
			sdkDir := filepath.Join(tmpDir, "node_modules", "@agentuity", "sdk", "dist")
			err = os.MkdirAll(sdkDir, 0755)
			if err != nil {
				t.Fatalf("failed to create SDK dir: %v", err)
			}

			// Write test content to index.d.ts  
			indexPath := filepath.Join(sdkDir, "index.d.ts")
			err = os.WriteFile(indexPath, []byte(tt.inputContent), 0644)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}
			

			
			// Also need to create the file_types.d.ts file that the patching logic expects to exist
			// This triggers the SDK patching logic
			fileTypesPath := filepath.Join(sdkDir, "file_types.d.ts") 
			err = os.WriteFile(fileTypesPath, []byte("// placeholder"), 0644)
			if err != nil {
				t.Fatalf("failed to write file_types.d.ts: %v", err)
			}

			// Create a mock logger
			mockLog := &mockLogger{}

			// Call the function under test
			err = possiblyCreateDeclarationFile(mockLog, tmpDir)
			if err != nil {
				t.Fatalf("possiblyCreateDeclarationFile failed: %v", err)
			}

			// Read the result
			result, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatalf("failed to read result file: %v", err)
			}
			


			resultStr := string(result)
			if resultStr != tt.expectedOutput {
				t.Errorf("unexpected output\nexpected:\n%s\n\nactual:\n%s", tt.expectedOutput, resultStr)
			}

			// Verify import detection logic
			hasImport := strings.Contains(resultStr, "import './file_types'") || strings.Contains(resultStr, "import \"./file_types\"")
			if tt.shouldInsert && !hasImport {
				t.Errorf("expected import to be inserted but it wasn't found")
			}
			if !tt.shouldInsert && hasImport && !strings.Contains(tt.inputContent, "file_types") {
				t.Errorf("expected import not to be inserted but it was added")
			}
		})
	}
}

func TestNeedsDeclarationUpdate(t *testing.T) {
	tests := []struct {
		name           string
		fileContent    string
		expectedHash   string
		shouldUpdate   bool
	}{
		{
			name:           "file doesn't exist",
			fileContent:    "",
			expectedHash:   "abc123",
			shouldUpdate:   true,
		},
		{
			name:           "file has matching hash",
			fileContent:    "// agentuity-types-hash:abc123\ndeclare module '*.yml' {}",
			expectedHash:   "abc123",
			shouldUpdate:   false,
		},
		{
			name:           "file has different hash",
			fileContent:    "// agentuity-types-hash:def456\ndeclare module '*.yml' {}",
			expectedHash:   "abc123",
			shouldUpdate:   true,
		},
		{
			name:           "file has no hash",
			fileContent:    "declare module '*.yml' {}",
			expectedHash:   "abc123",
			shouldUpdate:   true,
		},
		{
			name:           "empty file",
			fileContent:    "",
			expectedHash:   "abc123",
			shouldUpdate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "file doesn't exist" {
				// Test non-existent file
				result := needsDeclarationUpdate("/nonexistent/path", tt.expectedHash)
				if result != tt.shouldUpdate {
					t.Errorf("expected %v, got %v", tt.shouldUpdate, result)
				}
				return
			}

			// Create temporary file
			tmpFile, err := os.CreateTemp("", "test_declaration")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write test content
			_, err = tmpFile.WriteString(tt.fileContent)
			if err != nil {
				t.Fatalf("failed to write test content: %v", err)
			}
			tmpFile.Close()

			// Test the function
			result := needsDeclarationUpdate(tmpFile.Name(), tt.expectedHash)
			if result != tt.shouldUpdate {
				t.Errorf("expected %v, got %v for content: %s", tt.shouldUpdate, result, tt.fileContent)
			}
		})
	}
}
