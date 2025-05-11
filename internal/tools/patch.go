package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
)

type genPatchInput struct{}

type genPatchOutput struct {
	Diff string `json:"diff"`
}

// GeneratePatch is a placeholder – execution simply echoes diff back.
func GeneratePatch() Tool {
	return Tool{
		Name:        "generate_patch",
		Description: "Return a unified diff patch proposal (LLM-only).",
		InputSchema: generateSchema[genPatchInput](),
		Exec: func(input json.RawMessage) (string, error) {
			// Simply echo back payload – allows display to user before apply.
			return string(input), nil
		},
	}
}

type applyPatchInput struct {
	Diff string `json:"diff" jsonschema_description:"Unified diff text to apply."`
}

type applyPatchOutput struct {
	Status string `json:"status"`
}

var diffFileRe = regexp.MustCompile(`(?m)^[+]{3} b/(.+)$`)

func ApplyPatch(root string) Tool {
	return Tool{
		Name:        "apply_patch",
		Description: "Apply a unified diff patch to the project (requires clean git repo).",
		InputSchema: generateSchema[applyPatchInput](),
		Exec: func(input json.RawMessage) (string, error) {
			var in applyPatchInput
			if err := json.Unmarshal(input, &in); err != nil {
				return "", err
			}
			if in.Diff == "" {
				return "", errors.New("diff required")
			}
			if len(in.Diff) > 5*1024 {
				return "", errors.New("diff too large")
			}

			// Validate file paths inside diff
			matches := diffFileRe.FindAllStringSubmatch(in.Diff, -1)
			for _, m := range matches {
				if len(m) < 2 {
					continue
				}
				if _, err := secureJoin(root, m[1]); err != nil {
					return "", errors.New("diff references path outside project")
				}
			}

			// Ensure we're in a git repo
			if err := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
				return "", errors.New("not a git repository")
			}

			// Apply patch (check first)
			check := exec.Command("git", "-C", root, "apply", "--check", "-")
			check.Stdin = bytes.NewBufferString(in.Diff)
			if err := check.Run(); err != nil {
				return "", errors.New("patch does not apply cleanly")
			}

			apply := exec.Command("git", "-C", root, "apply", "-")
			apply.Stdin = bytes.NewBufferString(in.Diff)
			if err := apply.Run(); err != nil {
				return "", err
			}

			resp, _ := json.Marshal(applyPatchOutput{Status: "ok"})
			return string(resp), nil
		},
	}
}
