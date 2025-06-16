package agent

import (
	"fmt"
	"strings"
)

// Custom error types for better user experience

type AgentNotFoundError struct {
	Source string
	Cause  error
}

func (e *AgentNotFoundError) Error() string {
	baseName := getBaseName(e.Source)
	msg := fmt.Sprintf("Agent not found: %s", e.Source)
	if e.Cause != nil {
		msg += fmt.Sprintf(" (%s)", e.Cause.Error())
	}
	msg += fmt.Sprintf("\n\nTry:\n  - Check the agent name spelling\n  - Use 'agentuity search %s' to find similar agents\n  - Verify the source URL is accessible", baseName)
	return msg
}

func (e *AgentNotFoundError) Unwrap() error {
	return e.Cause
}

type ConflictError struct {
	AgentName    string
	ExistingPath string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("Agent '%s' already exists at: %s\n\nUse --force to overwrite or --as <name> to install with a different name", e.AgentName, e.ExistingPath)
}

type ValidationError struct {
	Source string
	Issues []string
}

func (e *ValidationError) Error() string {
	msg := fmt.Sprintf("Agent validation failed for: %s\n\nIssues found:", e.Source)
	for i, issue := range e.Issues {
		msg += fmt.Sprintf("\n  %d. %s", i+1, issue)
	}
	return msg
}

type DependencyInstallError struct {
	Language     string
	Dependencies []string
	Cause        error
}

func (e *DependencyInstallError) Error() string {
	msg := fmt.Sprintf("Failed to install %s dependencies: %s", e.Language, strings.Join(e.Dependencies, ", "))
	if e.Cause != nil {
		msg += fmt.Sprintf("\nError: %s", e.Cause.Error())
	}

	switch strings.ToLower(e.Language) {
	case "typescript", "javascript":
		msg += "\n\nTry:\n  - Ensure npm is installed and accessible\n  - Check your package.json exists\n  - Run 'npm install' manually"
	case "python":
		msg += "\n\nTry:\n  - Ensure pip is installed and accessible\n  - Check if you're in a virtual environment\n  - Run 'pip install <package>' manually"
	case "go":
		msg += "\n\nTry:\n  - Ensure go is installed and accessible\n  - Check your go.mod exists\n  - Run 'go get <package>' manually"
	}

	return msg
}

func (e *DependencyInstallError) Unwrap() error {
	return e.Cause
}

type SourceResolveError struct {
	Source string
	Reason string
	Cause  error
}

func (e *SourceResolveError) Error() string {
	msg := fmt.Sprintf("Failed to resolve source: %s", e.Source)
	if e.Reason != "" {
		msg += fmt.Sprintf("\nReason: %s", e.Reason)
	}
	if e.Cause != nil {
		msg += fmt.Sprintf("\nError: %s", e.Cause.Error())
	}

	msg += "\n\nSupported source formats:"
	msg += "\n  - Catalog: memory/vector-store"
	msg += "\n  - Git: github.com/user/repo#branch path/to/agent"
	msg += "\n  - URL: https://example.com/agent.zip"
	msg += "\n  - Local: ./path/to/agent"

	return msg
}

func (e *SourceResolveError) Unwrap() error {
	return e.Cause
}

type DownloadError struct {
	URL    string
	Reason string
	Cause  error
}

func (e *DownloadError) Error() string {
	msg := fmt.Sprintf("Failed to download from: %s", e.URL)
	if e.Reason != "" {
		msg += fmt.Sprintf("\nReason: %s", e.Reason)
	}
	if e.Cause != nil {
		msg += fmt.Sprintf("\nError: %s", e.Cause.Error())
	}

	msg += "\n\nTry:\n  - Check your internet connection\n  - Verify the URL is accessible\n  - Use --cache-dir to specify a different cache location"

	return msg
}

func (e *DownloadError) Unwrap() error {
	return e.Cause
}

type SecurityError struct {
	Issue  string
	Detail string
}

func (e *SecurityError) Error() string {
	msg := fmt.Sprintf("Security check failed: %s", e.Issue)
	if e.Detail != "" {
		msg += fmt.Sprintf("\nDetail: %s", e.Detail)
	}
	msg += "\n\nThis agent was rejected for security reasons. Only install agents from trusted sources."
	return msg
}

type ProjectConfigError struct {
	ConfigPath string
	Cause      error
}

func (e *ProjectConfigError) Error() string {
	msg := fmt.Sprintf("Failed to update project configuration: %s", e.ConfigPath)
	if e.Cause != nil {
		msg += fmt.Sprintf("\nError: %s", e.Cause.Error())
	}
	msg += "\n\nTry:\n  - Check file permissions\n  - Ensure agentuity.yaml is valid\n  - Run 'agentuity init' if needed"
	return msg
}

func (e *ProjectConfigError) Unwrap() error {
	return e.Cause
}

// Helper functions

func getBaseName(source string) string {
	// Extract the base name from different source types
	if strings.Contains(source, "/") {
		parts := strings.Split(source, "/")
		return parts[len(parts)-1]
	}
	return source
}

// Wrap common errors with more user-friendly messages

func WrapAgentNotFound(source string, err error) error {
	return &AgentNotFoundError{
		Source: source,
		Cause:  err,
	}
}

func WrapValidationError(source string, issues []string) error {
	return &ValidationError{
		Source: source,
		Issues: issues,
	}
}

func WrapDependencyError(language string, deps []string, err error) error {
	return &DependencyInstallError{
		Language:     language,
		Dependencies: deps,
		Cause:        err,
	}
}

func WrapSourceResolveError(source, reason string, err error) error {
	return &SourceResolveError{
		Source: source,
		Reason: reason,
		Cause:  err,
	}
}

func WrapDownloadError(url, reason string, err error) error {
	return &DownloadError{
		URL:    url,
		Reason: reason,
		Cause:  err,
	}
}

func WrapSecurityError(issue, detail string) error {
	return &SecurityError{
		Issue:  issue,
		Detail: detail,
	}
}

func WrapProjectConfigError(configPath string, err error) error {
	return &ProjectConfigError{
		ConfigPath: configPath,
		Cause:      err,
	}
}
