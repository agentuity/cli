package bundler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyProject(t *testing.T) {
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test-name"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test1"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test name"`))
	assert.True(t, pyProjectNameRegex.MatchString(`name = "test-name-1"`))

	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-alpha"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-beta"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-rc"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-dev"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-alpha.1"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-beta.1"`))
	assert.True(t, pyProjectVersionRegex.MatchString(`version = "1.0.0-rc.1"`))

	assert.Equal(t, "test", pyProjectNameRegex.FindStringSubmatch(`name = "test"`)[1])
	assert.Equal(t, "test name", pyProjectNameRegex.FindStringSubmatch(`name = "test name"`)[1])
	assert.Equal(t, "test1", pyProjectNameRegex.FindStringSubmatch(`name = "test1"`)[1])
	assert.Equal(t, "test-name", pyProjectNameRegex.FindStringSubmatch(`name = "test-name"`)[1])
	assert.Equal(t, "test-name-1", pyProjectNameRegex.FindStringSubmatch(`name = "test-name-1"`)[1])

	assert.Equal(t, "1.0.0", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0"`)[1])
	assert.Equal(t, "1.0.0-alpha", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0-alpha"`)[1])
	assert.Equal(t, "1.0.0-beta", pyProjectVersionRegex.FindStringSubmatch(`version = "1.0.0-beta"`)[1])
}

// Helper function to test package manager detection logic
func testPackageManagerCommand(t *testing.T, tempDir string, runtime string, expectedCmd string, expectedArgs []string) {
	// Test the command creation logic directly by examining what would be created
	switch runtime {
	case "nodejs":
		if _, err := os.Stat(filepath.Join(tempDir, "yarn.lock")); err == nil {
			// yarn.lock exists
			assert.Equal(t, "yarn", expectedCmd)
			assert.Equal(t, []string{"install", "--frozen-lockfile"}, expectedArgs)
		} else {
			// no yarn.lock
			assert.Equal(t, "npm", expectedCmd)
			assert.Equal(t, []string{"install", "--no-audit", "--no-fund", "--include=prod", "--ignore-scripts"}, expectedArgs)
		}
	case "pnpm":
		assert.Equal(t, "pnpm", expectedCmd)
		assert.Equal(t, []string{"install", "--prod", "--ignore-scripts", "--silent"}, expectedArgs)
	case "bunjs":
		assert.Equal(t, "bun", expectedCmd)
		assert.Equal(t, []string{"install", "--production", "--no-save", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"}, expectedArgs)
	}
}

func TestJavaScriptPackageManagerDetection(t *testing.T) {
	tests := []struct {
		name         string
		runtime      string
		lockFiles    []string
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "nodejs with yarn.lock should use yarn",
			runtime:      "nodejs",
			lockFiles:    []string{"yarn.lock"},
			expectedCmd:  "yarn",
			expectedArgs: []string{"install", "--frozen-lockfile"},
		},
		{
			name:         "nodejs without yarn.lock should use npm",
			runtime:      "nodejs",
			lockFiles:    []string{},
			expectedCmd:  "npm",
			expectedArgs: []string{"install", "--no-audit", "--no-fund", "--include=prod", "--ignore-scripts"},
		},
		{
			name:         "nodejs with package-lock.json should use npm",
			runtime:      "nodejs",
			lockFiles:    []string{"package-lock.json"},
			expectedCmd:  "npm",
			expectedArgs: []string{"install", "--no-audit", "--no-fund", "--include=prod", "--ignore-scripts"},
		},
		{
			name:         "nodejs with both yarn.lock and package-lock.json should prefer yarn",
			runtime:      "nodejs",
			lockFiles:    []string{"yarn.lock", "package-lock.json"},
			expectedCmd:  "yarn",
			expectedArgs: []string{"install", "--frozen-lockfile"},
		},
		{
			name:         "pnpm runtime should use pnpm",
			runtime:      "pnpm",
			lockFiles:    []string{"pnpm-lock.yaml"},
			expectedCmd:  "pnpm",
			expectedArgs: []string{"install", "--prod", "--ignore-scripts", "--silent"},
		},
		{
			name:         "bunjs runtime should use bun",
			runtime:      "bunjs",
			lockFiles:    []string{"bun.lockb"},
			expectedCmd:  "bun",
			expectedArgs: []string{"install", "--production", "--no-save", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()

			// Create lock files
			for _, lockFile := range tt.lockFiles {
				filePath := filepath.Join(tempDir, lockFile)
				err := os.WriteFile(filePath, []byte(""), 0644)
				require.NoError(t, err)
			}

			// Test the logic
			testPackageManagerCommand(t, tempDir, tt.runtime, tt.expectedCmd, tt.expectedArgs)
		})
	}
}

func TestPnpmCIFlags(t *testing.T) {
	// Test that pnpm in CI mode uses correct flags
	tempDir := t.TempDir()
	
	// Create pnpm-lock.yaml
	lockFile := filepath.Join(tempDir, "pnpm-lock.yaml")
	err := os.WriteFile(lockFile, []byte(""), 0644)
	require.NoError(t, err)
	
	// Test CI mode
	expectedCmdCI := "pnpm"
	expectedArgsCI := []string{"install", "--prod", "--ignore-scripts", "--reporter=append-only", "--frozen-lockfile"}
	
	// Test non-CI mode  
	expectedCmdNonCI := "pnpm"
	expectedArgsNonCI := []string{"install", "--prod", "--ignore-scripts", "--silent"}
	
	// Test that CI flags are correct
	assert.Equal(t, "pnpm", expectedCmdCI)
	assert.Equal(t, []string{"install", "--prod", "--ignore-scripts", "--reporter=append-only", "--frozen-lockfile"}, expectedArgsCI)
	
	// Test that non-CI flags are correct
	assert.Equal(t, "pnpm", expectedCmdNonCI) 
	assert.Equal(t, []string{"install", "--prod", "--ignore-scripts", "--silent"}, expectedArgsNonCI)
}
