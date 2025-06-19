package agent

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentuity/cli/internal/util"
)

const (
	MaxAgentNameLength = 50
	MaxFileSize        = 10 * 1024 * 1024 // 10MB per file
)

var (
	validAgentNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	allowedFileExtensions = map[string]bool{
		".ts":   true,
		".js":   true,
		".py":   true,
		".go":   true,
		".yaml": true,
		".yml":  true,
		".json": true,
		".md":   true,
		".txt":  true,
		".toml": true,
		".sh":   true,
		".bat":  true,
	}
	dangerousFileExtensions = map[string]bool{
		".exe": true,
		".dll": true,
		".so":  true,
		".bin": true,
		".app": true,
	}
)

type AgentValidator struct {
	strictMode bool
}

func NewAgentValidator(strictMode bool) *AgentValidator {
	return &AgentValidator{
		strictMode: strictMode,
	}
}

func (v *AgentValidator) ValidatePackage(pkg *AgentPackage) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Errors: []string{},
	}

	// Validate metadata
	v.validateMetadata(pkg.Metadata, result)

	// Validate files
	v.validateFiles(pkg.Files, result)

	// Validate security
	v.validateSecurity(pkg, result)

	result.Valid = len(result.Errors) == 0
	return result
}

func (v *AgentValidator) validateMetadata(metadata *AgentMetadata, result *ValidationResult) {
	if metadata == nil {
		result.Errors = append(result.Errors, "metadata: agent metadata is required")
		return
	}

	// Validate name
	if metadata.Name == "" {
		result.Errors = append(result.Errors, "name: agent name is required")
	} else {
		if len(metadata.Name) > MaxAgentNameLength {
			result.Errors = append(result.Errors, fmt.Sprintf("name: agent name too long (max %d characters)", MaxAgentNameLength))
		}
		if !validAgentNameRegex.MatchString(metadata.Name) {
			result.Errors = append(result.Errors, "name: agent name contains invalid characters (only alphanumeric, dots, underscores, and hyphens allowed)")
		}
	}

	// Validate version
	if metadata.Version == "" {
		result.Errors = append(result.Errors, "version: agent version is required")
	}

	// Validate description
	if metadata.Description == "" {
		result.Errors = append(result.Errors, "description: agent description is required")
	}

	// Validate language
	if metadata.Language == "" {
		result.Errors = append(result.Errors, "language: agent language is required")
	} else {
		validLanguages := []string{"typescript", "javascript", "python", "go"}
		valid := false
		for _, lang := range validLanguages {
			if strings.ToLower(metadata.Language) == lang {
				valid = true
				break
			}
		}
		if !valid {
			result.Errors = append(result.Errors, fmt.Sprintf("language: unsupported language: %s (supported: %s)", metadata.Language, strings.Join(validLanguages, ", ")))
		}
	}

	// Validate files list
	if len(metadata.Files) == 0 {
		result.Errors = append(result.Errors, "files: agent must specify at least one file")
	}

	// Validate file paths
	for _, file := range metadata.Files {
		if strings.Contains(file, "..") {
			result.Errors = append(result.Errors, fmt.Sprintf("files: file path contains directory traversal: %s", file))
		}
		if filepath.IsAbs(file) {
			result.Errors = append(result.Errors, fmt.Sprintf("files: file path must be relative: %s", file))
		}
	}
}

func (v *AgentValidator) validateFiles(files map[string][]byte, result *ValidationResult) {
	if len(files) == 0 {
		result.Errors = append(result.Errors, "files: agent package must contain files")
		return
	}

	for filename, content := range files {
		// Check file size
		if len(content) > MaxFileSize {
			result.Errors = append(result.Errors, fmt.Sprintf("files: file too large: %s (%d bytes, max %d)", filename, len(content), MaxFileSize))
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != "" {
			if dangerousFileExtensions[ext] {
				result.Errors = append(result.Errors, fmt.Sprintf("files: dangerous file extension not allowed: %s", filename))
			} else if v.strictMode && !allowedFileExtensions[ext] {
				result.Errors = append(result.Errors, fmt.Sprintf("files: file extension not allowed: %s", filename))
			}
		}

		// Check for suspicious content
		v.validateFileContent(filename, content, result)
	}
}

func (v *AgentValidator) validateFileContent(filename string, content []byte, result *ValidationResult) {
	contentStr := string(content)

	// Check for suspicious patterns
	suspiciousPatterns := []struct {
		pattern string
		message string
	}{
		{`(?i)eval\s*\(`, "potentially dangerous eval() usage"},
		{`(?i)exec\s*\(`, "potentially dangerous exec() usage"},
		{`(?i)system\s*\(`, "potentially dangerous system() usage"},
		{`(?i)shell_exec`, "potentially dangerous shell execution"},
		{`(?i)passthru`, "potentially dangerous command execution"},
		{`(?i)base64_decode`, "potentially obfuscated code"},
		{`(?i)document\.write\s*\(`, "potentially dangerous DOM manipulation"},
		{`(?i)innerHTML\s*=`, "potentially dangerous HTML injection"},
	}

	for _, pattern := range suspiciousPatterns {
		matched, _ := regexp.MatchString(pattern.pattern, contentStr)
		if matched {
			result.Errors = append(result.Errors, fmt.Sprintf("files: suspicious content in %s: %s", filename, pattern.message))
		}
	}

	// Check for hardcoded secrets patterns
	secretPatterns := []struct {
		pattern string
		message string
	}{
		{`(?i)(api[_-]?key|apikey)\s*[:=]\s*["\']?[a-zA-Z0-9]{20,}`, "potential API key"},
		{`(?i)(secret|password|passwd|pwd)\s*[:=]\s*["\']?[a-zA-Z0-9]{8,}`, "potential hardcoded secret"},
		{`(?i)token\s*[:=]\s*["\']?[a-zA-Z0-9]{20,}`, "potential access token"},
		{`sk-[a-zA-Z0-9]{20,}`, "potential OpenAI API key"},
		{`ghp_[a-zA-Z0-9]{36}`, "potential GitHub personal access token"},
	}

	for _, pattern := range secretPatterns {
		matched, _ := regexp.MatchString(pattern.pattern, contentStr)
		if matched {
			result.Errors = append(result.Errors, fmt.Sprintf("files: potential hardcoded secret in %s: %s", filename, pattern.message))
		}
	}
}

func (v *AgentValidator) validateSecurity(pkg *AgentPackage, result *ValidationResult) {
	// Check source security
	if pkg.Source.Type == SourceTypeURL {
		if !strings.HasPrefix(pkg.Source.Location, "https://") {
			result.Errors = append(result.Errors, "source: only HTTPS URLs are allowed for security")
		}
	}

	// Validate dependencies if present
	if pkg.Metadata.Dependencies != nil {
		v.validateDependencies(pkg.Metadata.Dependencies, result)
	}
}

func (v *AgentValidator) validateDependencies(deps *AgentDependencies, result *ValidationResult) {
	// Check for known malicious packages
	maliciousPackages := map[string][]string{
		"npm": {"event-stream", "eslint-scope", "getcookies"},
		"pip": {"python3-dateutil", "python3-urllib3", "jeIlyfish"},
	}

	if deps.NPM != nil {
		for _, pkg := range deps.NPM {
			if v.isMaliciousPackage("npm", pkg, maliciousPackages["npm"]) {
				result.Errors = append(result.Errors, fmt.Sprintf("dependencies: potentially malicious NPM package: %s", pkg))
			}
		}
	}

	if deps.Pip != nil {
		for _, pkg := range deps.Pip {
			if v.isMaliciousPackage("pip", pkg, maliciousPackages["pip"]) {
				result.Errors = append(result.Errors, fmt.Sprintf("dependencies: potentially malicious Python package: %s", pkg))
			}
		}
	}
}

func (v *AgentValidator) isMaliciousPackage(packageManager, packageName string, maliciousList []string) bool {
	for _, malicious := range maliciousList {
		if strings.EqualFold(packageName, malicious) {
			return true
		}
	}
	return false
}

func (v *AgentValidator) ValidateInstallPath(projectRoot, agentName string) error {
	if agentName == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	if !validAgentNameRegex.MatchString(agentName) {
		return fmt.Errorf("invalid agent name: %s (only alphanumeric, dots, underscores, and hyphens allowed)", agentName)
	}

	agentPath := filepath.Join(projectRoot, "agents", agentName)
	if util.Exists(agentPath) {
		return fmt.Errorf("agent already exists at: %s", agentPath)
	}

	return nil
}
