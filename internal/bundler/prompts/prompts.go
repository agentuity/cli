package prompts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentuity/go-common/logger"
)

// VariableInfo holds information about extracted variables
type VariableInfo struct {
	Names []string
}

// FindPromptsYAML finds prompts.yaml in the given directory
func FindPromptsYAML(dir string) string {
	possiblePaths := []string{
		filepath.Join(dir, "src", "prompts", "prompts.yaml"),
		filepath.Join(dir, "src", "prompts", "prompts.yml"),
		filepath.Join(dir, "src", "prompts.yaml"),
		filepath.Join(dir, "src", "prompts.yml"),
		filepath.Join(dir, "prompts.yaml"),
		filepath.Join(dir, "prompts.yml"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// FindSDKGeneratedDir finds the SDK's generated directory in node_modules
func FindSDKGeneratedDir(logger logger.Logger, projectDir string) (string, error) {
	// Try project dir first
	possibleRoots := []string{
		projectDir,
	}

	for _, root := range possibleRoots {
		// For production SDK, generate into the new prompt folder structure
		sdkPath := filepath.Join(root, "node_modules", "@agentuity", "sdk", "dist", "apis", "prompt", "generated")
		if _, err := os.Stat(filepath.Join(root, "node_modules", "@agentuity", "sdk")); err == nil {
			// SDK exists, ensure generated directory exists
			if err := os.MkdirAll(sdkPath, 0755); err != nil {
				logger.Debug("failed to create directory %s: %v", sdkPath, err)
				// Try next location
			} else {
				return sdkPath, nil
			}
		}
		// Fallback to src directory (development)
		sdkPath = filepath.Join(root, "node_modules", "@agentuity", "sdk", "src", "apis", "prompt", "generated")
		if _, err := os.Stat(filepath.Join(root, "node_modules", "@agentuity", "sdk", "src", "apis", "prompt")); err == nil {
			if err := os.MkdirAll(sdkPath, 0755); err != nil {
				logger.Debug("failed to create directory %s: %v", sdkPath, err)
			} else {
				return sdkPath, nil
			}
		}
	}

	return "", fmt.Errorf("could not find @agentuity/sdk in node_modules")
}

// ProcessPrompts finds, parses, and generates prompt files into the SDK
func ProcessPrompts(logger logger.Logger, projectDir string) error {
	// Find prompts.yaml
	promptsPath := FindPromptsYAML(projectDir)
	if promptsPath == "" {
		// No prompts.yaml found - this is OK, not all projects will have prompts
		logger.Debug("No prompts.yaml found in project, skipping prompt generation")
		return nil
	}

	logger.Debug("Found prompts.yaml at: %s", promptsPath)

	// Read and parse prompts.yaml
	data, err := os.ReadFile(promptsPath)
	if err != nil {
		return fmt.Errorf("failed to read prompts.yaml: %w", err)
	}

	promptsList, err := ParsePromptsYAML(data)
	if err != nil {
		return fmt.Errorf("failed to parse prompts: %w", err)
	}

	logger.Debug("Parsed %d prompts from YAML", len(promptsList))

	// Find SDK generated directory
	sdkGeneratedDir, err := FindSDKGeneratedDir(logger, projectDir)
	if err != nil {
		return fmt.Errorf("failed to find SDK directory: %w", err)
	}

	logger.Debug("Found SDK generated directory: %s", sdkGeneratedDir)

	// Generate code using the code generator
	codeGen := NewCodeGenerator(promptsList)

	// Generate index.js file (overwrite SDK's placeholder, following POC pattern)
	jsContent := codeGen.GenerateJavaScript()
	jsPath := filepath.Join(sdkGeneratedDir, "_index.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.js: %w", err)
	}

	// Generate index.d.ts file (overwrite SDK's placeholder, following POC pattern)
	dtsContent := codeGen.GenerateTypeScriptTypes()
	dtsPath := filepath.Join(sdkGeneratedDir, "index.d.ts")
	if err := os.WriteFile(dtsPath, []byte(dtsContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.d.ts: %w", err)
	}

	logger.Info("Generated prompts into SDK: %s and %s", jsPath, dtsPath)

	return nil
}
