package tools

import (
	"encoding/json"
	"errors"
	"os"
)

type readFileInput struct {
	Path string `json:"path" jsonschema_description:"Relative file path inside the project root."`
}

// FSRead returns the read_file tool confined to root.
func FSRead(root string) Tool {
	return Tool{
		Name:        "read_file",
		Description: "Read the contents of a file relative to the project root directory.",
		InputSchema: generateSchema[readFileInput](),
		Exec: func(input json.RawMessage) (string, error) {
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
			const maxLen = 16 * 1024
			if len(data) > maxLen {
				return string(data[:maxLen]) + "\n...[truncated]", nil
			}
			return string(data), nil
		},
	}
}
