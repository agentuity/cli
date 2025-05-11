package codeagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/agentuity/cli/internal/tools"
	"github.com/agentuity/go-common/logger"
	"github.com/anthropics/anthropic-sdk-go"
)

// NOTE: I think we should be able to use that fancy go:embed thing here
// but the import gets nuked when we build the CLI, so doing this nasty
// init() thing instead.
var systemPrompt string

func init() {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Dir(file)
	p := filepath.Join(base, "./code-system-prompt.txt")
	if data, err := os.ReadFile(p); err == nil {
		systemPrompt = string(data)
	} else {
		systemPrompt = ""
	}
}

type Options struct {
	Dir           string
	Goal          string
	Logger        logger.Logger
	MaxIterations int
}

func Generate(ctx context.Context, opts Options) error {
	if opts.Dir == "" {
		return errors.New("codeagent: Dir must be provided")
	}
	if opts.Goal == "" {
		return errors.New("codeagent: Goal must be provided")
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 10
	}

	// Ensure absolute path for safety checks later.
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("codeagent: failed to resolve dir: %w", err)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return errors.New("codeagent: ANTHROPIC_API_KEY environment variable not set")
	}

	// Init client using default environment-based auth.
	client := anthropic.NewClient()

	// Build tool definitions.
	tk := []tools.Tool{
		tools.FSRead(absDir),
		tools.FSList(absDir),
		tools.FSEdit(absDir),
		tools.Grep(absDir),
		tools.GitDiff(absDir),
	}

	// Build initial conversation with the user's goal. System prompt is supplied separately.
	conversation := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(opts.Goal)),
	}

	for i := 0; i < opts.MaxIterations; i++ {
		// Prepare Anthropic tool schemas.
		var anthropicTools []anthropic.ToolUnionParam

		for _, t := range tk {
			anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(t.Description),
					InputSchema: t.InputSchema,
				},
			})
		}

		message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model: anthropic.ModelClaude3_7SonnetLatest,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages:  conversation,
			Tools:     anthropicTools,
			MaxTokens: int64(64000),
		})
		if err != nil {
			return fmt.Errorf("codeagent: LLM error: %w", err)
		}

		// Append assistant output.
		conversation = append(conversation, message.ToParam())

		// Collect tool results if any.
		var toolResults []anthropic.ContentBlockParamUnion
		for _, c := range message.Content {
			if c.Type != "tool_use" {
				continue
			}

			// Find tool.
			var tool *tools.Tool
			for i := range tk {
				if tk[i].Name == c.Name {
					tool = &tk[i]
					break
				}
			}
			if tool == nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, "tool not found", true))
				continue
			}

			// Execute.
			res, execErr := tool.Exec(c.Input)
			if execErr != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, execErr.Error(), true))
			} else {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, res, false))
			}
		}

		if len(toolResults) == 0 {
			// No more tool requests – stop.
			return nil
		}

		// Feed tool results back as a user message.
		conversation = append(conversation, anthropic.NewUserMessage(toolResults...))
	}

	return errors.New("codeagent: reached max iterations without convergence")
}
