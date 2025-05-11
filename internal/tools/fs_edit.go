package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type editFileInput struct {
	Path   string `json:"path" jsonschema_description:"File path to edit or create."`
	OldStr string `json:"old_str" jsonschema_description:"Exact text to replace (optional)."`
	NewStr string `json:"new_str" jsonschema_description:"Replacement text (required)."`
}

func FSEdit(root string) Tool {
	return Tool{
		Name:        "edit_file",
		Description: "Replace occurrences of old_str with new_str or create a new file with new_str if old_str empty.",
		InputSchema: generateSchema[editFileInput](),
		Exec: func(input json.RawMessage) (string, error) {
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
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return "", err
			}
			if in.OldStr == "" {
				if err := os.WriteFile(abs, []byte(in.NewStr), 0644); err != nil {
					return "", err
				}
				return "OK", nil
			}
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
		},
	}
}
