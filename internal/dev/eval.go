package dev

import (
	"context"
	"fmt"

	"github.com/agentuity/cli/internal/eval"
	"github.com/agentuity/cli/internal/gravity"
	"github.com/agentuity/go-common/logger"
)

// EvalProcessor handles eval processing for dev mode
type EvalProcessor struct {
	logger          logger.Logger
	apiUrl          string
	apiKey          string
	agentPort       int
	evalMetadataMap map[string]string
}

// NewEvalProcessor creates a new eval processor
func NewEvalProcessor(logger logger.Logger, apiUrl, apiKey string, agentPort int, evalMetadataMap map[string]string) *EvalProcessor {
	return &EvalProcessor{
		logger:          logger,
		apiUrl:          apiUrl,
		apiKey:          apiKey,
		agentPort:       agentPort,
		evalMetadataMap: evalMetadataMap,
	}
}

// StartEvalProcessor starts the eval processor goroutine
func (ep *EvalProcessor) StartEvalProcessor(ctx context.Context, evalChannel <-chan gravity.EvalInfo) {
	go func() {
		for evalInfo := range evalChannel {
			go ep.processEvalSession(ctx, evalInfo.SessionID)
		}
	}()
}

// processEvalSession processes evals for a specific session
func (ep *EvalProcessor) processEvalSession(ctx context.Context, sessionID string) {
	ep.logger.Debug("processing evals for session %s", sessionID)

	// Fetch session data from API
	sessionData, err := eval.GetSessionPrompts(ctx, ep.logger, ep.apiUrl, ep.apiKey, sessionID)
	if err != nil {
		ep.logger.Error("failed to fetch session prompts: %s", err)
		return
	}

	// Process each span
	for _, span := range sessionData.Spans {
		ep.logger.Debug("running evals for span %s in session %s", span.SpanID, sessionID)

		// Process each prompt in the span
		for _, prompt := range span.Prompts {
			ep.logger.Debug("running evals for prompt %s in session %s", prompt.Slug, sessionID)

			// Run each eval specified for this prompt
			for _, evalSlug := range prompt.Evals {
				ep.runEvalForPrompt(ctx, evalSlug, prompt.Slug, span, sessionID)
			}
		}
	}
}

// runEvalForPrompt runs a specific eval for a prompt
func (ep *EvalProcessor) runEvalForPrompt(ctx context.Context, evalSlug, promptSlug string, span eval.Span, sessionID string) {
	// Map slug to eval ID using metadata, fallback to slug if not found
	evalID := evalSlug
	if mappedID, ok := ep.evalMetadataMap[evalSlug]; ok {
		evalID = mappedID
		ep.logger.Debug("mapped eval slug '%s' to ID '%s'", evalSlug, evalID)
	} else {
		ep.logger.Debug("no mapping found for eval slug '%s', using slug as ID", evalSlug)
	}

	ep.logger.Debug("running eval %s (ID: %s) for prompt %s in session %s", evalSlug, evalID, promptSlug, sessionID)

	agentURL := fmt.Sprintf("http://127.0.0.1:%d", ep.agentPort)
	result, err := eval.RunEval(ctx, ep.logger, agentURL, evalSlug, evalID, span.Input, span.Output, sessionID, span.SpanID)
	if err != nil {
		ep.logger.Error("failed to run eval %s: %s", evalSlug, err)
		return
	}

	// Log the eval result
	ep.logEvalResult(evalSlug, result)
}

// logEvalResult logs the result of an eval
func (ep *EvalProcessor) logEvalResult(evalSlug string, result *eval.EvalResult) {
	if result.ScoreValue != nil {
		ep.logger.Info("✓ Eval %s: %s (score: %.2f)", evalSlug, result.ResultType, *result.ScoreValue)
	} else {
		ep.logger.Info("✓ Eval %s: %s", evalSlug, result.ResultType)
	}

	if result.Metadata != nil {
		if reasoning, ok := result.Metadata["reasoning"].(string); ok && reasoning != "" {
			ep.logger.Debug("  Reasoning: %s", reasoning)
		}
	}
}
