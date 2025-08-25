package bundler

import (
	"context"
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
func testPackageManagerCommand(t *testing.T, tempDir string, runtime string, isCI bool, expectedCmd string, expectedArgs []string) {
	ctx := BundleContext{
		Context: context.Background(),
		Logger:  nil, // nil logger will skip bun lockfile generation in tests
		CI:      isCI,
	}
	
	actualCmd, actualArgs, err := getJSInstallCommand(ctx, tempDir, runtime)
	require.NoError(t, err)
	
	assert.Equal(t, expectedCmd, actualCmd)
	assert.Equal(t, expectedArgs, actualArgs)
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
			lockFiles:    []string{"bun.lockb", "package.json"},
			expectedCmd:  "bun",
			expectedArgs: []string{"install", "--production", "--ignore-scripts", "--no-progress", "--no-summary", "--silent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()

			// Create lock files and package.json
			for _, lockFile := range tt.lockFiles {
				filePath := filepath.Join(tempDir, lockFile)
				var content []byte
				if lockFile == "package.json" {
					content = []byte(`{"name": "test-package", "version": "1.0.0"}`)
				} else {
					content = []byte("")
				}
				err := os.WriteFile(filePath, content, 0644)
				require.NoError(t, err)
			}

			// Test the logic with CI=false
			testPackageManagerCommand(t, tempDir, tt.runtime, false, tt.expectedCmd, tt.expectedArgs)
		})
	}
}

func TestPnpmCIFlags(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pnpm-lock.yaml"), []byte(""), 0644))
	// CI=true
	testPackageManagerCommand(
		t,
		tempDir,
		"pnpm",
		true,
		"pnpm",
		[]string{"install", "--prod", "--ignore-scripts", "--reporter=append-only", "--frozen-lockfile"},
	)
	// CI=false
	testPackageManagerCommand(
		t,
		tempDir,
		"pnpm",
		false,
		"pnpm",
		[]string{"install", "--prod", "--ignore-scripts", "--silent"},
	)
}
