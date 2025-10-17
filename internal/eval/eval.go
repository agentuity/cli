package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type PromptData struct {
	Slug         string                 `json:"slug"`
	Compiled     string                 `json:"compiled"`
	Template     string                 `json:"template"`
	Variables    map[string]interface{} `json:"variables"`
	Evals        []string               `json:"evals"`
	TemplateHash string                 `json:"templateHash"`
	CompiledHash string                 `json:"compiledHash"`
}

type Span struct {
	SpanID    string       `json:"spanId"`
	SpanName  string       `json:"spanName"`
	Timestamp string       `json:"timestamp"`
	Duration  float64      `json:"duration"`
	Prompts   []PromptData `json:"prompts"`
	Input     string       `json:"input"`
	Output    string       `json:"output"`
}

type SessionData struct {
	SessionID string `json:"sessionId"`
	Spans     []Span `json:"spans"`
}

type SessionResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    SessionData `json:"data"`
}

// GetSessionPrompts fetches the prompts used in a session for eval purposes
// Retries up to 5 times with 1 minute wait between attempts if session data is not yet available
func GetSessionPrompts(ctx context.Context, logger logger.Logger, baseUrl string, token string, sessionID string) (*SessionData, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)
	// sleep for 1 second
	time.Sleep(15 * time.Second)

	maxAttempts := 5
	retryDelay := 1 * time.Minute

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		logger.Debug("fetching session prompts for %s (attempt %d/%d)", sessionID, attempt+1, maxAttempts)

		var resp SessionResponse
		err := client.Do("GET", fmt.Sprintf("/cli/evals/prompts/sess_%s", sessionID), nil, &resp)

		if err == nil && resp.Success {
			logger.Debug("successfully fetched session prompts on attempt %d", attempt+1)
			return &resp.Data, nil
		}

		if err != nil {
			lastErr = fmt.Errorf("error fetching session prompts: %s", err)
		} else {
			lastErr = fmt.Errorf("error fetching session prompts: %s", resp.Message)
		}

		// Log failure
		logger.Debug("attempt %d/%d failed: %v", attempt+1, maxAttempts, lastErr)

		// Wait before next retry (except after the last attempt)
		if attempt < maxAttempts-1 {
			logger.Debug("waiting %v before retry", retryDelay)

			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}

type RunEvalRequest struct {
	Input     string `json:"input"`
	Output    string `json:"output"`
	SessionID string `json:"sessionId"`
	SpanID    string `json:"spanId"`
	EvalName  string `json:"evalName"`
}

type EvalResult struct {
	SessionID  string                 `json:"sessionId"`
	ResultType string                 `json:"resultType"` // "pass", "fail", or "score"
	ScoreValue *float64               `json:"scoreValue,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// RunEval executes an eval function on the agent
func RunEval(ctx context.Context, logger logger.Logger, agentURL string, evalName string, evalID string, input string, output string, sessionID string, spanID string) (*EvalResult, error) {
	client := util.NewAPIClient(ctx, logger, agentURL, "")

	payload := RunEvalRequest{
		Input:     input,
		Output:    output,
		SessionID: sessionID,
		SpanID:    spanID,
		EvalName:  evalName,
	}

	var result EvalResult
	if err := client.Do("POST", fmt.Sprintf("/_agentuity/eval/%s", evalID), payload, &result); err != nil {
		return nil, fmt.Errorf("error running eval %s: %s", evalName, err)
	}

	return &result, nil
}

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EvalData struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type EvalMetadata struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LoadEvalMetadataMap scans the evals directory and builds a map of slug -> eval ID
func LoadEvalMetadataMap(logger logger.Logger, projectDir string) (map[string]string, error) {
	evalsDir := filepath.Join(projectDir, "src", "evals")

	// Check if evals directory exists
	if !util.Exists(evalsDir) {
		logger.Debug("evals directory not found: %s", evalsDir)
		return make(map[string]string), nil
	}

	files, err := os.ReadDir(evalsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read evals directory: %w", err)
	}

	slugToIDMap := make(map[string]string)
	metadataRegex := regexp.MustCompile(`export\s+const\s+metadata\s+=\s+({[^}]+})`)

	for _, file := range files {
		ext := filepath.Ext(file.Name())
		if file.IsDir() || (ext != ".ts" && ext != ".js") {
			continue
		}

		// Skip index files
		if file.Name() == "index.ts" || file.Name() == "index.js" {
			continue
		}

		filePath := filepath.Join(evalsDir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("failed to read eval file %s: %v", file.Name(), err)
			continue
		}

		// Extract metadata using regex
		matches := metadataRegex.FindSubmatch(content)
		if len(matches) < 2 {
			logger.Debug("no metadata found in eval file: %s", file.Name())
			continue
		}

		// Parse the metadata JSON
		var metadata EvalMetadata
		// Clean up the extracted JSON to make it valid
		metadataStr := string(matches[1])
		// Replace single quotes with double quotes for valid JSON
		metadataStr = regexp.MustCompile(`'([^']*)'`).ReplaceAllString(metadataStr, `"$1"`)

		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			logger.Warn("failed to parse metadata from %s: %v", file.Name(), err)
			continue
		}

		if metadata.Slug != "" && metadata.ID != "" {
			slugToIDMap[metadata.Slug] = metadata.ID
			logger.Debug("mapped eval slug '%s' to ID '%s'", metadata.Slug, metadata.ID)
		}
	}

	logger.Debug("loaded %d eval mappings", len(slugToIDMap))
	return slugToIDMap, nil
}

// CreateEval creates a new evaluation function in the project
func CreateEval(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, slug string, name string, description string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	payload := map[string]any{
		"projectId":   projectId,
		"slug":        slug,
		"name":        name,
		"description": description,
	}

	var resp Response[EvalData]
	if err := client.Do("POST", "/cli/evals", payload, &resp); err != nil {
		return "", fmt.Errorf("error creating eval: %s", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("error creating eval: %s", resp.Message)
	}
	return resp.Data.ID, nil
}
