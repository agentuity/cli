package errsystem

//go:generate go run ../../tools/generate_error_codes.go

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/agentuity/cli/internal/util"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type errorType struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errSystem struct {
	id         string
	code       errorType
	message    string
	err        error
	attributes map[string]any
	apiurl     string
}

type option func(*errSystem)

// New creates a new error.
func New(code errorType, err error, opts ...option) *errSystem {
	// if we get a context canceled error, we want to exit the program
	// instead of showing the error message since this is likely a user
	// interruption
	if errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
	var apiErr *util.APIError
	if errors.As(err, &apiErr) && apiErr != nil && errors.Is(apiErr.TheError, context.Canceled) {
		os.Exit(1)
	}
	res := &errSystem{
		id:         uuid.New().String(),
		err:        err,
		code:       code,
		attributes: make(map[string]any),
	}
	user_id := viper.GetString("auth.user_id")
	if user_id != "" {
		opts = append(opts, WithUserId(user_id))
	}
	res.apiurl = viper.GetString("overrides.api_url")
	if res.apiurl == "" {
		res.apiurl = "https://api.agentuity.com"
	}
	res.apiurl = util.TransformUrl(res.apiurl)
	for _, opt := range opts {
		opt(res)
	}
	return res
}

func (e *errSystem) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.err.Error())
}

// WithUserMessage adds a user-friendly message to the error.
func WithUserMessage(message string, args ...any) option {
	return func(e *errSystem) {
		e.message = fmt.Sprintf(message, args...)
	}
}

// WithAttributes adds additional metadata attributes to the error.
func WithAttributes(attributes map[string]any) option {
	return func(e *errSystem) {
		for k, v := range attributes {
			e.attributes[k] = v
		}
	}
}

// WithUserId adds the user ID to the error attributes.
func WithUserId(userId string) option {
	return func(e *errSystem) {
		e.attributes["user_id"] = userId
	}
}

// IsBreakingChangeError checks if an error is a breaking change error that should be handled gracefully
func IsBreakingChangeError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an errSystem error with breaking change codes
	if es, ok := err.(*errSystem); ok {
		return es.code.Code == "CLI-0030" || es.code.Code == "CLI-0031"
	}

	// Check if it's wrapped in an errSystem
	var es *errSystem
	if errors.As(err, &es) && es != nil {
		return es.code.Code == "CLI-0030" || es.code.Code == "CLI-0031"
	}

	return false
}

// WithProjectId adds the project ID to the error attributes.
func WithProjectId(projectId string) option {
	return func(e *errSystem) {
		e.attributes["project_id"] = projectId
	}
}

// WithContextMessage adds some internal context that can help with debugging.
func WithContextMessage(message string) option {
	return func(e *errSystem) {
		e.attributes["message"] = message
	}
}

// WithTraceID adds a trace ID to the error attributes.
func WithTraceID(traceID string) option {
	return func(e *errSystem) {
		e.attributes["trace_id"] = traceID
	}
}

// WithAPIURL allows the API URL to be overridden.
func WithAPIURL(apiurl string) option {
	return func(e *errSystem) {
		e.apiurl = apiurl
	}
}
