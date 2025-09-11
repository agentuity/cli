package cmd

import (
	"context"
	"encoding/json"
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
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

type EvalResponse = project.Response[string]

func CreateEvaluation(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, evalData string) error {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp EvalResponse
	payload := map[string]any{
		"data": evalData,
	}
	if err := client.Do("POST", fmt.Sprintf("/cli/project/%s/evaluations", projectId), payload, &resp); err != nil {
		return fmt.Errorf("error creating evaluation: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("failed to create evaluation: %s", resp.Message)
	}
	return nil
}

func PullEvaluation(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, evalId string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp EvalResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s/evaluations/%s", projectId, evalId), nil, &resp); err != nil {
		return "", fmt.Errorf("error pulling evaluation: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("failed to pull evaluation: %s", resp.Message)
	}
	return resp.Data, nil
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
	Use:   "create [file]",
	Short: "Create evaluation data in the cloud",
	Long: `Create evaluation data in the cloud for your project.

Arguments:
  [file]    Optional path to evaluation file (defaults to evals.json)

Flags:
  --force     Don't prompt for confirmation

Examples:
  agentuity eval create
  agentuity eval create evals.json
  agentuity eval create --force my-evals.json`,
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

		// Determine eval file path
		var evalFile string
		if len(args) > 0 {
			evalFile = args[0]
		} else {
			evalFile = filepath.Join(dir, "evals.json")
		}

		// Check if file exists
		if !sys.Exists(evalFile) {
			errsystem.New(errsystem.ErrInvalidCommandFlag, fmt.Errorf("evaluation file not found: %s", evalFile)).ShowErrorAndExit()
		}

		// Read evaluation data
		evalData, err := os.ReadFile(evalFile)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidCommandFlag, err, errsystem.WithUserMessage("Failed to read evaluation file")).ShowErrorAndExit()
		}

		// Validate JSON
		var evals interface{}
		if err := json.Unmarshal(evalData, &evals); err != nil {
			errsystem.New(errsystem.ErrInvalidCommandFlag, err, errsystem.WithUserMessage("Invalid JSON in evaluation file")).ShowErrorAndExit()
		}

		// Confirm create unless force flag is set
		if !force {
			if !tui.Ask(logger, fmt.Sprintf("Create evaluation from %s in the cloud?", evalFile), false) {
				tui.ShowWarning("cancelled")
				return
			}
		}

		action := func() {
			err := CreateEvaluation(ctx, logger, apiUrl, apiKey, theproject.ProjectId, string(evalData))
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to create evaluation")).ShowErrorAndExit()
			}
		}

		spinner.New().Title("Creating evaluation...").Action(action).Run()
		tui.ShowSuccess("Evaluation created successfully")
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
		apiUrl := context.APIURL
		apiKey := context.Token
		theproject := context.Project

		evalId := args[0]

		var evalData string
		action := func() {
			var err error
			evalData, err = PullEvaluation(ctx, logger, apiUrl, apiKey, theproject.ProjectId, evalId)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to pull evaluation")).ShowErrorAndExit()
			}
		}

		spinner.New().Title("Pulling evaluation...").Action(action).Run()

		// Output to stdout
		fmt.Println(evalData)
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
