package debugagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentuity/go-common/logger"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

var systemPrompt string

func init() {
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Dir(file)
	p := filepath.Join(base, "./debug-system-prompt.txt")
	if data, err := os.ReadFile(p); err == nil {
		systemPrompt = string(data)
	}
}

// Options controls the debug analysis session.
// Dir: project root.
// Error: the raw error snippet that triggered the debug session.
// MaxIterations: LLM tool loop iterations (default 8).
// Logger: std Agentuity logger.

type Options struct {
	Dir           string
	Error         string
	Logger        logger.Logger
	MaxIterations int
}

// Analyze runs the debug agent loop and returns the assistant's final response
// (natural-language analysis & suggestions). It does not modify files.

func Analyze(ctx context.Context, opts Options) (string, error) {
	if opts.Dir == "" {
		return "", errors.New("debugagent: Dir must be provided")
	}
	if opts.Error == "" {
		return "", errors.New("debugagent: Error must be provided")
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 8
	}

	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return "", fmt.Errorf("debugagent: failed to resolve dir: %w", err)
	}

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return "", errors.New("debugagent: ANTHROPIC_API_KEY env var not set")
	}

	// ----- Cache Check -----
	const cacheTTL = 24 * time.Hour
	cacheDir := filepath.Join(opts.Dir, ".agentcache")
	_ = os.MkdirAll(cacheDir, 0o755)

	// Add cache dir to .gitignore if inside project git repo
	giPath := filepath.Join(opts.Dir, ".gitignore")
	if data, err := os.ReadFile(giPath); err == nil {
		if !strings.Contains(string(data), ".agentcache") {
			_ = os.WriteFile(giPath, append(data, []byte("\n# Agentuity cache\n.agentcache/\n")...), 0644)
		}
	}

	keyHash := hash(opts.Error)
	cachePath := filepath.Join(cacheDir, keyHash+".txt")
	if fi, err := os.Stat(cachePath); err == nil {
		if time.Since(fi.ModTime()) < cacheTTL {
			if data, err := os.ReadFile(cachePath); err == nil {
				return string(data), nil
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		// non-fatal
		opts.Logger.Warn("debugagent: cache stat error: %v", err)
	}

	client := anthropic.NewClient()

	// Tools: read_file & list_files only.
	tools := []ToolDefinition{
		readFileDefinition(absDir),
		listFilesDefinition(absDir),
	}

	const maxErr = 8000
	errSnippet := opts.Error
	if len(errSnippet) > maxErr {
		errSnippet = errSnippet[:maxErr] + "\n...[truncated]"
	}
	conversation := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("Here is the error I saw while running the dev server:\n\n%s", errSnippet))),
	}

	var lastMsg *anthropic.Message
	for i := 0; i < opts.MaxIterations; i++ {
		// Map tools to anthropic schema.
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
			Model:     anthropic.ModelClaude3_7SonnetLatest,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  conversation,
			Tools:     anthropicTools,
			MaxTokens: int64(64000),
		})
		if err != nil {
			return "", fmt.Errorf("debugagent: LLM error: %w", err)
		}

		conversation = append(conversation, message.ToParam())
		lastMsg = message

		var toolResults []anthropic.ContentBlockParamUnion
		for _, c := range message.Content {
			if c.Type != "tool_use" {
				continue
			}
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
			res, execErr := tool.Function(c.Input)
			if execErr != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, execErr.Error(), true))
			} else {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, res, false))
			}
		}

		if len(toolResults) == 0 {
			// No more tool requests – return assistant text.
			analysis := collectAssistantResponse(message)
			// write cache (best effort)
			_ = os.WriteFile(cachePath, []byte(analysis), 0o644)
			return analysis, nil
		}

		conversation = append(conversation, anthropic.NewUserMessage(toolResults...))
	}

	if lastMsg != nil {
		analysis := collectAssistantResponse(lastMsg)
		_ = os.WriteFile(cachePath, []byte(analysis), 0o644)
		return analysis, nil
	}

	return "", errors.New("debugagent: reached max iterations without convergence")
}

func collectAssistantResponse(msg *anthropic.Message) string {
	var parts []string
	for _, c := range msg.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

/* ----------------- tool layer (copied from codeagent, minus edit) --------- */

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
	return anthropic.ToolInputSchemaParam{Properties: schema.Properties}
}

// read_file implementation

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
		const maxLen = 16384 // 16 KiB
		if len(data) > maxLen {
			return string(data[:maxLen]) + "\n...[truncated]", nil
		}
		return string(data), nil
	}
}

// list_files implementation

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
		const maxFiles = 50
		if len(files) > maxFiles {
			files = append(files[:maxFiles], "...etc (truncated)")
		}
		out, _ := json.Marshal(files)
		return string(out), nil
	}
}

// secureJoin duplicated (private in codeagent)
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

// hash generates a stable hex hash for cache keys.
func hash(s string) string {
	var h uint64 = 14695981039346656037
	const prime uint64 = 1099511628211
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return fmt.Sprintf("%x", h)
}
