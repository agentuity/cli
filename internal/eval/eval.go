package eval

import (
	"context"
	"fmt"
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
	time.Sleep(30 * time.Second)

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
}

type EvalResult struct {
	SessionID  string                 `json:"sessionId"`
	ResultType string                 `json:"resultType"` // "pass", "fail", or "score"
	ScoreValue *float64               `json:"scoreValue,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// RunEval executes an eval function on the agent
func RunEval(ctx context.Context, logger logger.Logger, agentURL string, evalName string, input string, output string, sessionID string) (*EvalResult, error) {
	client := util.NewAPIClient(ctx, logger, agentURL, "")

	payload := RunEvalRequest{
		Input:     input,
		Output:    output,
		SessionID: sessionID,
	}

	var result EvalResult
	if err := client.Do("POST", fmt.Sprintf("/eval/%s", evalName), payload, &result); err != nil {
		return nil, fmt.Errorf("error running eval %s: %s", evalName, err)
	}

	return &result, nil
}
