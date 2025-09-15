package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/prompts"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// PromptsYaml represents the full structure of src/prompts.yaml
type PromptsYaml struct {
	Prompts []prompts.Prompt `yaml:"prompts"`
}

// Slugify converts a name into a valid ID slug using dashes (kebab-case)
func Slugify(name string) string {
	// Convert to lowercase and trim whitespace
	slug := strings.ToLower(strings.TrimSpace(name))
	
	// Replace non-alphanumeric characters with dashes
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")
	
	// Remove leading/trailing dashes and collapse multiple dashes
	slug = strings.Trim(slug, "-")
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")
	
	return slug
}

// generateUniqueID creates a unique ID by appending numbers if needed
func generateUniqueID(baseID string, existingPrompts []prompts.Prompt) string {
	id := baseID
	counter := 2
	
	for {
		// Check if ID already exists
		exists := false
		for _, prompt := range existingPrompts {
			if prompt.ID == id {
				exists = true
				break
			}
		}
		
		if !exists {
			return id
		}
		
		// Try with counter
		id = fmt.Sprintf("%s-%d", baseID, counter)
		counter++
	}
}

// readPromptsFile reads existing prompts.yaml or returns empty structure
func readPromptsFile(filePath string) (*PromptsYaml, error) {
	if !util.Exists(filePath) {
		// Return empty structure for new file
		return &PromptsYaml{
			Prompts: []prompts.Prompt{},
		}, nil
	}
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	
	var promptsYaml PromptsYaml
	if err := yaml.Unmarshal(data, &promptsYaml); err != nil {
		return nil, err
	}
	
	return &promptsYaml, nil
}

// writePromptsFile writes the prompts structure to YAML file
func writePromptsFile(filePath string, promptsYaml *PromptsYaml) error {
	// Ensure src directory exists
	srcDir := filepath.Dir(filePath)
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}
	
	data, err := yaml.Marshal(promptsYaml)
	if err != nil {
		return err
	}
	
	return os.WriteFile(filePath, data, 0644)
}

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Prompt related commands",
	Long: `Prompt related commands for managing prompt templates.

Use the subcommands to create and manage prompt templates in your project.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var promptCreateCmd = &cobra.Command{
	Use:   "create [name] [description]",
	Short: "Create a new prompt template",
	Long: `Create a new prompt template in src/prompts.yaml.

This command will add a new prompt entry to your project's prompts.yaml file.
If the file doesn't exist, it will be created. The ID will be automatically
generated from the name using underscores.

Arguments:
  [name]        Optional name for the prompt
  [description] Optional description for the prompt

Flags:
  --force     Don't prompt for confirmation

Examples:
  agentuity prompt create
  agentuity prompt create "Product Helper" "Helps with product descriptions"
  agentuity prompt create --force "My Prompt" "Description"`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
		dir := context.Dir

		force, _ := cmd.Flags().GetBool("force")

		var name, description string

		// Get name and description from args or prompt
		if len(args) > 0 {
			name = args[0]
		}
		if len(args) > 1 {
			description = args[1]
		}

		// Interactive flow for name
		if name == "" {
			if !tui.HasTTY {
				logger.Fatal("No TTY detected, please specify a prompt name from the command line")
			}
			name = tui.InputWithValidation(logger, "What should we name this prompt?", "The name helps identify the prompt template", 255, func(name string) error {
				if name == "" {
					return fmt.Errorf("prompt name cannot be empty")
				}
				return nil
			})
		}

		// Interactive flow for description (optional)
		if description == "" && tui.HasTTY {
			description = tui.Input(logger, "How should we describe what this prompt does?", "The description is optional but helpful for understanding the prompt's purpose")
		}

		// Interactive flow for system and prompt fields (optional)
		var systemMsg, promptBody string
		if tui.HasTTY {
			systemMsg = tui.Input(logger, "Enter an optional SYSTEM message for this prompt", "Leave blank to skip; you can edit prompts.yaml later")
			promptBody = tui.Input(logger, "Enter the USER prompt text", "Leave blank to skip; you can edit prompts.yaml later")
		}

		// Generate ID from name
		baseID := Slugify(name)

		// Read existing prompts file
		promptsFile := filepath.Join(dir, "src", "prompts.yaml")
		promptsYaml, err := readPromptsFile(promptsFile)
		if err != nil {
			errsystem.New(errsystem.ErrOpenFile, err, errsystem.WithUserMessage("Failed to read prompts file")).ShowErrorAndExit()
		}

		// Generate unique ID
		uniqueID := generateUniqueID(baseID, promptsYaml.Prompts)

		// Confirm create unless force flag is set
		if !force {
			confirmMessage := fmt.Sprintf("Create prompt '%s' (%s) in src/prompts.yaml?", name, uniqueID)
			if !tui.Ask(logger, confirmMessage, true) {
				tui.ShowWarning("cancelled")
				return
			}
		}

		// Create new prompt with collected system and prompt fields
		newPrompt := prompts.Prompt{
			ID:          uniqueID,
			Name:        name,
			Description: description,
			System:      systemMsg,
			Prompt:      promptBody,
		}

		// Add to prompts array
		promptsYaml.Prompts = append(promptsYaml.Prompts, newPrompt)

		// Write back to file
		if err := writePromptsFile(promptsFile, promptsYaml); err != nil {
			errsystem.New(errsystem.ErrOpenFile, err, errsystem.WithUserMessage("Failed to write prompts file")).ShowErrorAndExit()
		}

		// Get absolute path for display
		absPath, err := filepath.Abs(promptsFile)
		if err != nil {
			absPath = promptsFile
		}

		tui.ShowSuccess("Prompt '%s' (%s) created successfully", name, uniqueID)
		
		// Show next steps guidance
		nextSteps := "1. Review the prompt in " + absPath + "\n"
		if systemMsg == "" || promptBody == "" {
			nextSteps += "2. Fill in any missing 'system' or 'prompt' fields\n"
		}
		nextSteps += "3. Add the prompt ID to your agent code"
		
		tui.ShowBanner("Next steps", nextSteps, false)
	},
}

func init() {
	rootCmd.AddCommand(promptCmd)

	promptCreateCmd.Flags().Bool("force", !hasTTY, "Don't prompt for confirmation")

	promptCmd.AddCommand(promptCreateCmd)

	for _, cmd := range []*cobra.Command{promptCreateCmd} {
		cmd.Flags().StringP("dir", "d", ".", "The directory to the project")
	}
}
