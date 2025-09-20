package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

type EvalObject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ProjectID   string `json:"projectId"`
	OrgID       string `json:"orgId"`
}

type EvalPullObject struct {
	Code        string `json:"code"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type EvalCreateResponse = project.Response[EvalObject]
type EvalPullResponse = project.Response[EvalPullObject]

func CreateGenerativeEvaluation(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp EvalCreateResponse
	payload := map[string]any{
		"projectId": projectId,
		"type":      "generative",
	}

	if err := client.Do("POST", "/cli/eval", payload, &resp); err != nil {
		return "", fmt.Errorf("error creating generative evaluation: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("failed to create generative evaluation: %s", resp.Message)
	}
	return resp.Data.ID, nil
}

func CreateTemplateEvaluation(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, name string, description string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp EvalCreateResponse
	payload := map[string]any{
		"projectId":   projectId,
		"name":        name,
		"description": description,
		"type":        "template",
	}

	if err := client.Do("POST", "/cli/eval", payload, &resp); err != nil {
		return "", fmt.Errorf("error creating template evaluation: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("failed to create template evaluation: %s", resp.Message)
	}
	return resp.Data.ID, nil
}

func PullEvaluation(ctx context.Context, logger logger.Logger, baseUrl string, token string, evalId string) (*EvalPullObject, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp EvalPullResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/eval/pull/%s", evalId), nil, &resp); err != nil {
		return nil, fmt.Errorf("error pulling evaluation: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("failed to pull evaluation: %s", resp.Message)
	}
	return &resp.Data, nil
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluation related commands",
	Long: `Evaluation related commands for managing evaluations and test data.

Use the subcommands to create and pull evaluation data to/from the cloud.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var evalCreateCmd = &cobra.Command{
	Use:   "create [name] [description]",
	Short: "Create evaluation data in the cloud",
	Long: `Create evaluation data in the cloud for your project.

Arguments:
  [name]        Optional name for the evaluation
  [description] Optional description for the evaluation

Flags:
  --force     Don't prompt for confirmation

Examples:
  agentuity eval create
  agentuity eval create "My Eval" "Description of evaluation"
  agentuity eval create --force "My Eval" "Description"`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
		dir := context.Dir
		apiUrl := context.APIURL
		apiKey := context.Token
		theproject := context.Project

		force, _ := cmd.Flags().GetBool("force")

		// First, get the evaluation type
		var evalType string
		if !tui.HasTTY {
			// Default to template when no TTY
			evalType = "template"
		} else {
			evalType = tui.Select(logger, "What type of evaluation would you like to create?", "Choose between template-based or generative evaluation", []tui.Option{
				{Text: tui.PadRight("Template", 20, " ") + tui.Muted("Use a predefined regex evaluation template"), ID: "template"},
				{Text: tui.PadRight("Generative", 20, " ") + tui.Muted("AI will generate custom evaluation code"), ID: "generative"},
			})
		}

		var name, description string

		// Get name and description only for template type
		if evalType == "template" {
			// Get name and description from args or prompt
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				description = args[1]
			}

			// Interactive flow for name and description
			if name == "" {
				if !tui.HasTTY {
					logger.Fatal("No TTY detected, please specify an evaluation name from the command line")
				}
				name = tui.InputWithValidation(logger, "What should we name this evaluation?", "The name helps identify the evaluation", 255, func(name string) error {
					if name == "" {
						return fmt.Errorf("evaluation name cannot be empty")
					}
					return nil
				})
			}

			if description == "" {
				description = tui.Input(logger, "How should we describe what this evaluation tests?", "The description is optional but helpful for understanding the purpose of the evaluation")
			}
		}

		// Confirm create unless force flag is set
		if !force {
			var confirmMessage string
			if evalType == "template" {
				confirmMessage = fmt.Sprintf("Create template evaluation '%s' in the cloud?", name)
			} else {
				confirmMessage = "Create generative evaluation in the cloud?"
			}

			if !tui.Ask(logger, confirmMessage, false) {
				tui.ShowWarning("cancelled")
				return
			}
		}

		var evalId string
		var evalObj *EvalPullObject
		action := func() {
			var err error

			// Call the appropriate function based on type
			if evalType == "template" {
				evalId, err = CreateTemplateEvaluation(ctx, logger, apiUrl, apiKey, theproject.ProjectId, name, description)
			} else {
				evalId, err = CreateGenerativeEvaluation(ctx, logger, apiUrl, apiKey, theproject.ProjectId)
			}

			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to create evaluation")).ShowErrorAndExit()
			}

			// Automatically pull the evaluation data
			evalObj, err = PullEvaluation(ctx, logger, apiUrl, apiKey, evalId)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to pull evaluation data")).ShowErrorAndExit()
			}
		}

		spinner.New().Title("Creating evaluation...").Action(action).Run()

		// Write code to file
		filename := evalObj.Name + ".ts"
		evalsDir := filepath.Join(dir, "src", "evals")

		// Create the evals directory if it doesn't exist
		if err := os.MkdirAll(evalsDir, 0755); err != nil {
			errsystem.New(errsystem.ErrCreateDirectory, err, errsystem.WithUserMessage("Failed to create evals directory")).ShowErrorAndExit()
		}

		filePath := filepath.Join(evalsDir, filename)
		if err := os.WriteFile(filePath, []byte(evalObj.Code), 0644); err != nil {
			errsystem.New(errsystem.ErrOpenFile, err, errsystem.WithUserMessage("Failed to write evaluation code to file")).ShowErrorAndExit()
		}

		if evalType == "template" {
			tui.ShowSuccess("Template evaluation '%s' created successfully with ID: %s", name, evalId)
		} else {
			tui.ShowSuccess("Generative evaluation created successfully with ID: %s", evalId)
		}

		tui.ShowSuccess("Evaluation code written to: %s", filePath)
		fmt.Println("\nEvaluation code:")
		fmt.Println(evalObj.Code)
	},
}

var evalPullCmd = &cobra.Command{
	Use:   "pull <id>",
	Short: "Pull evaluation data from the cloud by ID",
	Long: `Pull evaluation data from the cloud for your project using the evaluation ID.

Arguments:
  <id>    The evaluation ID to pull

Examples:
  agentuity eval pull abc123
  agentuity eval pull def456`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
		dir := context.Dir
		apiUrl := context.APIURL
		apiKey := context.Token

		evalId := args[0]

		var evalObj *EvalPullObject
		action := func() {
			var err error
			evalObj, err = PullEvaluation(ctx, logger, apiUrl, apiKey, evalId)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to pull evaluation")).ShowErrorAndExit()
			}
		}

		spinner.New().Title("Pulling evaluation...").Action(action).Run()

		// Write code to file
		filename := evalObj.Name + ".ts"
		evalsDir := filepath.Join(dir, "src", "evals")

		// Create the evals directory if it doesn't exist
		if err := os.MkdirAll(evalsDir, 0755); err != nil {
			errsystem.New(errsystem.ErrCreateDirectory, err, errsystem.WithUserMessage("Failed to create evals directory")).ShowErrorAndExit()
		}

		filePath := filepath.Join(evalsDir, filename)
		if err := os.WriteFile(filePath, []byte(evalObj.Code), 0644); err != nil {
			errsystem.New(errsystem.ErrOpenFile, err, errsystem.WithUserMessage("Failed to write evaluation code to file")).ShowErrorAndExit()
		}

		tui.ShowSuccess("Evaluation code written to: %s", filePath)

		// Output to stdout
		fmt.Println(evalObj.Code)
	},
}

func init() {
	rootCmd.AddCommand(evalCmd)

	evalCreateCmd.Flags().Bool("force", !hasTTY, "Don't prompt for confirmation")

	evalCmd.AddCommand(evalCreateCmd)
	evalCmd.AddCommand(evalPullCmd)

	for _, cmd := range []*cobra.Command{evalCreateCmd, evalPullCmd} {
		cmd.Flags().StringP("dir", "d", ".", "The directory to the project")
	}
}
