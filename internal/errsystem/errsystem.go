package errsystem

//go:generate go run ../../tools/generate_error_codes.go

import (
	"fmt"

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
	for _, opt := range opts {
		opt(res)
	}
	return res
}

func (e *errSystem) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.err.Error())
}

// WithUserMessage adds a user-friendly message to the error.
func WithUserMessage(message string) option {
	return func(e *errSystem) {
		e.message = message
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
