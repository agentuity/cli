package bundler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

func TestFormatBuildError(t *testing.T) {
	tempDir := t.TempDir()

	jsFilePath := filepath.Join(tempDir, "test.js")
	jsContent := `function test() {
  const x = {
    name: "test",
    value: 123
  }
  return x.missing.property;
}`
	if err := os.WriteFile(jsFilePath, []byte(jsContent), 0644); err != nil {
		t.Fatalf("Failed to create test JS file: %v", err)
	}

	tsFilePath := filepath.Join(tempDir, "test.ts")
	tsContent := `function test(): string {
  const x: any = {
    name: "test",
    value: 123
  }
  return x.missing.property;
}`
	if err := os.WriteFile(tsFilePath, []byte(tsContent), 0644); err != nil {
		t.Fatalf("Failed to create test TS file: %v", err)
	}

	tests := []struct {
		name           string
		message        api.Message
		wantContain    []string
		wantNotContain []string
	}{
		{
			name: "JavaScript error with line and column",
			message: api.Message{
				Text: "Cannot access property 'property' of undefined",
				Location: &api.Location{
					File:     jsFilePath,
					Line:     6,
					Column:   17,
					LineText: "  return x.missing.property;",
				},
			},
			wantContain: []string{
				"Cannot access property 'property' of undefined",
				"test.js:6:17",
				"6 │",
				"╵",
				"note: JavaScript build failed",
			},
		},
		{
			name: "TypeScript error with line and column",
			message: api.Message{
				Text: "Cannot access property 'property' of undefined",
				Location: &api.Location{
					File:     tsFilePath,
					Line:     6,
					Column:   17,
					LineText: "  return x.missing.property;",
				},
			},
			wantContain: []string{
				"Cannot access property 'property' of undefined",
				"test.ts:6:17",
				"6 │",
				"╵",
				"note: JavaScript build failed",
			},
		},
		{
			name: "Error without location",
			message: api.Message{
				Text: "Bundle failed",
			},
			wantContain: []string{
				"Bundle failed",
				"note: JavaScript build failed",
			},
		},
		{
			name: "Error with line but no column",
			message: api.Message{
				Text: "Syntax error",
				Location: &api.Location{
					File:     jsFilePath,
					Line:     3,
					LineText: "    name: \"test\",",
				},
			},
			wantContain: []string{
				"Syntax error",
				"test.js:3",
				"name: \"test\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBuildError(tempDir, tt.message)

			for _, want := range tt.wantContain {
				if !strings.Contains(result, want) {
					t.Errorf("FormatBuildError() = %v, should contain %v", result, want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(result, notWant) {
					t.Errorf("FormatBuildError() = %v, should not contain %v", result, notWant)
				}
			}
		})
	}
}
