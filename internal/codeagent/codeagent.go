package codeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

// Just reading this in for the system prompt for now.
var systemPrompt string

func init() {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Dir(file)
	p := filepath.Join(base, "../../docs/code-system-prompt.txt")
	if data, err := os.ReadFile(p); err == nil {
		systemPrompt = string(data)
	} else {
		systemPrompt = ""
	}
}

// Options encapsulates configuration for the Generate routine.
// Dir must point at the root directory that contains the freshly-scaffolded
// Agent source (e.g. /path/to/project/src/agents/myagent).
// Goal is the free-form user description of what the Agent should do.
// MaxIterations controls how many request/response tool loops the agent can perform.
// Logger is the standard Agentuity logger.
type Options struct {
	Dir           string
	Goal          string
	Logger        logger.Logger
	MaxIterations int
}

// Generate takes the scaffold located at opts.Dir and applies LLM-driven edits so
// that the skeleton reflects the user-provided Goal.  It does this by running a
// minimal RAG-style tool-calling loop with Claude 3.  All file edits are scoped
// inside opts.Dir.
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
	tools := []ToolDefinition{
		readFileDefinition(absDir),
		listFilesDefinition(absDir),
		editFileDefinition(absDir),
	}

	// Build initial conversation with the user's goal. System prompt is supplied separately.
	conversation := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(opts.Goal)),
	}

	for i := 0; i < opts.MaxIterations; i++ {
		// Prepare Anthropic tool schemas.
		var anthropicTools []anthropic.ToolUnionParam

		for _, t := range tools {
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
			var tool *ToolDefinition
			for _, t := range tools {
				if t.Name == c.Name {
					tool = &t
					break
				}
			}
			if tool == nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, "tool not found", true))
				continue
			}

			// Execute.
			res, execErr := tool.Function(c.Input)
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

/* -------------------------------------------------------------------------- */
/*                                Tool layer                                  */
/* -------------------------------------------------------------------------- */

type ToolDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	InputSchema anthropic.ToolInputSchemaParam `json:"input_schema"`
	Function    func(input json.RawMessage) (string, error)
}

// Schema helper.
func generateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

/* ------------------------------ read_file --------------------------------- */

type readFileInput struct {
	Path string `json:"path" jsonschema_description:"Relative file path inside the agent directory."`
}

func readFileDefinition(root string) ToolDefinition {
	return ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file relative to the agent root directory.",
		InputSchema: generateSchema[readFileInput](),
		Function:    makeReadFileFunc(root),
	}
}

func makeReadFileFunc(root string) func(input json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var in readFileInput
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
		if in.Path == "" {
			return "", errors.New("path is required")
		}
		abs, err := secureJoin(root, in.Path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

/* ------------------------------ list_files -------------------------------- */

type listFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from."`
}

func listFilesDefinition(root string) ToolDefinition {
	return ToolDefinition{
		Name:        "list_files",
		Description: "Recursively list files/directories relative to the agent root directory.",
		InputSchema: generateSchema[listFilesInput](),
		Function:    makeListFilesFunc(root),
	}
}

func makeListFilesFunc(root string) func(input json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var in listFilesInput
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
		start := root
		if in.Path != "" {
			p, err := secureJoin(root, in.Path)
			if err != nil {
				return "", err
			}
			start = p
		}
		var files []string
		err := filepath.Walk(start, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			if info.IsDir() {
				files = append(files, rel+"/")
			} else {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(files)
		return string(out), nil
	}
}

/* ------------------------------ edit_file --------------------------------- */

type editFileInput struct {
	Path   string `json:"path" jsonschema_description:"File path to edit or create."`
	OldStr string `json:"old_str" jsonschema_description:"Exact text to replace (optional)."`
	NewStr string `json:"new_str" jsonschema_description:"Replacement text (required)."`
}

func editFileDefinition(root string) ToolDefinition {
	return ToolDefinition{
		Name:        "edit_file",
		Description: "Replace occurrences of old_str with new_str or create a new file with new_str if old_str empty.",
		InputSchema: generateSchema[editFileInput](),
		Function:    makeEditFileFunc(root),
	}
}

func makeEditFileFunc(root string) func(input json.RawMessage) (string, error) {
	return func(input json.RawMessage) (string, error) {
		var in editFileInput
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
		if in.Path == "" || in.NewStr == "" {
			return "", errors.New("path and new_str are required")
		}
		abs, err := secureJoin(root, in.Path)
		if err != nil {
			return "", err
		}
		// Ensure directory exists.
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return "", err
		}
		// If old_str empty, overwrite/create.
		if in.OldStr == "" {
			if err := os.WriteFile(abs, []byte(in.NewStr), 0644); err != nil {
				return "", err
			}
			return "OK", nil
		}
		// Read file.
		content, err := os.ReadFile(abs)
		if err != nil {
			return "", err
		}
		updated := strings.ReplaceAll(string(content), in.OldStr, in.NewStr)
		if updated == string(content) {
			return "", errors.New("old_str not found")
		}
		if err := os.WriteFile(abs, []byte(updated), 0644); err != nil {
			return "", err
		}
		return "OK", nil
	}
}

/* -------------------------------------------------------------------------- */
/*                              helper functions                              */
/* -------------------------------------------------------------------------- */

// secureJoin joins base and relPath while preventing path traversal outside base.
func secureJoin(base, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", errors.New("absolute paths are not allowed")
	}
	p := filepath.Clean(filepath.Join(base, relPath))
	if !strings.HasPrefix(p, base) {
		return "", errors.New("invalid path – outside root")
	}
	return p, nil
}
