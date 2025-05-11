package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

type grepInput struct {
	Pattern string `json:"pattern" jsonschema_description:"Regular expression pattern to search for."`
	Path    string `json:"path,omitempty" jsonschema_description:"Optional subdirectory to limit the search."`
}

type grepMatch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Grep creates a grep search tool (read-only).
func Grep(root string) Tool {
	return Tool{
		Name:        "grep_search",
		Description: "Regex search across files within the project root (caps 50 matches).",
		InputSchema: generateSchema[grepInput](),
		Exec: func(input json.RawMessage) (string, error) {
			var in grepInput
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			if in.Pattern == "" {
				return "", errors.New("pattern required")
			}
			re, err := regexp.Compile(in.Pattern)
			if err != nil {
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

			var matches []grepMatch
			walkFn := func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				// Simple line scanning
				lines := bytes.Split(data, []byte("\n"))
				for i, l := range lines {
					if re.Match(l) {
						rel, _ := filepath.Rel(root, path)
						matches = append(matches, grepMatch{File: rel, Line: i + 1, Text: string(l)})
						if len(matches) >= 50 {
							return fs.SkipDir
						}
					}
				}
				return nil
			}
			_ = filepath.WalkDir(start, walkFn)
			out, _ := json.Marshal(matches)
			return string(out), nil
		},
	}
}
