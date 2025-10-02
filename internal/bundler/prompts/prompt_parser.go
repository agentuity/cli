package prompts

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// PromptsYAML represents the structure of prompts.yaml for unmarshaling
type PromptsYAML struct {
	Prompts []Prompt `yaml:"prompts"`
}

// ParsePromptsYAML parses YAML bytes and returns the prompt definitions
func ParsePromptsYAML(data []byte) ([]Prompt, error) {
	var promptsData PromptsYAML
	if err := yaml.Unmarshal(data, &promptsData); err != nil {
		return nil, fmt.Errorf("failed to parse prompts.yaml: %w", err)
	}

	if len(promptsData.Prompts) == 0 {
		return nil, fmt.Errorf("no prompts found in prompts.yaml")
	}

	// Validate and parse templates for each prompt
	for i, prompt := range promptsData.Prompts {
		if prompt.Name == "" || prompt.Slug == "" {
			return nil, fmt.Errorf("prompt at index %d is missing required fields (name, slug)", i)
		}
		// At least one of system or prompt must be present
		if prompt.System == "" && prompt.Prompt == "" {
			return nil, fmt.Errorf("prompt at index %d must have at least one of system or prompt", i)
		}

		// Parse templates
		if prompt.System != "" {
			promptsData.Prompts[i].SystemTemplate = ParseTemplate(prompt.System)
		}
		if prompt.Prompt != "" {
			promptsData.Prompts[i].PromptTemplate = ParseTemplate(prompt.Prompt)
		}
	}

	return promptsData.Prompts, nil
}
