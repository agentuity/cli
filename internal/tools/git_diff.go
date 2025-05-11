package tools

import (
	"bytes"
	"encoding/json"
	"os/exec"
)

type gitDiffInput struct{}

type gitDiffOutput struct {
	Diff string `json:"diff"`
}

func GitDiff(root string) Tool {
	return Tool{
		Name:        "git_diff",
		Description: "Return the git diff (unstaged changes) for the project, truncated to 5KB.",
		InputSchema: generateSchema[gitDiffInput](),
		Exec: func(input json.RawMessage) (string, error) {
			cmd := exec.Command("git", "-C", root, "diff", "--no-color")
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				return "", err
			}
			data := out.Bytes()
			const max = 5 * 1024
			if len(data) > max {
				data = append(data[:max], []byte("\n...[truncated]")...)
			}
			enc, _ := json.Marshal(gitDiffOutput{Diff: string(data)})
			return string(enc), nil
		},
	}
}
