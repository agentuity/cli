package tools

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

// Tool represents a single callable function exposed to the LLM.
// It mirrors anthropic.ToolParam.

type Tool struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	InputSchema anthropic.ToolInputSchemaParam `json:"input_schema"`
	Exec        func(input json.RawMessage) (string, error)
}

// generateSchema derives a JSON-schema from a Go struct with jsonschema.
func generateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return anthropic.ToolInputSchemaParam{Properties: schema.Properties}
}

// secureJoin joins base and relPath ensuring the result stays within base.
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
