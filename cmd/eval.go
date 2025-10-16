package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/eval"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluation related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func getEvalInfoFlow(logger logger.Logger, name string, description string) (string, string) {
	if name == "" {
		if !tui.HasTTY {
			logger.Fatal("No TTY detected, please specify an eval name from the command line")
		}
		name = tui.InputWithValidation(logger, "What should we name the evaluation?", "The name of the eval helps identify its purpose", 255, func(name string) error {
			if name == "" {
				return fmt.Errorf("Eval name cannot be empty")
			}
			return nil
		})
	}

	if description == "" {
		description = tui.Input(logger, "How should we describe what the "+name+" eval does?", "The description of the eval is optional but helpful for understanding its purpose")
	}

	return name, description
}

func generateEvalFile(logger logger.Logger, projectDir string, evalID string, slug string, name string, description string, isTypeScript bool) error {
	// Always generate TypeScript files for evals
	ext := ".ts"

	// Create evals directory if it doesn't exist
	evalsDir := filepath.Join(projectDir, "src", "evals")
	if err := os.MkdirAll(evalsDir, 0755); err != nil {
		return fmt.Errorf("failed to create evals directory: %w", err)
	}

	// Generate file path
	filename := filepath.Join(evalsDir, slug+ext)

	// Check if file already exists
	if util.Exists(filename) {
		return fmt.Errorf("eval file already exists: %s", filename)
	}

	// Generate TypeScript content with metadata
	content := fmt.Sprintf(`import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';

export const metadata = {
  id: '%s',
  slug: '%s',
  name: '%s',
  description: '%s'
};

/**
 * %s
 * %s
 */
export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  const { input, output } = req;

  // TODO: Implement your evaluation logic here
  // Example: Score the output based on some criteria
  
  const score = 0.8; // Replace with your actual scoring logic
  const metadata = {
    reasoning: 'Replace with your evaluation reasoning'
  };

  res.score(score, metadata);
}
`, evalID, slug, name, description, name, description)

	// Write file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write eval file: %w", err)
	}

	logger.Debug("Created eval file: %s", filename)
	return nil
}

var evalCreateCmd = &cobra.Command{
	Use:     "create [name] [description]",
	Short:   "Create a new evaluation function",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		theproject := project.EnsureProject(ctx, cmd)
		apikey := theproject.Token
		urls := util.GetURLs(logger)
		apiUrl := urls.API

		var name string
		var description string

		if len(args) > 0 {
			name = args[0]
		}

		if len(args) > 1 {
			description = args[1]
		}

		name, description = getEvalInfoFlow(logger, name, description)

		// Generate slug from name
		isPython := theproject.Project.Bundler.Language == "python"
		slug := util.SafeProjectFilename(strings.ToLower(name), isPython)

		action := func() {
			// Create eval via API
			evalID, err := eval.CreateEval(ctx, logger, apiUrl, apikey, theproject.Project.ProjectId, slug, name, description)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to create eval")).ShowErrorAndExit()
			}

			logger.Debug("Created eval with ID: %s", evalID)

			// Generate eval file (always TypeScript) with the real ID from API
			if err := generateEvalFile(logger, theproject.Dir, evalID, slug, name, description, true); err != nil {
				errsystem.New(errsystem.ErrOpenFile, err, errsystem.WithContextMessage("Failed to create eval file")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Creating evaluation ...", action)

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			result := map[string]string{
				"id":          "eval_" + slug,
				"slug":        slug,
				"name":        name,
				"description": description,
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			tui.ShowSuccess("Evaluation created successfully")
			fmt.Printf("\nFile created: %s\n", tui.Muted(fmt.Sprintf("src/evals/%s.ts", slug)))
		}
	},
}

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.AddCommand(evalCreateCmd)

	for _, cmd := range []*cobra.Command{evalCreateCmd} {
		cmd.Flags().StringP("dir", "d", "", "The project directory")
		cmd.Flags().String("format", "text", "The format to use for the output. Can be either 'text' or 'json'")
	}
}
