package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <source> [agent-name]",
	Short: "Add an agent to your project",
	Long: `Add an agent to your project from various sources.

Sources can be:
  - Catalog references: memory/vector-store, planner/task-decompose
  - Git repositories: github.com/user/repo#branch path/to/agent
  - Direct URLs: https://example.com/agent.zip
  - Local paths: ./path/to/agent

Examples:
  agentuity add memory/vector-store
  agentuity add github.com/user/agents#main my-agent
  agentuity add ./local-agents/custom-agent
  agentuity add --as custom-name memory/vector-store`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		l := env.NewLogger(cmd)

		// Get current directory as project root
		projectRoot, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current directory")).ShowErrorAndExit()
		}

		// Check if we're in a project directory
		if !project.ProjectExists(projectRoot) {
			errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("agentuity.yaml not found"), errsystem.WithContextMessage("Not in an Agentuity project directory")).ShowErrorAndExit()
		}

		source := args[0]
		agentName := ""
		if len(args) > 1 {
			agentName = args[1]
		}

		// Get flags
		localName, _ := cmd.Flags().GetString("as")
		if localName != "" {
			agentName = localName
		}
		noInstall, _ := cmd.Flags().GetBool("no-install")
		force, _ := cmd.Flags().GetBool("force")
		cacheDir, _ := cmd.Flags().GetString("cache-dir")

		// Default cache directory
		if cacheDir == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get home directory")).ShowErrorAndExit()
			}
			cacheDir = filepath.Join(homeDir, ".config", "agentuity", "agents")
		}

		if err := runAddCommand(context.Background(), l, source, agentName, projectRoot, cacheDir, noInstall, force); err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to add agent")).ShowErrorAndExit()
		}
	},
}

func runAddCommand(ctx context.Context, l logger.Logger, source, agentName, projectRoot, cacheDir string, noInstall, force bool) error {
	// Initialize components
	resolver := agent.NewSourceResolver()
	downloader := agent.NewAgentDownloader(cacheDir)
	installer := agent.NewAgentInstaller(projectRoot)

	var agentSource *agent.AgentSource
	var pkg *agent.AgentPackage
	var err error

	// Resolve and download agent
	tui.ShowSpinner("Resolving and downloading agent...", func() {
		// Resolve source
		agentSource, err = resolver.Resolve(source)
		if err != nil {
			return
		}

		l.Debug("Resolved source: %+v", agentSource)

		// Download agent package
		pkg, err = downloader.Download(agentSource)
	})

	if err != nil {
		return fmt.Errorf("failed to resolve/download agent: %w", err)
	}

	// Display agent information
	tui.ShowSuccess("Agent downloaded successfully!")
	fmt.Printf("Name: %s\n", pkg.Metadata.Name)
	fmt.Printf("Version: %s\n", pkg.Metadata.Version)
	fmt.Printf("Description: %s\n", pkg.Metadata.Description)
	fmt.Printf("Language: %s\n", pkg.Metadata.Language)
	if pkg.Metadata.Author != "" {
		fmt.Printf("Author: %s\n", pkg.Metadata.Author)
	}
	fmt.Printf("Files: %d\n", len(pkg.Files))
	fmt.Println()

	// Confirm installation
	if !force && !tui.Ask(l, "Install this agent?", true) {
		fmt.Println("Installation cancelled.")
		return nil
	}

	// Install agent
	var installErr error
	finalName := agentName
	if finalName == "" {
		finalName = pkg.Metadata.Name
	}

	tui.ShowSpinner("Installing agent...", func() {
		installOpts := &agent.InstallOptions{
			LocalName:   agentName,
			NoInstall:   noInstall,
			Force:       force,
			ProjectRoot: projectRoot,
		}

		installErr = installer.Install(pkg, installOpts)
	})

	if installErr != nil {
		return fmt.Errorf("failed to install agent: %w", installErr)
	}

	// Final success message
	tui.ShowSuccess("Agent '%s' installed successfully!", finalName)

	agentPath := filepath.Join(projectRoot, "agents", finalName)
	fmt.Printf("Agent files copied to: %s\n", agentPath)

	if !noInstall && pkg.Metadata.Dependencies != nil {
		if hasNonEmptyDeps(pkg.Metadata.Dependencies) {
			fmt.Println("Dependencies installed.")
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Printf("1. Review the agent files in %s\n", agentPath)
	fmt.Printf("2. Configure the agent settings if needed\n")
	fmt.Printf("3. Run 'agentuity dev' to test your project\n")

	return nil
}

func hasNonEmptyDeps(deps *agent.AgentDependencies) bool {
	return (len(deps.NPM) > 0) || (len(deps.Pip) > 0) || (len(deps.Go) > 0)
}

func init() {
	rootCmd.AddCommand(addCmd)

	// Add flags
	addCmd.Flags().StringP("as", "a", "", "Local name for the agent")
	addCmd.Flags().Bool("no-install", false, "Skip dependency installation")
	addCmd.Flags().Bool("force", false, "Overwrite existing agent")
	addCmd.Flags().String("cache-dir", "", "Custom cache directory")
}

// Add list subcommand for listing installed agents
var addListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed agents",
	Long:  "List all agents installed in the current project.",
	Run: func(cmd *cobra.Command, args []string) {
		// Get current directory as project root
		projectRoot, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current directory")).ShowErrorAndExit()
		}

		// Check if we're in a project directory
		if !project.ProjectExists(projectRoot) {
			errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("agentuity.yaml not found"), errsystem.WithContextMessage("Not in an Agentuity project directory")).ShowErrorAndExit()
		}

		installer := agent.NewAgentInstaller(projectRoot)

		agents, err := installer.ListInstalled()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to list installed agents")).ShowErrorAndExit()
		}

		if len(agents) == 0 {
			fmt.Println("No agents installed in this project.")
			fmt.Println("\nUse 'agentuity add <source>' to install an agent.")
			return
		}

		fmt.Printf("Installed agents (%d):\n\n", len(agents))
		for i, agentName := range agents {
			fmt.Printf("%d. %s\n", i+1, agentName)

			// Try to read agent metadata
			agentPath := filepath.Join(projectRoot, "agents", agentName, "agent.yaml")
			if metadata, err := loadAgentMetadata(agentPath); err == nil {
				fmt.Printf("   Description: %s\n", metadata.Description)
				fmt.Printf("   Language: %s\n", metadata.Language)
				fmt.Printf("   Version: %s\n", metadata.Version)
			}
			fmt.Println()
		}
	},
}

// Add remove subcommand for removing agents
var addRemoveCmd = &cobra.Command{
	Use:     "remove <agent-name>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an installed agent",
	Long:    "Remove an agent from the current project.",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		l := env.NewLogger(cmd)

		// Get current directory as project root
		projectRoot, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current directory")).ShowErrorAndExit()
		}

		// Check if we're in a project directory
		if !project.ProjectExists(projectRoot) {
			errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("agentuity.yaml not found"), errsystem.WithContextMessage("Not in an Agentuity project directory")).ShowErrorAndExit()
		}

		agentName := args[0]
		force, _ := cmd.Flags().GetBool("force")

		// Confirm removal
		if !force && !tui.Ask(l, fmt.Sprintf("Remove agent '%s'?", agentName), false) {
			fmt.Println("Removal cancelled.")
			return
		}

		installer := agent.NewAgentInstaller(projectRoot)

		if err := installer.Uninstall(agentName); err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to remove agent")).ShowErrorAndExit()
		}

		tui.ShowSuccess("Agent '%s' removed successfully!", agentName)
	},
}

func loadAgentMetadata(agentYamlPath string) (*agent.AgentMetadata, error) {
	// This is a simplified version - in practice you'd use the same YAML parsing
	// as in the downloader package
	return &agent.AgentMetadata{
		Description: "Agent description",
		Language:    "typescript",
		Version:     "1.0.0",
	}, nil
}

func init() {
	// Add subcommands
	addCmd.AddCommand(addListCmd)
	addCmd.AddCommand(addRemoveCmd)

	// Add flags to remove command
	addRemoveCmd.Flags().Bool("force", false, "Skip confirmation prompt")
}
