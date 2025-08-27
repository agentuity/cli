package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/slice"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const emptyProjectDescription = "No description provided"

func fetchFileFromURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func extractImportsFromTS(content []byte, filename string) ([]string, []string, error) {
	// Determine loader based on file extension
	loader := api.LoaderTS
	if strings.HasSuffix(filename, ".js") {
		loader = api.LoaderJS
	} else if strings.HasSuffix(filename, ".jsx") {
		loader = api.LoaderJSX
	} else if strings.HasSuffix(filename, ".tsx") {
		loader = api.LoaderTSX
	}

	// Parse the file using esbuild
	result := api.Transform(string(content), api.TransformOptions{
		Loader: loader,
		Format: api.FormatESModule,
	})

	if len(result.Errors) > 0 {
		return nil, nil, fmt.Errorf("esbuild parse error: %s", result.Errors[0].Text)
	}

	// Extract imports from the transformed code
	// Since esbuild doesn't directly expose AST, we'll use regex on the original content
	// but this is more reliable than pure regex since we know the file is valid TypeScript
	externalImports := make(map[string]bool)
	localImports := make(map[string]bool)

	// Match ES6 import statements
	importRegex := regexp.MustCompile(`import\s+(?:{[^}]*}|\*\s+as\s+\w+|\w+)?\s*(?:,\s*(?:{[^}]*}|\*\s+as\s+\w+|\w+))?\s*from\s+['"]([^'"]+)['"]`)
	matches := importRegex.FindAllStringSubmatch(string(content), -1)

	for _, match := range matches {
		if len(match) > 1 {
			pkg := match[1]
			// Check if it's a local import (starting with . or /)
			if strings.HasPrefix(pkg, ".") || strings.HasPrefix(pkg, "/") {
				localImports[pkg] = true
			} else {
				externalImports[pkg] = true
			}
		}
	}

	// Match CommonJS require statements
	requireRegex := regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	requireMatches := requireRegex.FindAllStringSubmatch(string(content), -1)

	for _, match := range requireMatches {
		if len(match) > 1 {
			pkg := match[1]
			// Check if it's a local import
			if strings.HasPrefix(pkg, ".") || strings.HasPrefix(pkg, "/") {
				localImports[pkg] = true
			} else {
				externalImports[pkg] = true
			}
		}
	}

	// Convert maps to slices
	var externalResult []string
	for pkg := range externalImports {
		externalResult = append(externalResult, pkg)
	}

	var localResult []string
	for pkg := range localImports {
		localResult = append(localResult, pkg)
	}

	sort.Strings(externalResult)
	sort.Strings(localResult)
	return externalResult, localResult, nil
}

func extractImportsFromPython(content []byte) ([]string, []string, error) {
	externalImports := make(map[string]bool)
	localImports := make(map[string]bool)

	// Match Python import statements
	importRegex := regexp.MustCompile(`(?m)^(?:from\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\s+import|import\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*))`)
	matches := importRegex.FindAllStringSubmatch(string(content), -1)

	for _, match := range matches {
		var pkg string
		if len(match) > 1 && match[1] != "" {
			pkg = match[1] // from X import
		} else if len(match) > 2 && match[2] != "" {
			pkg = match[2] // import X
		}

		if pkg != "" {
			// Check if it's a local import (starts with .)
			if strings.HasPrefix(pkg, ".") {
				localImports[pkg] = true
			} else {
				// Get the root package name (before first dot)
				rootPkg := strings.Split(pkg, ".")[0]
				externalImports[rootPkg] = true
			}
		}
	}

	// Convert maps to slices
	var externalResult []string
	for pkg := range externalImports {
		externalResult = append(externalResult, pkg)
	}

	var localResult []string
	for pkg := range localImports {
		localResult = append(localResult, pkg)
	}

	sort.Strings(externalResult)
	sort.Strings(localResult)
	return externalResult, localResult, nil
}

func analyzeAllAgents(logger logger.Logger, githubURL string, agentConfig *project.Project, baseRawURL, branch string) {
	allImports := make(map[string]bool)
	agentSrcDir := agentConfig.Bundler.AgentConfig.Dir

	tui.ShowSpinner("Analyzing all agents...", func() {
		for _, agent := range agentConfig.Agents {
			normalizedName := normalAgentName(agent.Name, agentConfig.IsPython())

			// Try different possible file paths for each agent
			possiblePaths := []string{
				agentSrcDir + "/" + normalizedName + "/index.ts",
				agentSrcDir + "/" + normalizedName + "/index.js",
				agentSrcDir + "/" + normalizedName + "/index.py",
				agentSrcDir + "/" + normalizedName + "/main.py",
				agentSrcDir + "/" + agent.Name + "/index.ts",
				agentSrcDir + "/" + agent.Name + "/index.js",
				agentSrcDir + "/" + agent.Name + "/index.py",
				agentSrcDir + "/" + agent.Name + "/main.py",
			}

			for _, path := range possiblePaths {
				fileURL := baseRawURL + branch + "/" + path
				content, err := fetchFileFromURL(fileURL)
				if err != nil {
					continue // Try next path
				}

				// Extract imports based on file type
				var externalImports, _ []string
				if strings.HasSuffix(path, ".py") {
					externalImports, _, err = extractImportsFromPython(content)
				} else {
					externalImports, _, err = extractImportsFromTS(content, path)
				}

				if err != nil {
					logger.Info("Warning: Could not extract imports from %s: %v", path, err)
					continue
				}

				// Add external imports to global set
				for _, pkg := range externalImports {
					allImports[pkg] = true
				}
				break // Found and processed file for this agent
			}
		}
	})

	if len(allImports) == 0 {
		tui.ShowWarning("No package dependencies found across all agents")
		return
	}

	// Convert to sorted slice
	var sortedImports []string
	for pkg := range allImports {
		sortedImports = append(sortedImports, pkg)
	}
	sort.Strings(sortedImports)

	fmt.Printf("\n%s\n", tui.Bold("Complete Project Dependencies:"))
	fmt.Printf("%s (%d packages)\n\n", tui.Muted("All unique packages used across "+fmt.Sprintf("%d", len(agentConfig.Agents))+" agents"), len(sortedImports))

	for _, pkg := range sortedImports {
		fmt.Printf("  • %s\n", pkg)
	}

	// Show package.json or requirements.txt suggestion
	if agentConfig.IsPython() {
		fmt.Printf("\n%s\n", tui.Muted("💡 These could be added to your requirements.txt or pyproject.toml"))
	} else {
		fmt.Printf("\n%s\n", tui.Muted("💡 These could be added to your package.json dependencies"))
	}
}

func listDirectoryContents(baseURL, dirPath string) ([]string, error) {
	// Convert raw GitHub URL to API URL for directory listing
	// Example: raw.githubusercontent.com/user/repo/main/agents/agent-name
	// becomes: api.github.com/repos/user/repo/contents/agents/agent-name?ref=main

	parts := strings.Split(baseURL, "/")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid GitHub URL format")
	}

	user := parts[3]
	repo := parts[4]
	branch := parts[5]

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", user, repo, dirPath, branch)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var files []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.Unmarshal(body, &files); err != nil {
		return nil, err
	}

	var fileNames []string
	for _, file := range files {
		if file.Type == "file" {
			// Only include source code files
			if strings.HasSuffix(file.Name, ".ts") ||
				strings.HasSuffix(file.Name, ".js") ||
				strings.HasSuffix(file.Name, ".tsx") ||
				strings.HasSuffix(file.Name, ".jsx") ||
				strings.HasSuffix(file.Name, ".py") {
				fileNames = append(fileNames, file.Name)
			}
		}
	}

	return fileNames, nil
}

type AgentFile struct {
	Path            string
	Content         []byte
	ExternalImports []string
	LocalImports    []string
}

type AgentDetails struct {
	Name  string
	Files []AgentFile
}

func resolveImportPath(basePath, importPath string) []string {
	// Convert relative import to actual file paths
	// Handle cases like:
	// './utils' -> './utils.ts', './utils.js', './utils/index.ts', etc.
	// '../config/settings' -> '../config/settings.ts', etc.

	var possiblePaths []string

	// Clean the base path (remove filename)
	baseDir := filepath.Dir(basePath)

	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		// Relative import
		resolvedBase := filepath.Join(baseDir, importPath)
		resolvedBase = strings.ReplaceAll(resolvedBase, "\\", "/") // Ensure forward slashes

		// Try different extensions and index files
		possiblePaths = []string{
			resolvedBase + ".ts",
			resolvedBase + ".js",
			resolvedBase + ".tsx",
			resolvedBase + ".jsx",
			resolvedBase + ".py",
			resolvedBase + "/index.ts",
			resolvedBase + "/index.js",
			resolvedBase + "/index.tsx",
			resolvedBase + "/index.jsx",
			resolvedBase + "/index.py",
		}
	}

	return possiblePaths
}

func resolveLocalImportsRecursively(baseRawURL, branch string, initialFiles []AgentFile, visitedFiles map[string]bool, allImports map[string]bool) []AgentFile {
	var allFiles []AgentFile
	filesToProcess := make([]AgentFile, len(initialFiles))
	copy(filesToProcess, initialFiles)

	for len(filesToProcess) > 0 {
		// Take the first file to process
		currentFile := filesToProcess[0]
		filesToProcess = filesToProcess[1:]

		// Skip if already visited
		if visitedFiles[currentFile.Path] {
			continue
		}
		visitedFiles[currentFile.Path] = true

		// Add to result
		allFiles = append(allFiles, currentFile)

		// Add external imports to global set
		for _, imp := range currentFile.ExternalImports {
			allImports[imp] = true
		}

		// Process local imports
		for _, localImport := range currentFile.LocalImports {
			possiblePaths := resolveImportPath(currentFile.Path, localImport)

			// Try to fetch the first existing file
			for _, possiblePath := range possiblePaths {
				fileURL := baseRawURL + branch + "/" + possiblePath
				content, err := fetchFileFromURL(fileURL)
				if err != nil {
					continue // Try next possible path
				}

				// Skip if already visited
				if visitedFiles[possiblePath] {
					break
				}

				// Parse imports from this file
				var externalImports, localImports []string
				var parseErr error

				if strings.HasSuffix(possiblePath, ".py") {
					externalImports, localImports, parseErr = extractImportsFromPython(content)
				} else if strings.HasSuffix(possiblePath, ".ts") || strings.HasSuffix(possiblePath, ".js") ||
					strings.HasSuffix(possiblePath, ".tsx") || strings.HasSuffix(possiblePath, ".jsx") {
					externalImports, localImports, parseErr = extractImportsFromTS(content, possiblePath)
				}

				if parseErr != nil {
					// Still include the file even if we can't parse imports
					externalImports = []string{}
					localImports = []string{}
				}

				// Create file object and add to processing queue
				newFile := AgentFile{
					Path:            possiblePath,
					Content:         content,
					ExternalImports: externalImports,
					LocalImports:    localImports,
				}
				filesToProcess = append(filesToProcess, newFile)
				break // Found the file, no need to try other paths
			}
		}
	}

	return allFiles
}

func analyzeSelectedAgents(logger logger.Logger, selectedAgents []project.AgentConfig, agentConfig *project.Project, baseRawURL, branch, projectDir string) {
	allImports := make(map[string]bool)
	agentSrcDir := agentConfig.Bundler.AgentConfig.Dir
	agentDetails := make(map[string]AgentDetails)

	tui.ShowSpinner(fmt.Sprintf("Analyzing %d selected agents and resolving imports...", len(selectedAgents)), func() {
		for _, agent := range selectedAgents {
			normalizedName := normalAgentName(agent.Name, agentConfig.IsPython())

			// Try different possible directory names
			possibleDirs := []string{
				agentSrcDir + "/" + normalizedName,
				agentSrcDir + "/" + agent.Name,
			}

			var initialFiles []AgentFile
			foundDir := false

			for _, dirPath := range possibleDirs {
				// Use GitHub API to list all files in the directory
				fileNames, err := listDirectoryContents(baseRawURL, dirPath)
				if err != nil {
					continue // Try next directory
				}

				if len(fileNames) == 0 {
					continue // No files found, try next directory
				}

				foundDir = true

				// Fetch each file found in the directory
				for _, fileName := range fileNames {
					filePath := dirPath + "/" + fileName
					fileURL := baseRawURL + branch + "/" + filePath
					content, err := fetchFileFromURL(fileURL)
					if err != nil {
						logger.Info("Warning: Could not fetch %s: %v", filePath, err)
						continue
					}

					// Extract imports based on file type
					var externalImports, localImports []string
					if strings.HasSuffix(fileName, ".py") {
						externalImports, localImports, err = extractImportsFromPython(content)
					} else if strings.HasSuffix(fileName, ".ts") || strings.HasSuffix(fileName, ".js") ||
						strings.HasSuffix(fileName, ".tsx") || strings.HasSuffix(fileName, ".jsx") {
						externalImports, localImports, err = extractImportsFromTS(content, fileName)
					}

					if err != nil {
						logger.Info("Warning: Could not extract imports from %s: %v", filePath, err)
						externalImports = []string{} // Still include file even if we can't parse imports
						localImports = []string{}
					}

					// Store file details
					initialFiles = append(initialFiles, AgentFile{
						Path:            filePath,
						Content:         content,
						ExternalImports: externalImports,
						LocalImports:    localImports,
					})
				}

				if foundDir {
					break // Found files in this directory, no need to try other directories
				}
			}

			if foundDir && len(initialFiles) > 0 {
				// Resolve local imports recursively
				visitedFiles := make(map[string]bool)
				allAgentFiles := resolveLocalImportsRecursively(baseRawURL, branch, initialFiles, visitedFiles, allImports)

				agentDetails[agent.Name] = AgentDetails{
					Name:  agent.Name,
					Files: allAgentFiles,
				}
			}
		}
	})

	// Display results
	fmt.Printf("\n%s\n", tui.Bold(fmt.Sprintf("Analysis Results (%d agents)", len(selectedAgents))))
	fmt.Printf("%s\n", strings.Repeat("═", 60))

	// Show agent details with file listings
	for _, agent := range selectedAgents {
		if details, exists := agentDetails[agent.Name]; exists {
			fmt.Printf("\n%s %s\n", tui.Bold("✓"), agent.Name)
			fmt.Printf("  %s (%d files)\n", tui.Muted("Found in directory"), len(details.Files))

			// List all files found
			fmt.Printf("\n  %s\n", tui.Bold("Files:"))
			for _, file := range details.Files {
				fileName := filepath.Base(file.Path)
				fileSize := len(file.Content)
				importCount := len(file.ExternalImports) + len(file.LocalImports)

				fmt.Printf("    📄 %s", tui.Bold(fileName))
				fmt.Printf(" %s", tui.Muted(fmt.Sprintf("(%d bytes", fileSize)))
				if importCount > 0 {
					fmt.Printf(", %d ext + %d local imports)", len(file.ExternalImports), len(file.LocalImports))
				} else {
					fmt.Printf(")")
				}
				fmt.Printf("\n")

				// Show external imports for this file if any
				if len(file.ExternalImports) > 0 {
					fmt.Printf("       %s %s\n", tui.Muted("external:"), strings.Join(file.ExternalImports, ", "))
				}

				// Show local imports for this file if any
				if len(file.LocalImports) > 0 {
					fmt.Printf("       %s %s\n", tui.Muted("local:"), strings.Join(file.LocalImports, ", "))
				}
			}
		} else {
			fmt.Printf("\n%s %s\n", tui.Warning("✗"), agent.Name)
			fmt.Printf("  %s\n", tui.Muted("No source files found"))
		}
	}

	// Show combined dependencies
	if len(allImports) > 0 {
		// Convert to sorted slice
		var sortedImports []string
		for pkg := range allImports {
			sortedImports = append(sortedImports, pkg)
		}
		sort.Strings(sortedImports)

		fmt.Printf("\n%s\n", strings.Repeat("═", 60))
		fmt.Printf("%s\n", tui.Bold("Combined Dependencies"))
		fmt.Printf("%s (%d packages)\n\n", tui.Muted(fmt.Sprintf("All unique packages across %d selected agents", len(selectedAgents))), len(sortedImports))

		for _, pkg := range sortedImports {
			fmt.Printf("  • %s\n", pkg)
		}

		// Show package manager suggestion
		if agentConfig.IsPython() {
			fmt.Printf("\n%s\n", tui.Muted("💡 Add these to your requirements.txt or pyproject.toml"))
		} else {
			fmt.Printf("\n%s\n", tui.Muted("💡 Add these to your package.json dependencies"))
		}
	} else {
		fmt.Printf("\n%s\n", tui.Warning("No external dependencies found in selected agents"))
	}

	// Offer to import files locally
	if len(agentDetails) > 0 {
		fmt.Printf("\n%s\n", strings.Repeat("═", 60))
		if tui.Ask(logger, "Would you like to import these files to your local project?", true) {
			importFilesLocally(logger, agentDetails, agentConfig, allImports, projectDir, baseRawURL, branch)
		}
	}
}

func importFilesLocally(logger logger.Logger, agentDetails map[string]AgentDetails, agentConfig *project.Project, allImports map[string]bool, projectDir, baseRawURL, branch string) {
	var totalFiles int
	for _, details := range agentDetails {
		totalFiles += len(details.Files)
	}

	tui.ShowSpinner(fmt.Sprintf("Importing %d files locally...", totalFiles), func() {
		// Create files with same directory structure
		for _, details := range agentDetails {
			for _, file := range details.Files {
				// Create local file path
				localPath := filepath.Join(projectDir, file.Path)

				// Create directory if it doesn't exist
				dir := filepath.Dir(localPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					logger.Info("Warning: Could not create directory %s: %v", dir, err)
					continue
				}

				// Write file content
				if err := os.WriteFile(localPath, file.Content, 0644); err != nil {
					logger.Info("Warning: Could not write file %s: %v", localPath, err)
					continue
				}
			}
		}

		// Update package.json or requirements.txt with dependencies
		if err := updateDependencies(logger, projectDir, allImports, agentConfig.IsPython(), baseRawURL, branch); err != nil {
			logger.Info("Warning: Could not update dependencies: %v", err)
		}
	})

	tui.ShowSuccess("Successfully imported %d files to local project", totalFiles)

	// Show summary of what was imported
	fmt.Printf("\n%s\n", tui.Bold("Import Summary:"))
	for agentName, details := range agentDetails {
		fmt.Printf("  %s %s (%d files)\n", tui.Bold("✓"), agentName, len(details.Files))
		for _, file := range details.Files {
			relPath, _ := filepath.Rel(projectDir, filepath.Join(projectDir, file.Path))
			fmt.Printf("    📄 %s\n", tui.Muted(relPath))
		}
	}

	if len(allImports) > 0 {
		if agentConfig.IsPython() {
			fmt.Printf("\n%s Updated requirements.txt with %d dependencies\n", tui.Bold("📦"), len(allImports))
		} else {
			fmt.Printf("\n%s Updated package.json with %d dependencies\n", tui.Bold("📦"), len(allImports))
		}
	}

	// Scan for environment variables
	envVars := scanForEnvironmentVariables(logger, agentDetails, projectDir)
	if len(envVars) > 0 {
		fmt.Printf("\n%s Found %d environment variables in the imported code:\n", tui.Bold("🔧"), len(envVars))
		for _, envVar := range envVars {
			fmt.Printf("  %s %s\n", tui.Bold("•"), tui.Muted(envVar))
		}
		fmt.Printf("\n%s Make sure to set these environment variables before running the agents.\n", tui.Bold("💡"))

		// Create placeholder environment files
		createEnvironmentFiles(logger, envVars, projectDir)
	}

	// Install dependencies
	if len(allImports) > 0 {
		if err := installDependencies(logger, projectDir, agentConfig.IsPython(), agentConfig); err != nil {
			logger.Info("Warning: Could not install dependencies: %v", err)
		}
	}

	// Create agents in Agentuity Cloud
	if tui.Ask(logger, "Would you like to create these agents in Agentuity Cloud?", true) {
		createImportedAgents(logger, agentDetails, projectDir)
	}
}

func detectPackageManager(projectDir string, agentConfig *project.Project) string {
	// First check the agentuity.yaml bundler configuration
	if agentConfig != nil && agentConfig.Bundler != nil {
		switch agentConfig.Bundler.Runtime {
		case "bunjs":
			return "bun"
		case "deno":
			return "deno"
		case "nodejs":
			// For nodejs, check for specific lock files to determine package manager
			if util.Exists(filepath.Join(projectDir, "bun.lock")) {
				return "bun"
			}
			if util.Exists(filepath.Join(projectDir, "pnpm-lock.yaml")) {
				return "pnpm"
			}
			if util.Exists(filepath.Join(projectDir, "yarn.lock")) {
				return "yarn"
			}
			return "npm"
		}
	}

	// Fallback to lock file detection if bundler config doesn't specify
	if util.Exists(filepath.Join(projectDir, "bun.lock")) {
		return "bun"
	}
	if util.Exists(filepath.Join(projectDir, "pnpm-lock.yaml")) {
		return "pnpm"
	}
	if util.Exists(filepath.Join(projectDir, "yarn.lock")) {
		return "yarn"
	}
	if util.Exists(filepath.Join(projectDir, "package-lock.json")) {
		return "npm"
	}
	if util.Exists(filepath.Join(projectDir, "package.json")) {
		return "npm" // Default to npm if package.json exists
	}
	return "npm" // Default fallback
}

// scanForEnvironmentVariables scans imported files for process.env usage
func scanForEnvironmentVariables(logger logger.Logger, agentDetails map[string]AgentDetails, projectDir string) []string {
	envVarSet := make(map[string]bool)

	// Regex patterns for different environment variable access patterns
	patterns := []*regexp.Regexp{
		// JavaScript/TypeScript patterns
		regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`),           // process.env.VAR_NAME
		regexp.MustCompile(`process\.env\[['"]([A-Z_][A-Z0-9_]*)['"]\]`), // process.env['VAR_NAME'] or process.env["VAR_NAME"]

		// Python patterns
		regexp.MustCompile(`os\.environ\.get\(['"]([A-Z_][A-Z0-9_]*)['"]\)`), // os.environ.get('VAR_NAME')
		regexp.MustCompile(`os\.environ\[['"]([A-Z_][A-Z0-9_]*)['"]\]`),      // os.environ['VAR_NAME']
		regexp.MustCompile(`os\.getenv\(['"]([A-Z_][A-Z0-9_]*)['"]\)`),       // os.getenv('VAR_NAME')
	}

	for _, details := range agentDetails {
		for _, file := range details.Files {
			filePath := filepath.Join(projectDir, file.Path)

			// Only scan text files (skip binary files)
			ext := strings.ToLower(filepath.Ext(file.Path))
			if !isTextFile(ext) {
				continue
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				logger.Info("Warning: Could not read file %s for environment variable scanning: %v", file.Path, err)
				continue
			}

			// Scan content with regex patterns
			for _, pattern := range patterns {
				matches := pattern.FindAllStringSubmatch(string(content), -1)
				for _, match := range matches {
					if len(match) > 1 {
						envVar := match[1]
						// Filter out common non-environment variables
						if !isCommonNonEnvVar(envVar) {
							envVarSet[envVar] = true
						}
					}
				}
			}
		}
	}

	// Convert set to sorted slice
	var envVars []string
	for envVar := range envVarSet {
		envVars = append(envVars, envVar)
	}
	sort.Strings(envVars)

	return envVars
}

// isTextFile checks if a file extension indicates a text file
func isTextFile(ext string) bool {
	textExts := map[string]bool{
		".js":   true,
		".ts":   true,
		".jsx":  true,
		".tsx":  true,
		".py":   true,
		".json": true,
		".yaml": true,
		".yml":  true,
		".md":   true,
		".txt":  true,
		".env":  true,
		".sh":   true,
		".bash": true,
		".zsh":  true,
	}
	return textExts[ext]
}

// isCommonNonEnvVar filters out common variable names that aren't environment variables
func isCommonNonEnvVar(varName string) bool {
	common := map[string]bool{
		"NODE_ENV": false, // This IS an env var
		"PATH":     false, // This IS an env var
		"HOME":     false, // This IS an env var
		"PWD":      false, // This IS an env var
		"USER":     false, // This IS an env var
		"SHELL":    false, // This IS an env var
		// Add actual non-env vars here if needed
	}

	// If we explicitly know it's not an env var, filter it out
	if isEnv, exists := common[varName]; exists && !isEnv {
		return true
	}

	// Default: assume it's an environment variable
	return false
}

// createEnvironmentFiles creates placeholder .env and .env.development files
func createEnvironmentFiles(logger logger.Logger, envVars []string, projectDir string) {
	files := []struct {
		name        string
		description string
	}{
		{".env", "production/general environment variables"},
		{".env.development", "development environment variables"},
	}

	for _, file := range files {
		filePath := filepath.Join(projectDir, file.name)

		// Check if file already exists
		var existingVars map[string]string
		if util.Exists(filePath) {
			existingVars = readExistingEnvFile(filePath)
		} else {
			existingVars = make(map[string]string)
		}

		// Create content with header comment
		var content strings.Builder
		content.WriteString("# Environment variables for imported agents\n")
		content.WriteString("# Fill in the actual values for your environment\n")
		if file.name == ".env.development" {
			content.WriteString("# This file is for development environment\n")
		}
		content.WriteString("\n")

		// Add existing variables first (to preserve order and existing values)
		var addedVars = make(map[string]bool)
		for key, value := range existingVars {
			content.WriteString(fmt.Sprintf("%s=%s\n", key, value))
			addedVars[key] = true
		}

		// Add new variables as placeholders if they don't exist
		hasNewVars := false
		for _, envVar := range envVars {
			if !addedVars[envVar] {
				if !hasNewVars {
					content.WriteString("\n# New variables found in imported code:\n")
					hasNewVars = true
				}
				content.WriteString(fmt.Sprintf("%s=\n", envVar))
			}
		}

		// Write the file
		if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
			logger.Info("Warning: Could not create %s: %v", file.name, err)
			continue
		}

		if util.Exists(filePath) && len(existingVars) > 0 {
			fmt.Printf("  %s Updated %s with new variables\n", tui.Bold("✓"), tui.Muted(file.name))
		} else {
			fmt.Printf("  %s Created %s for %s\n", tui.Bold("✓"), tui.Muted(file.name), file.description)
		}
	}
}

// readExistingEnvFile reads an existing .env file and returns key-value pairs
func readExistingEnvFile(filePath string) map[string]string {
	vars := make(map[string]string)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return vars
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vars[key] = value
		}
	}

	return vars
}

func installDependencies(logger logger.Logger, projectDir string, isPython bool, agentConfig *project.Project) error {
	if isPython {
		return installPythonDependencies(logger, projectDir)
	}
	return installJSDependencies(logger, projectDir, agentConfig)
}

func installJSDependencies(logger logger.Logger, projectDir string, agentConfig *project.Project) error {
	packageManager := detectPackageManager(projectDir, agentConfig)

	var cmd string
	switch packageManager {
	case "bun":
		cmd = "bun install"
	case "pnpm":
		cmd = "pnpm install"
	case "yarn":
		cmd = "yarn install"
	case "deno":
		// Deno doesn't need explicit install, it handles dependencies automatically
		tui.ShowSuccess("Deno runtime detected - dependencies will be resolved automatically")
		return nil
	default:
		cmd = "npm install"
	}

	var installErr error
	tui.ShowSpinner(fmt.Sprintf("Installing dependencies with %s...", packageManager), func() {
		// Execute install command
		parts := strings.Fields(cmd)
		execCmd := exec.Command(parts[0], parts[1:]...)
		execCmd.Dir = projectDir
		execCmd.Stdout = nil // Suppress output
		execCmd.Stderr = nil
		installErr = execCmd.Run()
	})

	if installErr != nil {
		return fmt.Errorf("failed to install dependencies with %s: %w", packageManager, installErr)
	}

	tui.ShowSuccess("Dependencies installed successfully with %s", packageManager)
	return nil
}

func installPythonDependencies(logger logger.Logger, projectDir string) error {
	// Check if uv is available first, then fall back to pip
	var cmd string
	var packageManager string

	if _, err := exec.LookPath("uv"); err == nil {
		cmd = "uv pip install -r requirements.txt"
		packageManager = "uv"
	} else if _, err := exec.LookPath("pip"); err == nil {
		cmd = "pip install -r requirements.txt"
		packageManager = "pip"
	} else {
		return fmt.Errorf("neither uv nor pip found in PATH")
	}

	var installErr error
	tui.ShowSpinner(fmt.Sprintf("Installing dependencies with %s...", packageManager), func() {
		parts := strings.Fields(cmd)
		execCmd := exec.Command(parts[0], parts[1:]...)
		execCmd.Dir = projectDir
		execCmd.Stdout = nil // Suppress output
		execCmd.Stderr = nil
		installErr = execCmd.Run()
	})

	if installErr != nil {
		return fmt.Errorf("failed to install dependencies with %s: %w", packageManager, installErr)
	}

	tui.ShowSuccess("Dependencies installed successfully with %s", packageManager)
	return nil
}

func createImportedAgents(logger logger.Logger, agentDetails map[string]AgentDetails, projectDir string) {
	// We need access to the project context for agent creation
	ctx := context.Background()

	// Load the local project to get API access
	apiUrl, _, _ := util.GetURLs(logger)

	// Try to get authentication
	apikey, _, ok := util.TryLoggedIn()
	if !ok {
		tui.ShowWarning("Not logged in to Agentuity. Please run 'agentuity auth login' first.")
		return
	}
	token := apikey

	// Load local project
	theproject := project.NewProject()
	if err := theproject.Load(projectDir); err != nil {
		logger.Info("Warning: Could not load local project for agent creation: %v", err)
		return
	}

	if theproject.ProjectId == "" {
		tui.ShowWarning("No project found. Please run 'agentuity new' to create a project first.")
		return
	}

	var createdCount int
	var createdAgents []string

	for agentName := range agentDetails {
		finalAgentName := agentName

		// Try to create the agent, handling name conflicts
		for attempt := 1; attempt <= 3; attempt++ {
			agentID, err := agent.CreateAgent(ctx, logger, apiUrl, token, theproject.ProjectId, finalAgentName, "Imported agent", "project")
			if err != nil {
				// Check if this is a name conflict error
				if strings.Contains(err.Error(), "already exists with the name") || strings.Contains(err.Error(), "Agent already exists") {
					fmt.Printf("\n%s Agent name '%s' already exists.\n", tui.Bold("⚠️"), finalAgentName)

					if tui.HasTTY {
						options := []tui.Option{
							{ID: "rename", Text: "Create with a different name"},
							{ID: "skip", Text: "Skip this agent"},
						}

						choice := tui.Select(logger,
							fmt.Sprintf("How would you like to handle the conflict for '%s'?", finalAgentName),
							"",
							options)

						if choice == "skip" {
							fmt.Printf("  %s Skipped agent '%s'\n", tui.Bold("⏭️"), finalAgentName)
							break
						} else {
							// Get new name from user
							newName := tui.Input(logger, fmt.Sprintf("Enter a new name for '%s':", finalAgentName), "")
							if newName == "" {
								fmt.Printf("  %s Skipped agent '%s' (no name provided)\n", tui.Bold("⏭️"), finalAgentName)
								break
							}
							finalAgentName = newName
							continue
						}
					} else {
						// No TTY, suggest a name with suffix
						suggestedName := fmt.Sprintf("%s-imported-%d", agentName, attempt)
						finalAgentName = suggestedName
						fmt.Printf("  %s Trying with name '%s'...\n", tui.Bold("🔄"), finalAgentName)
						continue
					}
				} else {
					// Other error, not a name conflict
					logger.Info("Warning: Could not create agent %s: %v", finalAgentName, err)
					break
				}
			} else {
				// Success! Add to project config
				theproject.Agents = append(theproject.Agents, project.AgentConfig{
					ID:          agentID,
					Name:        finalAgentName,
					Description: "Imported agent",
				})

				createdCount++
				createdAgents = append(createdAgents, finalAgentName)

				if finalAgentName != agentName {
					fmt.Printf("  %s Created agent '%s' (renamed from '%s')\n", tui.Bold("✓"), finalAgentName, agentName)
				}
				break
			}
		}
	}

	// Save updated project
	if err := theproject.Save(projectDir); err != nil {
		logger.Info("Warning: Could not save project after agent creation: %v", err)
	}

	if createdCount > 0 {
		tui.ShowSuccess("Created %d agents in Agentuity Cloud with project key authentication", createdCount)

		fmt.Printf("\n%s\n", tui.Bold("Agents Created:"))
		for _, agentName := range createdAgents {
			fmt.Printf("  %s %s\n", tui.Bold("✓"), agentName)
		}

		fmt.Printf("\n%s\n", tui.Muted("You can now deploy your agents with: agentuity deploy"))
	} else {
		tui.ShowWarning("No agents were created. Check the warnings above.")
	}
}

func updateDependencies(logger logger.Logger, projectDir string, dependencies map[string]bool, isPython bool, baseRawURL, branch string) error {
	if isPython {
		return updateRequirementsTxt(logger, projectDir, dependencies, baseRawURL, branch)
	}
	return updatePackageJson(logger, projectDir, dependencies, baseRawURL, branch)
}

func fetchRemotePackageJson(baseRawURL, branch string) (map[string]interface{}, error) {
	packageJsonURL := baseRawURL + branch + "/package.json"
	content, err := fetchFileFromURL(packageJsonURL)
	if err != nil {
		return nil, err
	}

	var packageData map[string]interface{}
	if err := json.Unmarshal(content, &packageData); err != nil {
		return nil, err
	}

	return packageData, nil
}

// findRelatedDevDependencies looks for related dev dependencies for a given package
// For example, for "js-yaml" it would look for "@types/js-yaml" in both dependencies and devDependencies
func findRelatedDevDependencies(packageName string, remoteDeps, remoteDevDeps map[string]interface{}) map[string]string {
	relatedDeps := make(map[string]string)

	// Common patterns for related dev dependencies
	possibleDevDeps := []string{
		"@types/" + packageName,                                // TypeScript types
		"@types/" + strings.ReplaceAll(packageName, "/", "__"), // Scoped packages
	}

	// Handle scoped packages differently (e.g., @babel/core -> @types/babel__core)
	if strings.HasPrefix(packageName, "@") {
		parts := strings.Split(packageName[1:], "/") // Remove @ and split
		if len(parts) == 2 {
			possibleDevDeps = append(possibleDevDeps, "@types/"+parts[0]+"__"+parts[1])
		}
	}

	// Check if any of these potential dev dependencies exist in either section
	for _, devDep := range possibleDevDeps {
		// Check in devDependencies first (most common location)
		if remoteDevDeps != nil {
			if version, exists := remoteDevDeps[devDep].(string); exists {
				relatedDeps[devDep] = version
				continue
			}
		}

		// Check in dependencies (in case it was put in the wrong spot)
		if remoteDeps != nil {
			if version, exists := remoteDeps[devDep].(string); exists {
				relatedDeps[devDep] = version
			}
		}
	}

	return relatedDeps
}

func updatePackageJson(logger logger.Logger, projectDir string, dependencies map[string]bool, baseRawURL, branch string) error {
	packageJsonPath := filepath.Join(projectDir, "package.json")

	// Fetch remote package.json to get actual versions
	var remoteDeps map[string]interface{}
	var remoteDevDeps map[string]interface{}

	remotePackage, err := fetchRemotePackageJson(baseRawURL, branch)
	if err == nil {
		if deps, ok := remotePackage["dependencies"].(map[string]interface{}); ok {
			remoteDeps = deps
		}
		if devDeps, ok := remotePackage["devDependencies"].(map[string]interface{}); ok {
			remoteDevDeps = devDeps
		}
	}

	var packageData map[string]interface{}

	// Read existing package.json if it exists
	if util.Exists(packageJsonPath) {
		content, err := os.ReadFile(packageJsonPath)
		if err != nil {
			return fmt.Errorf("failed to read package.json: %w", err)
		}

		if err := json.Unmarshal(content, &packageData); err != nil {
			return fmt.Errorf("failed to parse package.json: %w", err)
		}
	} else {
		// Create basic package.json structure
		packageData = map[string]interface{}{
			"name":         "imported-agent-project",
			"version":      "1.0.0",
			"description":  "Imported agent project",
			"main":         "index.js",
			"dependencies": map[string]interface{}{},
		}
	}

	// Ensure dependencies field exists
	deps, ok := packageData["dependencies"].(map[string]interface{})
	if !ok {
		deps = make(map[string]interface{})
		packageData["dependencies"] = deps
	}

	// Ensure devDependencies field exists
	devDeps, ok := packageData["devDependencies"].(map[string]interface{})
	if !ok {
		devDeps = make(map[string]interface{})
		packageData["devDependencies"] = devDeps
	}

	// Process each dependency and look for related dev dependencies

	for dep := range dependencies {
		var versionToUse string

		// Get remote version
		var remoteVersion string
		if remoteDeps != nil {
			if rv, ok := remoteDeps[dep].(string); ok {
				remoteVersion = rv
			}
		}
		if remoteVersion == "" && remoteDevDeps != nil {
			if rv, ok := remoteDevDeps[dep].(string); ok {
				remoteVersion = rv
			}
		}

		// Check if local version exists and conflicts
		if existingVersion, exists := deps[dep].(string); exists {
			if remoteVersion != "" && existingVersion != remoteVersion {
				// Ask user which version to use
				if tui.HasTTY {
					options := []tui.Option{
						{ID: "local", Text: fmt.Sprintf("Keep local version: %s", existingVersion)},
						{ID: "remote", Text: fmt.Sprintf("Use remote version: %s", remoteVersion)},
					}

					choice := tui.Select(logger,
						fmt.Sprintf("Version conflict for %s", dep),
						fmt.Sprintf("Local: %s, Remote: %s", existingVersion, remoteVersion),
						options)

					if choice == "remote" {
						versionToUse = remoteVersion
					} else {
						versionToUse = existingVersion
					}
				} else {
					// No TTY, keep existing version
					versionToUse = existingVersion
				}
			} else {
				// No conflict, keep existing
				versionToUse = existingVersion
			}
		} else {
			// New dependency - use remote version if available, otherwise skip
			if remoteVersion != "" {
				versionToUse = remoteVersion
			} else {
				// Skip adding this dependency if no version is available
				continue
			}
		}

		deps[dep] = versionToUse

		// Look for related dev dependencies (e.g., @types/package-name)
		relatedDevDeps := findRelatedDevDependencies(dep, remoteDeps, remoteDevDeps)
		for devDep, devVersion := range relatedDevDeps {
			// Check if dev dependency already exists locally
			if existingDevVersion, exists := devDeps[devDep].(string); exists {
				if devVersion != existingDevVersion {
					// Ask user which version to use for dev dependency
					if tui.HasTTY {
						options := []tui.Option{
							{ID: "local", Text: fmt.Sprintf("Keep local version: %s", existingDevVersion)},
							{ID: "remote", Text: fmt.Sprintf("Use remote version: %s", devVersion)},
						}

						choice := tui.Select(logger,
							fmt.Sprintf("Dev dependency version conflict for %s", devDep),
							fmt.Sprintf("Local: %s, Remote: %s", existingDevVersion, devVersion),
							options)

						if choice == "remote" {
							devDeps[devDep] = devVersion
						}
					}
					// If no TTY, keep existing version (no action needed)
				}
			} else {
				// New dev dependency, add it
				devDeps[devDep] = devVersion
			}
		}
	}

	// Clean up empty devDependencies section
	if len(devDeps) == 0 {
		delete(packageData, "devDependencies")
	}

	// Write updated package.json
	content, err := json.MarshalIndent(packageData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package.json: %w", err)
	}

	if err := os.WriteFile(packageJsonPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	return nil
}

func updateRequirementsTxt(logger logger.Logger, projectDir string, dependencies map[string]bool, baseRawURL, branch string) error {
	requirementsPath := filepath.Join(projectDir, "requirements.txt")

	// Try to fetch remote requirements.txt to preserve versions
	var remoteDeps map[string]string
	remoteReqURL := baseRawURL + branch + "/requirements.txt"
	if remoteContent, err := fetchFileFromURL(remoteReqURL); err == nil {
		remoteDeps = make(map[string]string)
		lines := strings.Split(string(remoteContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				// Parse package name and version
				parts := strings.FieldsFunc(line, func(r rune) bool {
					return r == '=' || r == '>' || r == '<' || r == '!' || r == '~'
				})
				if len(parts) >= 1 {
					pkg := parts[0]
					remoteDeps[pkg] = line // Store full line with version
				}
			}
		}
	}

	var existingDeps map[string]string

	// Read existing requirements.txt if it exists
	if util.Exists(requirementsPath) {
		content, err := os.ReadFile(requirementsPath)
		if err != nil {
			return fmt.Errorf("failed to read requirements.txt: %w", err)
		}

		existingDeps = make(map[string]string)
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				// Extract package name (before version specifier)
				parts := strings.FieldsFunc(line, func(r rune) bool {
					return r == '=' || r == '>' || r == '<' || r == '!' || r == '~'
				})
				if len(parts) > 0 {
					existingDeps[parts[0]] = line // Store full line with version
				}
			}
		}
	} else {
		existingDeps = make(map[string]string)
	}

	// Process dependencies with version conflict resolution
	var finalDeps []string

	// Add existing dependencies first
	for pkg, line := range existingDeps {
		if dependencies[pkg] {
			// This dependency is needed, check for version conflicts
			if remoteLine, hasRemote := remoteDeps[pkg]; hasRemote && remoteLine != line {
				// Version conflict
				if tui.HasTTY {
					options := []tui.Option{
						{ID: "local", Text: fmt.Sprintf("Keep local: %s", line)},
						{ID: "remote", Text: fmt.Sprintf("Use remote: %s", remoteLine)},
					}

					choice := tui.Select(logger,
						fmt.Sprintf("Version conflict for %s", pkg),
						fmt.Sprintf("Local: %s, Remote: %s", line, remoteLine),
						options)

					if choice == "remote" {
						finalDeps = append(finalDeps, remoteLine)
					} else {
						finalDeps = append(finalDeps, line)
					}
				} else {
					// No TTY, keep existing
					finalDeps = append(finalDeps, line)
				}
			} else {
				// No conflict or no remote version, keep existing
				finalDeps = append(finalDeps, line)
			}
			delete(dependencies, pkg) // Mark as processed
		} else {
			// Not needed, but keep existing dependencies
			finalDeps = append(finalDeps, line)
		}
	}

	// Add new dependencies from remote only if version is available
	for dep := range dependencies {
		if remoteLine, hasRemote := remoteDeps[dep]; hasRemote {
			finalDeps = append(finalDeps, remoteLine)
		}
		// Skip adding dependencies without version information
	}

	sort.Strings(finalDeps)

	// Write updated requirements.txt
	content := strings.Join(finalDeps, "\n")
	if content != "" {
		content += "\n"
	}

	if err := os.WriteFile(requirementsPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write requirements.txt: %w", err)
	}

	return nil
}

// AvailableAgent represents an agent available for import
type AvailableAgent struct {
	Name        string
	Description string
	URL         string
	Author      string
	Tags        []string
}

// browseAvailableAgents shows a list of available agents for import
func browseAvailableAgents(logger logger.Logger) string {
	// For now, we'll have a curated list of popular agents
	// In the future, this could be fetched from an API or registry
	availableAgents := []AvailableAgent{
		{
			Name:        "Postman Agent",
			Description: "A agent that updates your Postman collections and environments",
			URL:         "https://github.com/agentuity/postman-agent",
			Author:      "Agentuity",
			Tags:        []string{"chat", "openai", "gpt"},
		},
		{
			Name:        "File Processing Agent",
			Description: "Agent for processing and analyzing various file types",
			URL:         "https://github.com/agentuity/file-processor-agent",
			Author:      "Agentuity",
			Tags:        []string{"files", "processing", "analysis"},
		},
		{
			Name:        "Web Scraper Agent",
			Description: "Extract data from websites with intelligent scraping",
			URL:         "https://github.com/agentuity/web-scraper-agent",
			Author:      "Agentuity",
			Tags:        []string{"scraping", "web", "data"},
		},
		{
			Name:        "Email Assistant Agent",
			Description: "Automated email processing and response generation",
			URL:         "https://github.com/agentuity/email-assistant-agent",
			Author:      "Agentuity",
			Tags:        []string{"email", "assistant", "automation"},
		},
		{
			Name:        "Code Review Agent",
			Description: "Automated code review and suggestions for improvements",
			URL:         "https://github.com/agentuity/code-review-agent",
			Author:      "Agentuity",
			Tags:        []string{"code", "review", "development"},
		},
		{
			Name:        "Database Query Agent",
			Description: "Natural language to SQL query conversion and execution",
			URL:         "https://github.com/agentuity/database-query-agent",
			Author:      "Agentuity",
			Tags:        []string{"database", "sql", "queries"},
		},
		{
			Name:        "Document Summarizer Agent",
			Description: "Intelligent document analysis and summarization",
			URL:         "https://github.com/agentuity/document-summarizer-agent",
			Author:      "Agentuity",
			Tags:        []string{"documents", "summarization", "analysis"},
		},
		{
			Name:        "Social Media Agent",
			Description: "Automated social media content creation and management",
			URL:         "https://github.com/agentuity/social-media-agent",
			Author:      "Agentuity",
			Tags:        []string{"social", "content", "automation"},
		},
	}

	if !tui.HasTTY {
		// No TTY, can't show interactive selection
		fmt.Printf("Available agents:\n")
		for i, agent := range availableAgents {
			fmt.Printf("%d. %s - %s (%s)\n", i+1, agent.Name, agent.Description, agent.URL)
		}
		logger.Fatal("Cannot show interactive agent selection without TTY. Please provide a GitHub URL directly.")
	}

	// Create options for the multiselect
	var options []tui.Option
	for i, agent := range availableAgents {
		tagsStr := ""
		if len(agent.Tags) > 0 {
			tagsStr = fmt.Sprintf(" [%s]", strings.Join(agent.Tags, ", "))
		}

		options = append(options, tui.Option{
			ID:   fmt.Sprintf("%d", i),
			Text: fmt.Sprintf("%s - %s%s", agent.Name, agent.Description, tagsStr),
		})
	}

	fmt.Printf("\n%s\n", tui.Bold("Available Agents:"))

	selectedID := tui.Select(logger,
		"Select an agent to import:",
		"Use arrow keys to navigate, Enter to select, or Ctrl+C to cancel",
		options)

	if selectedID == "" {
		return ""
	}

	// Parse the selected index
	selectedIndex := 0
	if idx, err := fmt.Sscanf(selectedID, "%d", &selectedIndex); err != nil || idx != 1 {
		return ""
	}

	if selectedIndex < 0 || selectedIndex >= len(availableAgents) {
		return ""
	}

	selectedAgent := availableAgents[selectedIndex]
	fmt.Printf("\n%s Selected: %s\n", tui.Bold("✓"), selectedAgent.Name)
	fmt.Printf("  %s %s\n", tui.Bold("📝"), selectedAgent.Description)
	fmt.Printf("  %s %s\n", tui.Bold("👤"), selectedAgent.Author)
	fmt.Printf("  %s %s\n", tui.Bold("🔗"), selectedAgent.URL)

	return selectedAgent.URL
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var agentDeleteCmd = &cobra.Command{
	Use:     "delete [id]",
	Short:   "Delete one or more Agents",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"rm", "del"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		theproject := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		if !tui.HasTTY && len(args) == 0 {
			logger.Fatal("No TTY detected, please specify an Agent id from the command line")
		}

		keys, state := reconcileAgentList(logger, cmd, apiUrl, theproject.Token, theproject)
		var selected []string

		if len(args) > 0 {
			id := args[0]
			if _, ok := state[id]; ok {
				selected = append(selected, id)
			} else {
				logger.Fatal("Agent with id %s not found", id)
			}
		} else {
			var options []tui.Option
			for _, key := range keys {
				agent := state[key]
				if agent.FoundRemote {
					options = append(options, tui.Option{
						ID:   agent.Agent.ID,
						Text: tui.PadRight(agent.Agent.Name, 20, " ") + tui.Muted(agent.Agent.ID),
					})
				}
			}

			selected = tui.MultiSelect(logger, "Select one or more Agents to delete from Agentuity Cloud", "Toggle selection by pressing the spacebar\nPress enter to confirm\n", options)

			if len(selected) == 0 {
				tui.ShowWarning("no Agents selected")
				return
			}
		}

		var deleted []string
		var maybedelete []string

		action := func() {
			var err error
			deleted, err = agent.DeleteAgents(context.Background(), logger, apiUrl, theproject.Token, theproject.Project.ProjectId, selected)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete agents")).ShowErrorAndExit()
			}
			for _, key := range keys {
				agent := state[key]
				if slices.Contains(deleted, agent.Agent.ID) && util.Exists(agent.Filename) {
					maybedelete = append(maybedelete, agent.Filename)
				}
			}
			var agents []project.AgentConfig
			for _, agent := range theproject.Project.Agents {
				if !slice.Contains(deleted, agent.ID) {
					agents = append(agents, agent)
				}
			}
			theproject.Project.Agents = agents
			if err := theproject.Project.Save(theproject.Dir); err != nil {
				errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("saving project after agent delete")).ShowErrorAndExit()
			}
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force && !tui.HasTTY {
			logger.Fatal("No TTY detected, please --force to delete the selected Agents and pass in the Agent id from the command line")
		}

		if !force && !tui.Ask(logger, "Are you sure you want to delete the selected Agents from Agentuity Cloud?", true) {
			tui.ShowWarning("cancelled")
			return
		}

		tui.ShowSpinner("Deleting Agents ...", action)

		var filedeletes []string

		if len(maybedelete) > 0 {
			if !force {
				filetext := util.Pluralize(len(maybedelete), "source file", "source files")
				var opts []tui.Option
				for _, f := range maybedelete {
					rel, _ := filepath.Rel(theproject.Dir, f)
					opts = append(opts, tui.Option{
						ID:       f,
						Text:     rel,
						Selected: true,
					})
				}
				filedeletes = tui.MultiSelect(logger, fmt.Sprintf("Would you like to delete the %s?", filetext), "Press spacebar to toggle file selection. Press enter to continue.", opts)
			} else {
				filedeletes = maybedelete
			}
		}

		if len(filedeletes) > 0 {
			ad := filepath.Join(theproject.Dir, ".agentuity", "backup")
			if !util.Exists(ad) {
				os.MkdirAll(ad, 0755)
			}
			for _, f := range filedeletes {
				fd := filepath.Dir(f)
				util.CopyDir(fd, filepath.Join(ad, filepath.Base(fd))) // make a backup
				os.Remove(f)
				files, _ := util.ListDir(fd)
				if len(files) == 0 {
					os.Remove(fd)
				}
			}
			tui.ShowSuccess("A backup was made temporarily in %s", ad)
		}

		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "Agent", "Agents"))
	},
}

func getAgentAuthType(logger logger.Logger, authType string) string {
	if authType != "" {
		switch authType {
		case "project", "bearer", "none":
			return authType
		default:
		}
	}
	auth := tui.Select(logger, "Select your Agent's webhook authentication method", "Do you want to secure the webhook or make it publicly available?", []tui.Option{
		{Text: tui.PadRight("API Key", 20, " ") + tui.Muted("Bearer Token (will be generated for you)"), ID: "bearer"},
		{Text: tui.PadRight("Project API Key", 20, " ") + tui.Muted("The Project Key attched to your project"), ID: "project"},
		{Text: tui.PadRight("None", 20, " ") + tui.Muted("No Authentication Required"), ID: "none"},
	})
	return auth
}

func getAgentInfoFlow(logger logger.Logger, remoteAgents []agent.Agent, name string, description string, authType string) (string, string, string) {
	if name == "" {
		if !tui.HasTTY {
			logger.Fatal("No TTY detected, please specify an Agent name from the command line")
		}
		var prompt, help string
		if len(remoteAgents) > 0 {
			prompt = "What should we name the Agent?"
			help = "The name of the Agent must be unique within the project"
		} else {
			prompt = "What should we name the initial Agent?"
			help = "The name can be changed at any time and helps identify the Agent"
		}
		name = tui.InputWithValidation(logger, prompt, help, 255, func(name string) error {
			if name == "" {
				return fmt.Errorf("Agent name cannot be empty")
			}
			for _, agent := range remoteAgents {
				if strings.EqualFold(agent.Name, name) {
					return fmt.Errorf("Agent already exists with the name: %s", name)
				}
			}
			return nil
		})
	}

	if description == "" {
		description = tui.Input(logger, "How should we describe what the "+name+" Agent does?", "The description of the Agent is optional but helpful for understanding the role of the Agent")
	}

	if authType == "" && !tui.HasTTY {
		logger.Fatal("No TTY detected, please specify an Agent authentication type from the command line")
	}

	auth := getAgentAuthType(logger, authType)

	return name, description, auth
}

var agentCreateCmd = &cobra.Command{
	Use:     "create [name] [description] [auth_type]",
	Short:   "Create a new Agent",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		theproject := project.EnsureProject(ctx, cmd)
		apikey := theproject.Token
		apiUrl, _, _ := util.GetURLs(logger)

		var remoteAgents []agent.Agent

		if theproject.NewProject {
			var projectId string
			if theproject.Project != nil {
				projectId = theproject.Project.ProjectId
			}
			ShowNewProjectImport(ctx, logger, cmd, apiUrl, apikey, projectId, theproject.Project, theproject.Dir, false)
		} else {
			initScreenWithLogo()
		}

		checkForUpgrade(ctx, logger, false)

		loadTemplates(ctx, cmd)

		var err error
		remoteAgents, err = getAgentList(logger, apiUrl, apikey, theproject)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
		}

		var name string
		var description string
		var authType string

		if len(args) > 0 {
			name = args[0]
		}

		if len(args) > 1 {
			description = args[1]
		}

		if len(args) > 2 {
			authType = args[2]
		}

		force, _ := cmd.Flags().GetBool("force")

		// if we have a force flag and a name passed in, delete the existing agent if found
		if force && name != "" {
			for _, a := range remoteAgents {
				if strings.EqualFold(a.Name, name) {
					if _, err := agent.DeleteAgents(ctx, logger, apiUrl, apikey, theproject.Project.ProjectId, []string{a.ID}); err != nil {
						errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete existing Agent")).ShowErrorAndExit()
					}
					for i, ea := range theproject.Project.Agents {
						if ea.ID == a.ID {
							theproject.Project.Agents = append(theproject.Project.Agents[:i], theproject.Project.Agents[i+1:]...)
							break
						}
					}
					break
				}
			}
		}

		name, description, authType = getAgentInfoFlow(logger, remoteAgents, name, description, authType)

		action := func() {
			agentID, err := agent.CreateAgent(ctx, logger, apiUrl, apikey, theproject.Project.ProjectId, name, description, authType)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to create Agent")).ShowErrorAndExit()
			}

			tmpdir, _, err := getConfigTemplateDir(cmd)
			if err != nil {
				errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
			}

			rules, err := templates.LoadTemplateRuleForIdentifier(tmpdir, theproject.Project.Bundler.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
			}

			template, err := templates.LoadTemplateForRuntime(context.Background(), tmpdir, theproject.Project.Bundler.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
			}

			if err := rules.NewAgent(templates.TemplateContext{
				Logger:           logger,
				AgentName:        name,
				Name:             name,
				Description:      description,
				AgentDescription: description,
				ProjectDir:       theproject.Dir,
				TemplateDir:      tmpdir,
				Template:         template,
				AgentuityCommand: getAgentuityCommand(),
			}); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithAttributes(map[string]any{"name": name})).ShowErrorAndExit()
			}

			theproject.Project.Agents = append(theproject.Project.Agents, project.AgentConfig{
				ID:          agentID,
				Name:        name,
				Description: description,
			})

			if err := theproject.Project.Save(theproject.Dir); err != nil {
				errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("Failed to save project to disk")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("Creating Agent ...", action)

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(theproject.Project.Agents[len(theproject.Project.Agents)-1])
		} else {
			tui.ShowSuccess("Agent created successfully")
		}

	},
}

type agentListState struct {
	Agent       *agent.Agent `json:"agent"`
	Filename    string       `json:"filename"`
	FoundLocal  bool         `json:"foundLocal"`
	FoundRemote bool         `json:"foundRemote"`
	Rename      bool         `json:"rename"`
	RenameFrom  string       `json:"renameFrom"`
}

func getAgentList(logger logger.Logger, apiUrl string, apikey string, project project.ProjectContext) ([]agent.Agent, error) {
	var remoteAgents []agent.Agent
	var err error
	action := func() {
		remoteAgents, err = agent.ListAgents(context.Background(), logger, apiUrl, apikey, project.Project.ProjectId)
	}
	tui.ShowSpinner("Fetching Agents ...", action)
	return remoteAgents, err
}

func normalAgentName(name string, isPython bool) string {
	return util.SafeProjectFilename(strings.ToLower(name), isPython)
}

func reconcileAgentList(logger logger.Logger, cmd *cobra.Command, apiUrl string, apikey string, theproject project.ProjectContext) ([]string, map[string]agentListState) {
	remoteAgents, err := getAgentList(logger, apiUrl, apikey, theproject)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
	}

	tmpdir, _, err := getConfigTemplateDir(cmd)
	if err != nil {
		errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
	}

	rules, err := templates.LoadTemplateRuleForIdentifier(tmpdir, theproject.Project.Bundler.Identifier)
	if err != nil {
		errsystem.New(errsystem.ErrInvalidConfiguration, err,
			errsystem.WithContextMessage("Failed loading template rule"),
			errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
	}

	// make a map of the agents in the agentuity config file
	fileAgents := make(map[string]project.AgentConfig)
	fileAgentsByID := make(map[string]project.AgentConfig)
	for _, agent := range theproject.Project.Agents {
		key := normalAgentName(agent.Name, theproject.Project.IsPython())
		if existing, ok := fileAgents[key]; ok {
			logger.Warn(
				"agent name collision: %q and %q normalize to %q; keeping first entry",
				existing.Name, agent.Name, key,
			)
			continue
		}
		fileAgents[key] = agent
		fileAgentsByID[agent.ID] = agent
	}

	agentFilename := rules.Filename
	agentSrcDir := filepath.Join(theproject.Dir, theproject.Project.Bundler.AgentConfig.Dir)

	// perform the reconcilation
	state := make(map[string]agentListState)
	for _, agent := range remoteAgents {
		normalizedName := normalAgentName(agent.Name, theproject.Project.IsPython())
		filename1 := filepath.Join(agentSrcDir, normalizedName, agentFilename)
		filename2 := filepath.Join(agentSrcDir, agent.Name, agentFilename)
		if util.Exists(filename1) {
			state[normalizedName] = agentListState{
				Agent:       &agent,
				Filename:    filename1,
				FoundLocal:  true,
				FoundRemote: true,
			}
		} else if util.Exists(filename2) {
			state[normalizedName] = agentListState{
				Agent:       &agent,
				Filename:    filename2,
				FoundLocal:  true,
				FoundRemote: true,
			}
		}
	}
	localAgents, err := util.ListDir(agentSrcDir)
	if err != nil {
		errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to list agent source directory")).ShowErrorAndExit()
	}
	for _, filename := range localAgents {
		agentName := filepath.Base(filepath.Dir(filename))
		normalizedName := normalAgentName(agentName, theproject.Project.IsPython())
		key := normalizedName
		// var found bool
		// for _, agent := range remoteAgents {
		// 	if localAgent, ok := fileAgentsByID[agent.ID]; ok {
		// 		if localAgent.Name == agentName {
		// 			oldkey := normalAgentName(agent.Name)
		// 			agent.Name = localAgent.Name
		// 			state[key] = agentListState{
		// 				Agent:       &agent,
		// 				Filename:    filename,
		// 				FoundLocal:  true,
		// 				FoundRemote: true,
		// 				Rename:      true,
		// 				RenameFrom:  oldkey,
		// 			}
		// 			delete(state, oldkey)
		// 			found = true
		// 			break
		// 		}
		// 	}
		// }
		// if found {
		// 	continue
		// }
		if filepath.Base(filename) == agentFilename {
			if found, ok := state[key]; ok {
				state[key] = agentListState{
					Agent:       found.Agent,
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: true,
				}
				continue
			}
			if a, ok := fileAgents[key]; ok {
				state[key] = agentListState{
					Agent:       &agent.Agent{Name: a.Name, ID: a.ID, Description: a.Description},
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: true,
				}
			} else {
				state[key] = agentListState{
					Agent:       &agent.Agent{Name: agentName},
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: false,
				}
			}
		}
	}

	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys, state
}

var wrappedPipe = "\n│"

func buildAgentTree(keys []string, state map[string]agentListState, project project.ProjectContext) (*tree.Tree, int, int, error) {
	agentSrcDir := filepath.Join(project.Dir, project.Project.Bundler.AgentConfig.Dir)
	var root *tree.Tree
	var files *tree.Tree
	cwd, err := os.Getwd()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get current working directory: %w", err)
	}
	if filepath.Join(cwd, project.Project.Bundler.AgentConfig.Dir) == agentSrcDir {
		files = tree.Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
		root = files
	} else {
		srcdir := tree.New().Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
		root = tree.New().Root(tui.Muted(project.Dir) + wrappedPipe).Child(srcdir)
		files = srcdir
	}

	var localIssues, remoteIssues int

	for _, k := range keys {
		st := state[k]
		label := tui.PadRight(tui.Bold(st.Agent.Name), 20, " ")
		var sublabels []any
		if st.FoundLocal && st.FoundRemote {
			sublabels = append(sublabels, tui.Muted("ID: ")+tui.Secondary(st.Agent.ID))
			desc := st.Agent.Description
			if desc == "" {
				desc = emptyProjectDescription
			}
			sublabels = append(sublabels, tui.Muted("Description: ")+tui.Secondary(desc))
			if st.Rename {
				label += " " + tui.Warning("⚠ Renaming from "+st.RenameFrom)
			}
		} else if st.FoundLocal {
			sublabels = append(sublabels, tui.Warning("⚠ Agent found local but not remotely"))
			localIssues++
		} else if st.FoundRemote {
			sublabels = append(sublabels, tui.Muted("ID: ")+tui.Secondary(st.Agent.ID))
			sublabels = append(sublabels, tui.Warning("⚠ Agent found remotely but not locally"))
			remoteIssues++
		}
		if len(sublabels) > 0 {
			sublabels[len(sublabels)-1] = sublabels[len(sublabels)-1].(string) + "\n"
		}
		agentTree := tree.New().Root(label).Child(sublabels...)
		files.Child(agentTree)
	}

	return root, localIssues, remoteIssues, nil
}

func showAgentWarnings(remoteIssues int, localIssues int, deploying bool) bool {
	issues := remoteIssues + localIssues
	if issues > 0 {
		var msg string
		var title string
		if issues > 1 {
			title = "Issues"
		} else {
			title = "Issue"
		}
		localFmt := util.Pluralize(localIssues, "local agent", "local agents")
		remoteFmt := util.Pluralize(remoteIssues, "remote agent", "remote agents")
		var prefix string
		if !deploying {
			prefix = "When you deploy your project, the"
		} else {
			prefix = "The"
		}
		switch {
		case localIssues > 0 && remoteIssues > 0:
			msg = fmt.Sprintf("%s %s will be deployed and the %s will be undeployed.", prefix, localFmt, remoteFmt)
		case localIssues > 0:
			msg = fmt.Sprintf("%s %s will be deployed to the cloud and the ID will be saved.", prefix, localFmt)
		case remoteIssues > 0:
			msg = fmt.Sprintf("%s %s will be undeployed from the cloud and the ID will be removed from your project locally.", prefix, remoteFmt)
		}
		body := fmt.Sprintf("Detected %s in your project. %s\n\n", util.Pluralize(issues, "discrepancy", "discrepancies"), msg) + tui.Muted("$ ") + tui.Command("deploy")
		tui.ShowBanner(tui.Warning(fmt.Sprintf("⚠ Agent %s Detected", title)), body, false)
		if deploying {
			tui.WaitForAnyKey()
		}
		return true
	}
	return false
}

var agentListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all Agents in the project",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		project := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, cmd, apiUrl, project.Token, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(state)
		} else {
			root, localIssues, remoteIssues, err := buildAgentTree(keys, state, project)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to build agent tree")).ShowErrorAndExit()
			}
			fmt.Println(root)
			if showAgentWarnings(remoteIssues, localIssues, false) {
				os.Exit(1)
			}
		}

	},
}

var agentGetApiKeyCmd = &cobra.Command{
	Use:   "apikey [agent_name]",
	Short: "Get the API key for an agent",
	Long: `Get the API key for an agent by name or ID.

Arguments:
  [agent_name]  The name or ID of the agent to get the API key for

If no agent name is provided, you will be prompted to select an agent.

Examples:
  agentuity agent apikey "My Agent"
  agentuity agent apikey agent_ID
  agentuity agent apikey`,
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"key"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		project := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, cmd, apiUrl, project.Token, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		var agentID string
		var theagent *agentListState
		if len(args) > 0 {
			agentName := args[0]
			for _, v := range state {
				if v.Agent.ID == agentName || v.Agent.Name == agentName {
					theagent = &v
					agentID = v.Agent.ID
					break
				}
			}
		}
		if theagent == nil {
			if len(state) == 1 {
				for _, v := range state {
					theagent = &v
					agentID = v.Agent.ID
					break
				}
			} else {
				if !tui.HasTTY {
					logger.Fatal("No TTY detected, please specify an Agent name or id")
				}
				var options []tui.Option
				for _, v := range keys {
					options = append(options, tui.Option{
						ID:   state[v].Agent.ID,
						Text: tui.PadRight(state[v].Agent.Name, 20, " ") + tui.Muted(state[v].Agent.ID),
					})
				}
				selected := tui.Select(logger, "Select an Agent", "Select the Agent you want to get the API key for", options)
				for _, v := range state {
					if v.Agent.ID == selected {
						theagent = &v
						break
					}
				}
			}
		}
		if theagent == nil {
			tui.ShowWarning("Agent not found")
			return
		}
		apikey, err := agent.GetApiKey(context.Background(), logger, apiUrl, project.Token, theagent.Agent.ID, theagent.Agent.Types[0])
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent API key")).ShowErrorAndExit()
		}
		if !tui.HasTTY {
			if apikey != "" {
				fmt.Print(apikey)
				return
			}
		}
		if apikey != "" {
			fmt.Println()
			tui.ShowLock("Agent %s API key: %s", theagent.Agent.Name, apikey)
			tip := fmt.Sprintf(`$(agentuity agent apikey %s)`, agentID)
			tui.ShowBanner("Developer Pro Tip", tui.Paragraph("Fetch your Agent's API key into a shell command dynamically:", tip), false)
			return
		} else {
			tui.ShowWarning("No API key found for Agent %s (%s)", theagent.Agent.Name, theagent.Agent.ID)
		}
		os.Exit(1) // no key
	},
}

var agentImportCmd = &cobra.Command{
	Use:   "import [github-url]",
	Short: "Import an agent from a GitHub repository or browse available agents",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)

		var githubURL string

		if len(args) == 0 {
			// No URL provided, show agent browser
			selectedURL := browseAvailableAgents(logger)
			if selectedURL == "" {
				logger.Info("No agent selected")
				return
			}
			githubURL = selectedURL
		} else {
			githubURL = args[0]

			// Basic URL validation
			if !strings.HasPrefix(githubURL, "https://github.com/") && !strings.HasPrefix(githubURL, "http://github.com/") {
				logger.Fatal("Invalid GitHub URL. Please provide a valid GitHub repository URL (e.g., https://github.com/user/repo)")
			}
		}

		// Resolve project directory (respects --dir flag)
		projectDir := project.ResolveProjectDir(logger, cmd, false)

		var agentConfig *project.Project

		tui.ShowSpinner(fmt.Sprintf("Importing agent from %s...", githubURL), func() {
			// Convert GitHub URL to raw URL for agentuity.yaml
			rawURL := strings.Replace(githubURL, "github.com", "raw.githubusercontent.com", 1)
			rawURL = strings.Replace(rawURL, "/blob/", "/", 1)
			if !strings.HasSuffix(rawURL, "/") {
				rawURL += "/"
			}
			yamlURL := rawURL + "main/agentuity.yaml"

			// Try main branch first, then master
			yamlContent, err := fetchFileFromURL(yamlURL)
			if err != nil {
				yamlURL = rawURL + "master/agentuity.yaml"
				yamlContent, err = fetchFileFromURL(yamlURL)
				if err != nil {
					logger.Fatal("Failed to fetch agentuity.yaml from %s (tried main and master branches): %v", githubURL, err)
				}
			}

			// Parse the YAML content
			agentProject := project.NewProject()
			if err := yaml.Unmarshal(yamlContent, agentProject); err != nil {
				logger.Fatal("Failed to parse agentuity.yaml: %v", err)
			}

			if agentProject.ProjectId == "" {
				logger.Fatal("Invalid agentuity.yaml: missing project_id")
			}

			agentConfig = agentProject
		})

		if agentConfig == nil || len(agentConfig.Agents) == 0 {
			tui.ShowWarning("No agents found in the project configuration")
			return
		}

		// Create options for agent selection
		var options []tui.Option
		for _, agent := range agentConfig.Agents {
			desc := agent.Description
			if desc == "" {
				desc = emptyProjectDescription
			}
			options = append(options, tui.Option{
				ID:       agent.ID,
				Text:     tui.PadRight(agent.Name, 25, " ") + tui.Muted(desc),
				Selected: false, // Start with none selected
			})
		}

		// Let user select one or more agents
		selectedAgentIDs := tui.MultiSelect(logger,
			fmt.Sprintf("Select agents from %s (%d found)", agentConfig.Name, len(agentConfig.Agents)),
			"Toggle selection by pressing spacebar\nPress enter to analyze selected agents\n",
			options)

		if len(selectedAgentIDs) == 0 {
			tui.ShowWarning("No agents selected")
			return
		}

		// Find the selected agents
		var selectedAgents []project.AgentConfig
		for _, agent := range agentConfig.Agents {
			for _, selectedID := range selectedAgentIDs {
				if agent.ID == selectedID {
					selectedAgents = append(selectedAgents, agent)
					break
				}
			}
		}

		// Build the base URL for raw files
		baseRawURL := strings.Replace(githubURL, "github.com", "raw.githubusercontent.com", 1)
		baseRawURL = strings.Replace(baseRawURL, "/blob/", "/", 1)
		if !strings.HasSuffix(baseRawURL, "/") {
			baseRawURL += "/"
		}

		// Determine branch (already determined from previous fetch)
		branch := "main"
		if strings.Contains(projectDir, "master") {
			branch = "master"
		}

		// Analyze selected agents
		analyzeSelectedAgents(logger, selectedAgents, agentConfig, baseRawURL, branch, projectDir)
	},
}

var agentTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test an agent",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		theproject := project.EnsureProject(ctx, cmd)

		agentID, _ := cmd.Flags().GetString("agent-id")
		payload, _ := cmd.Flags().GetString("payload")
		local, _ := cmd.Flags().GetBool("local")
		contentType, _ := cmd.Flags().GetString("content-type")
		tag, _ := cmd.Flags().GetString("tag")
		var selectedAgent *agent.Agent
		if agentID == "" {
			keys, state := reconcileAgentList(logger, cmd, theproject.APIURL, theproject.Token, theproject)
			if len(keys) == 0 {
				tui.ShowWarning("no Agents found")
				tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
				return
			}
			var options []tui.Option
			for _, v := range keys {
				options = append(options, tui.Option{
					ID:   v,
					Text: tui.PadRight(state[v].Agent.Name, 20, " ") + tui.Muted(state[v].Agent.ID),
				})
			}
			selected := tui.Select(logger, "Select an agent", "Select the agent you want to test", options)
			selectedAgent = state[selected].Agent
			agentID = selectedAgent.ID
		}

		if len(selectedAgent.Types) == 0 {
			// this should never ever happen
			tui.ShowError("Agent %s has no running types (webhook or api)", selectedAgent.Name)
			os.Exit(1)
		}
		var route string
		if len(selectedAgent.Types) > 1 {
			options := []tui.Option{}
			for _, route := range selectedAgent.Types {
				options = append(options, tui.Option{
					ID:   route,
					Text: route,
				})
			}
			route = tui.Select(logger, "Select an running type", "Select the running type you want to use", options)
		} else {
			route = selectedAgent.Types[0]
		}

		if payload == "" {
			payload = tui.Input(logger, "Enter the payload to send to the agent", "{\"hello\": \"world\"}")
		}

		apikey, err := agent.GetApiKey(context.Background(), logger, theproject.APIURL, theproject.Token, agentID, route)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent API key")).ShowErrorAndExit()
		}
		endpoint := fmt.Sprintf("%s/%s/%s", theproject.TransportURL, route, agentID)
		if local {
			port, _ := dev.FindAvailablePort(theproject, 0)
			endpoint = fmt.Sprintf("http://127.0.0.1:%d/%s", port, agentID)
		}

		if tag != "" {
			endpoint = fmt.Sprintf("%s/%s", endpoint, tag)
		}

		// use http package to send a POST request to the agent
		req, err := http.NewRequest("POST", endpoint, strings.NewReader(payload))
		if err != nil {
			logger.Fatal("Failed to create request: %s", err)
		}
		if contentType == "" {
			// check if payload is json
			if json.Valid([]byte(payload)) {
				req.Header.Set("Content-Type", "application/json")
			} else {
				req.Header.Set("Content-Type", "text/plain")
			}
		} else {
			req.Header.Set("Content-Type", contentType)
		}
		if apikey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apikey))
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Fatal("Failed to send request: %s", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Fatal("Failed to read response: %s", err)
		}
		var jsonBody map[string]interface{}
		if json.Unmarshal(body, &jsonBody) == nil {
			stringified, _ := json.MarshalIndent(jsonBody, "", "  ")
			tui.ShowSuccess("Agent Test: %s", tui.Paragraph(tui.Bold(string(stringified))))
		} else {
			tui.ShowSuccess("Agent Test: %s", tui.Paragraph(tui.Bold(string(body))))
		}
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	agentCmd.AddCommand(agentGetApiKeyCmd)
	agentCmd.AddCommand(agentImportCmd)

	agentTestCmd.Flags().String("agent-id", "", "The ID of the agent to test")
	agentTestCmd.Flags().String("payload", "", "The payload to send to the agent")
	agentTestCmd.Flags().Bool("local", false, "Enable local testing")
	agentTestCmd.Flags().String("content-type", "", "The content type to use for the request, will try to detect if not provided")
	agentTestCmd.Flags().String("tag", "", "The tag to use for the deployment")
	agentCmd.AddCommand(agentTestCmd)

	for _, cmd := range []*cobra.Command{agentListCmd, agentCreateCmd, agentDeleteCmd, agentGetApiKeyCmd, agentTestCmd, agentImportCmd} {
		cmd.Flags().StringP("dir", "d", "", "The project directory")
		cmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")
	}
	for _, cmd := range []*cobra.Command{agentListCmd, agentCreateCmd} {
		cmd.Flags().String("format", "text", "The format to use for the output. Can be either 'text' or 'json'")
	}
	agentListCmd.Flags().String("org-id", "", "The organization to create the project in on import")
	for _, cmd := range []*cobra.Command{agentCreateCmd, agentDeleteCmd} {
		cmd.Flags().Bool("force", false, "Force the creation of the agent even if it already exists")
	}

}
