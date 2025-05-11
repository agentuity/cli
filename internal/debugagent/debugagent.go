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
	p := filepath.Join(base, "./debug-system-prompt.txt")
	if data, err := os.ReadFile(p); err == nil {
		systemPrompt = string(data)
	}
}

type Options struct {
	Dir           string
	Error         string
	Extra         string
	AllowWrites   bool
	Logger        logger.Logger
	MaxIterations int
}

type Result struct {
	Analysis string
	Patch    string // empty if no patch proposed
	Edited   bool
}

// cacheEntry stored on disk
type cacheEntry struct {
	Analysis string `json:"analysis"`
}

func Analyze(ctx context.Context, opts Options) (Result, error) {
	if opts.Dir == "" {
		return Result{}, errors.New("debugagent: Dir must be provided")
	}
	if opts.Error == "" {
		return Result{}, errors.New("debugagent: Error must be provided")
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 8
	}

	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("debugagent: failed to resolve dir: %w", err)
	}

	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return Result{}, errors.New("debugagent: ANTHROPIC_API_KEY env var not set")
	}

	// ----- Cache Check -----
	// We only use the cache for read-only analyses (no writes, no extra guidance).
	useCache := !opts.AllowWrites && opts.Extra == ""

	const cacheTTL = 24 * time.Hour
	cacheDir := filepath.Join(opts.Dir, ".agentcache")
	if useCache {
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
					var ce cacheEntry
					if json.Unmarshal(data, &ce) == nil && ce.Analysis != "" {
						return Result{Analysis: ce.Analysis, Patch: ""}, nil
					}
					// legacy plain-text
					return Result{Analysis: string(data), Patch: ""}, nil
				}
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			// non-fatal
			opts.Logger.Warn("debugagent: cache stat error: %v", err)
		}
	}

	client := anthropic.NewClient()

	// Tools: read-only set
	tk := []tools.Tool{
		tools.FSRead(absDir),
		tools.FSList(absDir),
		tools.Grep(absDir),
		tools.GitDiff(absDir),
	}
	if opts.AllowWrites {
		tk = append(tk, tools.FSEdit(absDir))
	}

	const maxErr = 8000
	errSnippet := opts.Error
	if len(errSnippet) > maxErr {
		errSnippet = errSnippet[:maxErr] + "\n...[truncated]"
	}
	conversation := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("Here is the error I saw while running the dev server:\n\n%s", errSnippet))),
	}
	if opts.Extra != "" {
		conversation = append(conversation, anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("Additional guidance from user:\n\n%s", opts.Extra))))
	}

	// Prune conversation to avoid exceeding token limits. Keep initial
	// context (first two messages) and the most recent 28 exchanges.
	const keepRecent = 28
	if len(conversation) > 2+keepRecent {
		head := conversation[:2]
		tail := conversation[len(conversation)-keepRecent:]
		conversation = append(head, tail...)
	}

	var lastMsg *anthropic.Message
	var edited bool
	for i := 0; i < opts.MaxIterations; i++ {
		// Map tools to anthropic schema.
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

		// Apply the same pruning rule before each LLM call as well.
		if len(conversation) > 2+keepRecent {
			head := conversation[:2]
			tail := conversation[len(conversation)-keepRecent:]
			conversation = append(head, tail...)
		}

		message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaude3_7SonnetLatest,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  conversation,
			Tools:     anthropicTools,
			MaxTokens: int64(64000),
		})
		if err != nil {
			return Result{}, fmt.Errorf("debugagent: LLM error: %w", err)
		}

		conversation = append(conversation, message.ToParam())
		lastMsg = message

		var toolResults []anthropic.ContentBlockParamUnion
		for _, c := range message.Content {
			if c.Type != "tool_use" {
				continue
			}
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
			res, execErr := tool.Exec(c.Input)
			if tool.Name == "edit_file" && execErr == nil {
				edited = true
			}
			if execErr != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, execErr.Error(), true))
			} else {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(c.ID, res, false))
			}
		}

		if len(toolResults) == 0 {
			// No more tool requests – return assistant text.
			analysis := collectAssistantResponse(message)
			if useCache {
				keyHash := hash(opts.Error)
				cachePath := filepath.Join(cacheDir, keyHash+".txt")
				_ = writeCache(cachePath, cacheEntry{Analysis: analysis})
			}
			return Result{Analysis: analysis, Patch: "", Edited: edited}, nil
		}

		conversation = append(conversation, anthropic.NewUserMessage(toolResults...))
	}

	if lastMsg != nil {
		analysis := collectAssistantResponse(lastMsg)
		if useCache {
			keyHash := hash(opts.Error)
			cachePath := filepath.Join(cacheDir, keyHash+".txt")
			_ = writeCache(cachePath, cacheEntry{Analysis: analysis})
		}
		return Result{Analysis: analysis, Patch: "", Edited: edited}, nil
	}

	return Result{}, errors.New("debugagent: reached max iterations without convergence")
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

func writeCache(path string, ce cacheEntry) error {
	data, err := json.Marshal(ce)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
