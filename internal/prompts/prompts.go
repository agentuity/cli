package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"gopkg.in/yaml.v3"
)

type Prompt struct {
	Slug        string   `yaml:"slug" json:"slug"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	System      string   `yaml:"system,omitempty" json:"system,omitempty"`
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Eval        []string `yaml:"eval,omitempty" json:"eval,omitempty"`
}

type PromptsFile struct {
	Prompts []Prompt `yaml:"prompts" json:"prompts"`
}

type PromptRequest struct {
	Slug    string                 `json:"slug"`
	Content map[string]interface{} `json:"content"`
}

type PromptsAPIRequest struct {
	Prompts []PromptRequest `json:"prompts"`
}

// ProcessPrompts reads the single prompts.yaml file from .agentuity bundle and sends it to the API
func ProcessPrompts(ctx context.Context, logger logger.Logger, client *util.APIClient, projectDir string, deploymentId string) error {
	// Look for the single prompts.yaml file that was copied by bundler
	promptsFile := filepath.Join(projectDir, ".agentuity", "src", "prompts.yaml")
	if !util.Exists(promptsFile) {
		logger.Debug("no prompts.yaml found in bundle, skipping prompts processing")
		return nil
	}

	prompts, err := ParsePromptsFile(promptsFile)
	if err != nil {
		return fmt.Errorf("failed to parse prompts.yaml: %w", err)
	}

	if len(prompts) == 0 {
		logger.Debug("no prompts found in prompts.yaml, skipping prompts processing")
		return nil
	}

	// Validate prompts have required fields
	for _, prompt := range prompts {
		if prompt.Slug == "" {
<<<<<<< HEAD
			return fmt.Errorf("prompt missing required 'id' field")
=======
			return fmt.Errorf("prompt missing required 'slug' field")
>>>>>>> ec4370965ca48f3091d7fb35b2711a6709557677
		}
		if prompt.Name == "" {
			return fmt.Errorf("prompt '%s' missing required 'name' field", prompt.Slug)
		}
<<<<<<< HEAD
		// Either / or
		if prompt.System == "" {
			return fmt.Errorf("prompt '%s' missing required 'system' field", prompt.Slug)
		}
		if prompt.Prompt == "" {
			return fmt.Errorf("prompt '%s' missing required 'prompt' field", prompt.Slug)
=======
		if prompt.System == "" && prompt.Prompt == "" {
			return fmt.Errorf("prompt '%s' must have either 'system' or 'prompt' field", prompt.Slug)
>>>>>>> ec4370965ca48f3091d7fb35b2711a6709557677
		}
	}

	// Convert to API request format
	var apiRequest PromptsAPIRequest
	for _, prompt := range prompts {
		// Convert prompt to map for JSON serialization
		contentBytes, err := json.Marshal(prompt)
		if err != nil {
			return fmt.Errorf("failed to marshal prompt %s: %w", prompt.Slug, err)
		}

		var content map[string]interface{}
		if err := json.Unmarshal(contentBytes, &content); err != nil {
			return fmt.Errorf("failed to unmarshal prompt %s content: %w", prompt.Slug, err)
		}

		apiRequest.Prompts = append(apiRequest.Prompts, PromptRequest{
			Slug:    prompt.Slug,
			Content: content,
		})

		logger.Debug("processing prompt: %s", prompt.Slug)
	}

	// Send to API
	endpoint := fmt.Sprintf("/cli/deploy/%s/prompts", deploymentId)
	if err := client.Do("PUT", endpoint, apiRequest, nil); err != nil {
		return fmt.Errorf("failed to process prompts via API: %w", err)
	}

	logger.Info("processed %d prompts successfully", len(prompts))
	return nil
}

// ParsePromptsFile parses a single prompts.yaml file
func ParsePromptsFile(filename string) ([]Prompt, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var promptsFile PromptsFile
	if err := yaml.Unmarshal(content, &promptsFile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return promptsFile.Prompts, nil
}
