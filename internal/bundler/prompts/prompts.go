package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/go-common/logger"
)

// FindAllPromptFiles finds all YAML files in the prompts directory
func FindAllPromptFiles(dir string) []string {
	var promptFiles []string
	seenFiles := make(map[string]bool)

	// Check for prompts directory in various locations
	possibleDirs := []string{
		filepath.Join(dir, "src", "prompts"),
		filepath.Join(dir, "prompts"),
	}

	// Scan all possible directories
	for _, promptDir := range possibleDirs {
		if _, err := os.Stat(promptDir); err == nil {
			// Found prompts directory, scan for YAML files
			entries, err := os.ReadDir(promptDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
					filePath := filepath.Join(promptDir, entry.Name())
					if !seenFiles[filePath] {
						promptFiles = append(promptFiles, filePath)
						seenFiles[filePath] = true
					}
				}
			}
		}
	}

	return promptFiles
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
	// Find all prompt files
	promptFiles := FindAllPromptFiles(projectDir)
	if len(promptFiles) == 0 {
		// No prompt files found - this is OK, not all projects will have prompts
		logger.Debug("No prompt files found in project, skipping prompt generation")
		return nil
	}

	logger.Debug("Found %d prompt files: %v", len(promptFiles), promptFiles)

	// Parse all prompt files and combine prompts
	var allPrompts []Prompt
	for _, promptFile := range promptFiles {
		data, err := os.ReadFile(promptFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", promptFile, err)
		}

		promptsList, err := ParsePromptsYAML(data)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", promptFile, err)
		}

		allPrompts = append(allPrompts, promptsList...)
		logger.Debug("Parsed %d prompts from %s", len(promptsList), promptFile)
	}

	logger.Debug("Total prompts parsed: %d", len(allPrompts))

	// Find SDK generated directory
	sdkGeneratedDir, err := FindSDKGeneratedDir(logger, projectDir)
	if err != nil {
		return fmt.Errorf("failed to find SDK directory: %w", err)
	}

	logger.Debug("Found SDK generated directory: %s", sdkGeneratedDir)

	// Generate code using the code generator
	codeGen := NewCodeGenerator(allPrompts)

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
