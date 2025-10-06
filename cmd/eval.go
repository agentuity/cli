package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
	"github.com/spf13/cobra"
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run evaluations on session data",
	Long: `Run evaluations on session data using the eval runner.

This command allows you to run evaluations on specific sessions or spans.
You can specify which evaluation handler to use and configure ClickHouse connection details.

Examples:
  agentuity eval run --session-id=abc123 --eval=./src/evals/politeness.ts
  agentuity eval run --span-id=span456 --eval=./src/evals/politeness.ts --clickhouse-host=localhost`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var evalRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an evaluation on a session or span",
	Long: `Run an evaluation on a specific session or span.

This command will:
1. Fetch the session/span data from ClickHouse
2. Run the specified evaluation handler
3. Store the results back to ClickHouse

Examples:
  agentuity eval run --session-id=abc123 --eval=./src/evals/politeness.ts
  agentuity eval run --span-id=span456 --eval=./src/evals/politeness.ts`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		started := time.Now()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		projectContext := project.EnsureProject(ctx, cmd)

		// Get flags
		sessionId, _ := cmd.Flags().GetString("session-id")
		spanId, _ := cmd.Flags().GetString("span-id")
		evalPath, _ := cmd.Flags().GetString("eval")
		clickhouseHost, _ := cmd.Flags().GetString("clickhouse-host")
		clickhouseUser, _ := cmd.Flags().GetString("clickhouse-user")
		clickhousePassword, _ := cmd.Flags().GetString("clickhouse-password")

		// Validate required flags
		if sessionId == "" && spanId == "" {
			errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("either --session-id or --span-id must be provided"), errsystem.WithContextMessage("Missing required parameter")).ShowErrorAndExit()
		}

		if evalPath == "" {
			errsystem.New(errsystem.ErrInvalidConfiguration, fmt.Errorf("--eval path is required"), errsystem.WithContextMessage("Missing required parameter")).ShowErrorAndExit()
		}

		// Set defaults for ClickHouse
		if clickhouseHost == "" {
			clickhouseHost = "http://localhost:9000"
		}
		if clickhouseUser == "" {
			clickhouseUser = "default"
		}
		if clickhousePassword == "" {
			clickhousePassword = ""
		}

		// Run the eval using Node.js
		if err := runEvalWithNode(ctx, projectContext.Logger, projectContext.Dir, sessionId, spanId, evalPath, clickhouseHost, clickhouseUser, clickhousePassword); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run evaluation")).ShowErrorAndExit()
		}

		projectContext.Logger.Info("Evaluation completed in %s", time.Since(started))
	},
}

func runEvalWithNode(ctx context.Context, logger logger.Logger, projectDir, sessionId, spanId, evalPath, clickhouseHost, clickhouseUser, clickhousePassword string) error {
	// Check if eval-runner.ts exists
	evalRunnerPath := "eval-runner.ts"
	if _, err := os.Stat(filepath.Join(projectDir, evalRunnerPath)); os.IsNotExist(err) {
		// Create eval-runner.ts if it doesn't exist
		if err := createEvalRunner(projectDir); err != nil {
			return fmt.Errorf("failed to create eval-runner.ts: %w", err)
		}
	}

	// Build arguments for the eval runner
	args := []string{"tsx", evalRunnerPath, "run"}

	if sessionId != "" {
		args = append(args, "--session-id", sessionId)
	}
	if spanId != "" {
		args = append(args, "--span-id", spanId)
	}
	args = append(args, "--eval-path", evalPath)
	args = append(args, "--clickhouse-host", clickhouseHost)
	args = append(args, "--clickhouse-user", clickhouseUser)
	args = append(args, "--clickhouse-password", clickhousePassword)

	// Run the eval runner
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Debug("Running eval with command: npx %s", strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("eval runner failed: %w", err)
	}

	return nil
}

func createEvalRunner(projectDir string) error {
	evalRunnerContent := `#!/usr/bin/env node

import { EvalAPI } from '@agentuity/sdk';
import { Command } from 'commander';

const program = new Command();

program
  .name('eval-runner')
  .description('Run evaluations on session data')
  .version('1.0.0');

program
  .command('run')
  .description('Run an evaluation on a session or span')
  .option('--session-id <id>', 'Session ID to evaluate')
  .option('--span-id <id>', 'Span ID to evaluate')
  .option('--eval-path <path>', 'Path to evaluation handler')
  .option('--clickhouse-host <host>', 'ClickHouse host', 'http://localhost:9000')
  .option('--clickhouse-user <user>', 'ClickHouse user', 'default')
  .option('--clickhouse-password <password>', 'ClickHouse password', '')
  .action(async (options) => {
    const { sessionId, spanId, evalPath, clickhouseHost, clickhouseUser, clickhousePassword } = options;
    
    if (!sessionId && !spanId) {
      console.error('Error: Either --session-id or --span-id must be provided');
      process.exit(1);
    }
    
    if (!evalPath) {
      console.error('Error: --eval-path is required');
      process.exit(1);
    }
    
    try {
      const evalAPI = new EvalAPI({
        clickhouseHost,
        clickhouseUser,
        clickhousePassword,
        spansTable: 'spans',
        resultsTable: 'eval_results',
      });
      
      // Create results table if it doesn't exist
      await evalAPI.createResultsTable();
      
      // Run the evaluation
      const targetId = sessionId || spanId;
      const result = await evalAPI.runEval(targetId, evalPath);
      
      console.log('Evaluation completed successfully!');
      console.log('Result:', result);
      
    } catch (error) {
      console.error('Error running evaluation:', error);
      process.exit(1);
    }
  });

program.parse();
`

	evalRunnerPath := filepath.Join(projectDir, "eval-runner.ts")
	return os.WriteFile(evalRunnerPath, []byte(evalRunnerContent), 0755)
}

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.AddCommand(evalRunCmd)

	// Add flags for eval run command
	evalRunCmd.Flags().String("session-id", "", "Session ID to evaluate")
	evalRunCmd.Flags().String("span-id", "", "Span ID to evaluate")
	evalRunCmd.Flags().String("eval", "", "Path to evaluation handler (required)")
	evalRunCmd.Flags().String("clickhouse-host", "http://localhost:9000", "ClickHouse host")
	evalRunCmd.Flags().String("clickhouse-user", "default", "ClickHouse user")
	evalRunCmd.Flags().String("clickhouse-password", "", "ClickHouse password")

	// Mark eval as required
	evalRunCmd.MarkFlagRequired("eval")
}
