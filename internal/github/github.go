package github

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

// allowedHiddenFiles lists hidden files/directories that should be preserved during extraction
var allowedHiddenFiles = []string{
	".gitignore", ".env", ".env.example", ".env.local", ".env.production",
	".github", ".vscode", ".eslintrc", ".prettierrc", ".editorconfig", ".cursor", ".windsurf",
	".npmrc", ".nvmrc", ".node-version", ".dockerignore", ".gitattributes",
}

// RepoInfo contains information about a GitHub repository
type RepoInfo struct {
	Username string
	Name     string
	Branch   string
	FilePath string
}

// GitHubAPIRepo represents the GitHub API response for repository information
type GitHubAPIRepo struct {
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	FullName      string `json:"full_name"`
}

// IsURLValid checks if a URL is accessible and returns a 200 status code
func IsURLValid(ctx context.Context, targetURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", targetURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", util.UserAgent())

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetRepoInfo extracts repository information from a GitHub URL
// Supports various GitHub URL formats:
// - https://github.com/username/repo
// - https://github.com/username/repo/
// - https://github.com/username/repo/tree/branch
// - https://github.com/username/repo/tree/branch/path/to/example
func GetRepoInfo(ctx context.Context, logger logger.Logger, repoURL *url.URL, examplePath string) (*RepoInfo, error) {
	if repoURL.Host != "github.com" {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("only GitHub URLs are supported, got: %s", repoURL.Host))
	}

	// Split the path and remove empty elements
	pathParts := strings.Split(strings.Trim(repoURL.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("invalid GitHub URL format, expected at least username/repo"))
	}

	username := pathParts[0]
	name := pathParts[1]

	// Validate username and repository name
	if username == "" || name == "" {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("username and repository name cannot be empty"))
	}

	// Handle different URL formats
	switch {
	case len(pathParts) == 2:
		// Format: github.com/username/repo
		return handleBasicRepoURL(ctx, logger, username, name, examplePath)

	case len(pathParts) == 3 && pathParts[2] == "":
		// Format: github.com/username/repo/ (trailing slash)
		return handleBasicRepoURL(ctx, logger, username, name, examplePath)

	case len(pathParts) >= 4 && pathParts[2] == "tree":
		// Format: github.com/username/repo/tree/branch[/path/to/example]
		return handleTreeURL(username, name, pathParts[3:], examplePath)

	default:
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("unsupported GitHub URL format"))
	}
}

// handleBasicRepoURL handles URLs without branch specification
func handleBasicRepoURL(ctx context.Context, logger logger.Logger, username, name, examplePath string) (*RepoInfo, error) {
	// Get default branch from GitHub API
	defaultBranch, err := getDefaultBranch(ctx, logger, username, name)
	if err != nil {
		return nil, err
	}

	return &RepoInfo{
		Username: username,
		Name:     name,
		Branch:   defaultBranch,
		FilePath: cleanFilePath(examplePath),
	}, nil
}

// handleTreeURL handles URLs with tree/branch specification
func handleTreeURL(username, name string, treeParts []string, examplePath string) (*RepoInfo, error) {
	if len(treeParts) == 0 {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("branch name is required after 'tree'"))
	}

	branch := treeParts[0]
	var filePath string

	if examplePath != "" {
		// Use provided example path
		filePath = cleanFilePath(examplePath)
	} else if len(treeParts) > 1 {
		// Use path from URL
		filePath = strings.Join(treeParts[1:], "/")
	}

	return &RepoInfo{
		Username: username,
		Name:     name,
		Branch:   branch,
		FilePath: cleanFilePath(filePath),
	}, nil
}

// getDefaultBranch fetches the default branch from GitHub API
func getDefaultBranch(ctx context.Context, logger logger.Logger, username, name string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", username, name)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", errsystem.New(errsystem.ErrGithubApiRequest,
			fmt.Errorf("failed to create API request: %w", err))
	}

	req.Header.Set("User-Agent", util.UserAgent())
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errsystem.New(errsystem.ErrGithubApiRequest,
			fmt.Errorf("failed to fetch repository info: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return "", errsystem.New(errsystem.ErrGithubRepositoryNotFoundOrNotAccessible,
				fmt.Errorf("repository %s/%s not found or not accessible", username, name),
				errsystem.WithUserMessage("The repository '%s/%s' was not found. Please check the repository name and ensure it's publicly accessible.", username, name))
		case http.StatusForbidden:
			return "", errsystem.New(errsystem.ErrGithubRepositoryNotFoundOrNotAccessible,
				fmt.Errorf("repository %s/%s is private or access denied", username, name),
				errsystem.WithUserMessage("Access to repository '%s/%s' is forbidden. The repository may be private or you may not have permission to access it.", username, name))
		default:
			return "", errsystem.New(errsystem.ErrGithubApiRequest,
				fmt.Errorf("GitHub API returned status %d", resp.StatusCode))
		}
	}

	var repoInfo GitHubAPIRepo
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return "", errsystem.New(errsystem.ErrGithubApiRequest,
			fmt.Errorf("failed to decode GitHub API response: %w", err))
	}

	if repoInfo.DefaultBranch == "" {
		logger.Warn("No default branch found, using 'main' as fallback")
		return "main", nil
	}

	return repoInfo.DefaultBranch, nil
}

// HasRepo checks if a repository exists and is accessible
func HasRepo(ctx context.Context, repoInfo *RepoInfo) (bool, error) {
	if repoInfo == nil {
		return false, fmt.Errorf("repoInfo cannot be nil")
	}

	// Check if repository exists by trying to access its contents
	contentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents", repoInfo.Username, repoInfo.Name)

	// Add file path if specified
	if repoInfo.FilePath != "" {
		contentsURL = fmt.Sprintf("%s/%s", contentsURL, repoInfo.FilePath)
	}

	// Add branch reference
	contentsURL = fmt.Sprintf("%s?ref=%s", contentsURL, repoInfo.Branch)

	return IsURLValid(ctx, contentsURL)
}

// ValidateAgentuityProject checks if the project directory contains a valid agentuity.yaml file
func ValidateAgentuityProject(projectDir string) error {

	agentuityYamlPath := filepath.Join(projectDir, "agentuity.yaml")

	// Check if agentuity.yaml exists
	if _, err := os.Stat(agentuityYamlPath); err != nil {
		if os.IsNotExist(err) {
			return errsystem.New(errsystem.ErrNotValidAgentuityProject,
				fmt.Errorf("agentuity.yaml not found in project root"),
				errsystem.WithUserMessage("The downloaded repository is not a valid Agentuity project. An 'agentuity.yaml' file is required in the project root."))
		}
		return fmt.Errorf("failed to check agentuity.yaml: %w", err)
	}

	return nil
}

// DownloadAndExtractRepo downloads a repository and extracts it to the specified directory
func DownloadAndExtractRepo(ctx context.Context, logger logger.Logger, projectDir string, repoInfo *RepoInfo) error {
	if repoInfo == nil {
		return fmt.Errorf("repoInfo cannot be nil")
	}

	// Create the project directory if it doesn't exist
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Download the repository as a tar.gz file
	tarURL := fmt.Sprintf("https://codeload.github.com/%s/%s/tar.gz/%s",
		repoInfo.Username, repoInfo.Name, repoInfo.Branch)

	logger.Debug("Downloading repository from: %s", tarURL)

	req, err := http.NewRequestWithContext(ctx, "GET", tarURL, nil)
	if err != nil {
		return errsystem.New(errsystem.ErrDownloadGithubRepository,
			fmt.Errorf("failed to create download request: %w", err))
	}

	req.Header.Set("User-Agent", util.UserAgent())

	client := &http.Client{
		Timeout: 5 * time.Minute, // Longer timeout for large repositories
	}

	resp, err := client.Do(req)
	if err != nil {
		return errsystem.New(errsystem.ErrDownloadGithubRepository,
			fmt.Errorf("failed to download repository: %w", err),
			errsystem.WithUserMessage("Failed to download the repository from GitHub. Please check your internet connection and try again."))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errsystem.New(errsystem.ErrDownloadGithubRepository,
			fmt.Errorf("failed to download repository: HTTP %d", resp.StatusCode),
			errsystem.WithUserMessage("Failed to download the repository. The repository may not exist or the branch '%s' may not be available.", repoInfo.Branch))
	}

	// Extract the tar.gz content
	if err := extractTarGz(logger, resp.Body, projectDir, repoInfo.FilePath); err != nil {
		return errsystem.New(errsystem.ErrExtractGithubRepository,
			fmt.Errorf("failed to extract repository: %w", err))
	}

	// Validate that this is a valid Agentuity project
	if err := ValidateAgentuityProject(projectDir); err != nil {
		return err
	}

	logger.Debug("Successfully extracted repository to: %s", projectDir)
	return nil
}

// IsHiddenFileAllowed checks if a hidden file should be preserved
func IsHiddenFileAllowed(fileName string) bool {
	for _, allowedFile := range allowedHiddenFiles {
		if fileName == allowedFile || strings.HasPrefix(fileName, allowedFile+".") {
			return true
		}
	}
	return false
}

// extractTarGz extracts a tar.gz stream to the specified directory
func extractTarGz(logger logger.Logger, reader io.Reader, destDir, targetPath string) error {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var rootPath string
	stripCount := 1

	// Calculate strip count based on target path
	if targetPath != "" {
		stripCount += len(strings.Split(targetPath, "/"))
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		logger.Debug("Processing tar entry: %s (type: %c)", header.Name, header.Typeflag)

		// Skip disallowed hidden files
		fileName := filepath.Base(header.Name)
		if strings.HasPrefix(fileName, ".") && !IsHiddenFileAllowed(fileName) {
			continue
		}

		// Determine the root path dynamically from the first entry
		if rootPath == "" {
			pathParts := strings.Split(header.Name, "/")
			if len(pathParts) > 0 {
				rootPath = pathParts[0]
			}
		}

		// Check if this file should be extracted based on target path
		expectedPrefix := rootPath
		if targetPath != "" {
			expectedPrefix = filepath.Join(rootPath, targetPath)
		}

		// Normalize paths for comparison
		normalizedHeaderName := strings.ReplaceAll(header.Name, "\\", "/")
		normalizedPrefix := strings.ReplaceAll(expectedPrefix, "\\", "/")

		// More flexible prefix matching
		shouldExtract := false
		if targetPath == "" {
			// Extract everything under the root path
			shouldExtract = strings.HasPrefix(normalizedHeaderName, normalizedPrefix+"/") ||
				normalizedHeaderName == normalizedPrefix
		} else {
			// Extract files matching the target path
			shouldExtract = strings.HasPrefix(normalizedHeaderName, normalizedPrefix+"/") ||
				normalizedHeaderName == normalizedPrefix
		}

		if !shouldExtract {
			logger.Debug("Skipping file due to path filter: %s (expected prefix: %s)", normalizedHeaderName, normalizedPrefix)
			continue
		}

		// Calculate the relative path by stripping the prefix
		relativePath := strings.TrimPrefix(normalizedHeaderName, normalizedPrefix)
		if relativePath != "" && relativePath[0] == '/' {
			relativePath = relativePath[1:]
		}

		// Skip if this would create an empty path for regular files
		if relativePath == "" && header.Typeflag == tar.TypeReg {
			logger.Debug("Skipping file with empty relative path: %s", header.Name)
			continue
		}

		destPath := filepath.Join(destDir, relativePath)
		logger.Debug("Extracting to: %s (from: %s, relative: %s)", destPath, header.Name, relativePath)

		// Security check: ensure the destination path is within the target directory
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			logger.Warn("Skipping file outside target directory: %s", destPath)
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			logger.Debug("Created directory: %s", destPath)

		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
			}

			// Create and write the file
			file, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", destPath, err)
			}

			_, err = io.Copy(file, tr)
			closeErr := file.Close()

			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", destPath, err)
			}
			if closeErr != nil {
				return fmt.Errorf("failed to close file %s: %w", destPath, closeErr)
			}

			// Set file permissions
			if err := os.Chmod(destPath, os.FileMode(header.Mode)); err != nil {
				logger.Debug("Failed to set permissions for %s: %v", destPath, err)
			}

			logger.Debug("Extracted file: %s", destPath)
		}
	}

	return nil
}

// cleanFilePath cleans and normalizes a file path
func cleanFilePath(path string) string {
	if path == "" {
		return ""
	}

	// Remove leading/trailing slashes and clean the path
	cleaned := strings.Trim(path, "/")
	return filepath.Clean(cleaned)
}

// ValidateGitHubURL validates that a URL is a valid GitHub repository URL
func ValidateGitHubURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("URL cannot be empty"),
			errsystem.WithUserMessage("Please provide a valid GitHub repository URL."))
	}

	// Add https:// if no scheme is provided
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("invalid URL format: %w", err),
			errsystem.WithUserMessage("The provided URL format is invalid. Please check the URL and try again."))
	}

	if parsedURL.Host != "github.com" {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("only GitHub URLs are supported, got: %s", parsedURL.Host),
			errsystem.WithUserMessage("Only GitHub repositories are supported. Please provide a github.com URL."))
	}

	// Basic validation of the path
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("invalid GitHub URL: missing username or repository name"),
			errsystem.WithUserMessage("The GitHub URL must include both a username and repository name (e.g., github.com/username/repository)."))
	}

	// Validate username and repo name format (basic GitHub rules)
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9]|[a-zA-Z0-9]*)$`)
	repoRegex := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	username := pathParts[0]
	repoName := pathParts[1]

	if !usernameRegex.MatchString(username) {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("invalid GitHub username format: %s", username),
			errsystem.WithUserMessage("The GitHub username '%s' contains invalid characters.", username))
	}

	if !repoRegex.MatchString(repoName) {
		return nil, errsystem.New(errsystem.ErrInvalidGithubUrlFormat,
			fmt.Errorf("invalid GitHub repository name format: %s", repoName),
			errsystem.WithUserMessage("The GitHub repository name '%s' contains invalid characters.", repoName))
	}

	return parsedURL, nil
}

// ValidateAgentuityProjectPath checks if a GitHub repository path contains an agentuity.yaml file
func ValidateAgentuityProjectPath(ctx context.Context, logger logger.Logger, repoInfo *RepoInfo) error {
	if repoInfo == nil {
		return fmt.Errorf("repoInfo cannot be nil")
	}

	// Construct the path to check for agentuity.yaml
	var agentuityYamlPath string
	if repoInfo.FilePath != "" {
		agentuityYamlPath = fmt.Sprintf("%s/agentuity.yaml", strings.Trim(repoInfo.FilePath, "/"))
	} else {
		agentuityYamlPath = "agentuity.yaml"
	}

	// Check if agentuity.yaml exists in the specified path via GitHub API
	contentsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		repoInfo.Username, repoInfo.Name, agentuityYamlPath, repoInfo.Branch)

	logger.Debug("Checking for agentuity.yaml at: %s", contentsURL)

	req, err := http.NewRequestWithContext(ctx, "HEAD", contentsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}

	req.Header.Set("User-Agent", util.UserAgent())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check agentuity.yaml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		pathDesc := "repository root"
		if repoInfo.FilePath != "" {
			pathDesc = fmt.Sprintf("path '%s'", repoInfo.FilePath)
		}
		return errsystem.New(errsystem.ErrNotValidAgentuityProject,
			fmt.Errorf("agentuity.yaml not found in %s", pathDesc),
			errsystem.WithUserMessage("The specified %s does not contain an 'agentuity.yaml' file. Please ensure you're pointing to a valid Agentuity project directory.", pathDesc))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to validate agentuity.yaml: HTTP %d", resp.StatusCode)
	}

	logger.Debug("Found agentuity.yaml in the specified path")
	return nil
}
