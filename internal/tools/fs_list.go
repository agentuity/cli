package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type listFilesInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from."`
}

func FSList(root string) Tool {
	return Tool{
		Name:        "list_files",
		Description: "Recursively list files/directories relative to the project root directory.",
		InputSchema: generateSchema[listFilesInput](),
		Exec: func(input json.RawMessage) (string, error) {
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
			out, _ := json.Marshal(files)
			return string(out), nil
		},
	}
}
