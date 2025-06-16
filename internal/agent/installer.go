package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"gopkg.in/yaml.v3"
)

type AgentInstaller struct {
	projectRoot string
	validator   *AgentValidator
}

func NewAgentInstaller(projectRoot string) *AgentInstaller {
	return &AgentInstaller{
		projectRoot: projectRoot,
		validator:   NewAgentValidator(false),
	}
}

func (i *AgentInstaller) Install(pkg *AgentPackage, opts *InstallOptions) error {
	// Validate package
	validation := i.validator.ValidatePackage(pkg)
	if !validation.Valid {
		return fmt.Errorf("package validation failed: %s", i.formatValidationErrors(validation.Errors))
	}

	// Determine agent name
	agentName := opts.LocalName
	if agentName == "" {
		agentName = pkg.Metadata.Name
	}

	// Validate installation path
	if err := i.validator.ValidateInstallPath(i.projectRoot, agentName); err != nil && !opts.Force {
		return fmt.Errorf("installation path validation failed: %w", err)
	}

	// Create agent directory
	agentDir := filepath.Join(i.projectRoot, "agents", agentName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	// Copy files
	if err := i.copyAgentFiles(pkg, agentDir); err != nil {
		return fmt.Errorf("failed to copy agent files: %w", err)
	}

	// Update project configuration
	if err := i.updateProjectConfig(agentName, pkg.Metadata); err != nil {
		return fmt.Errorf("failed to update project configuration: %w", err)
	}

	// Install dependencies if requested
	if !opts.NoInstall && pkg.Metadata.Dependencies != nil {
		if err := i.installDependencies(pkg.Metadata.Dependencies, pkg.Metadata.Language); err != nil {
			return fmt.Errorf("failed to install dependencies: %w", err)
		}
	}

	return nil
}

func (i *AgentInstaller) copyAgentFiles(pkg *AgentPackage, agentDir string) error {
	for relativePath, content := range pkg.Files {
		// Ensure the file path is within the agent directory
		if strings.Contains(relativePath, "..") {
			continue // Skip files with path traversal
		}

		filePath := filepath.Join(agentDir, relativePath)

		// Create directory if needed
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write file
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	// Create agent.yaml in the agent directory
	agentYamlPath := filepath.Join(agentDir, "agent.yaml")
	agentYamlContent, err := yaml.Marshal(pkg.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal agent metadata: %w", err)
	}

	if err := os.WriteFile(agentYamlPath, agentYamlContent, 0644); err != nil {
		return fmt.Errorf("failed to write agent.yaml: %w", err)
	}

	return nil
}

func (i *AgentInstaller) updateProjectConfig(agentName string, metadata *AgentMetadata) error {
	projectConfigPath := filepath.Join(i.projectRoot, "agentuity.yaml")

	// Check if project config exists
	if !util.Exists(projectConfigPath) {
		return fmt.Errorf("project configuration not found at: %s", projectConfigPath)
	}

	// Load existing project config
	var proj project.Project
	if err := proj.Load(i.projectRoot); err != nil {
		return fmt.Errorf("failed to load project configuration: %w", err)
	}

	// Check if agent already exists
	for _, agent := range proj.Agents {
		if agent.Name == agentName {
			return fmt.Errorf("agent with name '%s' already exists in project configuration", agentName)
		}
	}

	// Add new agent to configuration
	newAgent := project.AgentConfig{
		ID:          generateAgentID(agentName),
		Name:        agentName,
		Description: metadata.Description,
		Types:       []string{}, // Will be populated based on agent implementation
	}

	proj.Agents = append(proj.Agents, newAgent)

	// Save updated configuration
	if err := i.saveProjectConfig(&proj); err != nil {
		return fmt.Errorf("failed to save project configuration: %w", err)
	}

	return nil
}

func (i *AgentInstaller) saveProjectConfig(proj *project.Project) error {
	projectConfigPath := filepath.Join(i.projectRoot, "agentuity.yaml")

	configContent, err := yaml.Marshal(proj)
	if err != nil {
		return fmt.Errorf("failed to marshal project configuration: %w", err)
	}

	if err := os.WriteFile(projectConfigPath, configContent, 0644); err != nil {
		return fmt.Errorf("failed to write project configuration: %w", err)
	}

	return nil
}

func (i *AgentInstaller) installDependencies(deps *AgentDependencies, language string) error {
	switch strings.ToLower(language) {
	case "typescript", "javascript":
		return i.installNPMDependencies(deps.NPM)
	case "python":
		return i.installPipDependencies(deps.Pip)
	case "go":
		return i.installGoDependencies(deps.Go)
	default:
		return fmt.Errorf("unsupported language for dependency installation: %s", language)
	}
}

func (i *AgentInstaller) installNPMDependencies(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	// Check if package.json exists
	packageJsonPath := filepath.Join(i.projectRoot, "package.json")
	if !util.Exists(packageJsonPath) {
		return fmt.Errorf("package.json not found in project root")
	}

	// Install packages using npm
	args := append([]string{"install"}, packages...)
	if err := i.runCommand("npm", args...); err != nil {
		return fmt.Errorf("failed to install npm packages: %w", err)
	}

	return nil
}

func (i *AgentInstaller) installPipDependencies(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	// Install packages using pip
	args := append([]string{"install"}, packages...)
	if err := i.runCommand("pip", args...); err != nil {
		return fmt.Errorf("failed to install pip packages: %w", err)
	}

	return nil
}

func (i *AgentInstaller) installGoDependencies(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	// Check if go.mod exists
	goModPath := filepath.Join(i.projectRoot, "go.mod")
	if !util.Exists(goModPath) {
		return fmt.Errorf("go.mod not found in project root")
	}

	// Install packages using go get
	for _, pkg := range packages {
		if err := i.runCommand("go", "get", pkg); err != nil {
			return fmt.Errorf("failed to install go package %s: %w", pkg, err)
		}
	}

	return nil
}

func (i *AgentInstaller) runCommand(name string, args ...string) error {
	// This is a placeholder for command execution
	// In a real implementation, you would use exec.Command
	// For now, we'll just log what would be executed
	fmt.Printf("Would execute: %s %s\n", name, strings.Join(args, " "))
	return nil
}

func (i *AgentInstaller) formatValidationErrors(errors []string) string {
	var messages []string
	for _, err := range errors {
		messages = append(messages, err)
	}
	return strings.Join(messages, "; ")
}

func generateAgentID(name string) string {
	// Generate a simple ID based on the name
	// In a real implementation, you might want to use a UUID or hash
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

func (i *AgentInstaller) Uninstall(agentName string) error {
	// Remove agent directory
	agentDir := filepath.Join(i.projectRoot, "agents", agentName)
	if util.Exists(agentDir) {
		if err := os.RemoveAll(agentDir); err != nil {
			return fmt.Errorf("failed to remove agent directory: %w", err)
		}
	}

	// Update project configuration
	var proj project.Project
	if err := proj.Load(i.projectRoot); err != nil {
		return fmt.Errorf("failed to load project configuration: %w", err)
	}

	// Remove agent from configuration
	var updatedAgents []project.AgentConfig
	for _, agent := range proj.Agents {
		if agent.Name != agentName {
			updatedAgents = append(updatedAgents, agent)
		}
	}

	proj.Agents = updatedAgents

	// Save updated configuration
	if err := i.saveProjectConfig(&proj); err != nil {
		return fmt.Errorf("failed to save updated project configuration: %w", err)
	}

	return nil
}

func (i *AgentInstaller) ListInstalled() ([]string, error) {
	agentsDir := filepath.Join(i.projectRoot, "agents")
	if !util.Exists(agentsDir) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var agents []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it has an agent.yaml file
			agentYaml := filepath.Join(agentsDir, entry.Name(), "agent.yaml")
			if util.Exists(agentYaml) {
				agents = append(agents, entry.Name())
			}
		}
	}

	return agents, nil
}
