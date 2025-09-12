package prompts

import (
	"context"
	"crypto/sha256"
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
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	System      string `yaml:"system,omitempty" json:"system,omitempty"`
	Prompt      string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
}

type PromptsFile struct {
	Prompts []Prompt `yaml:"prompts" json:"prompts"`
}

type PromptRequest struct {
	Slug        string                 `json:"slug"`
	Content     map[string]interface{} `json:"content"`
	ContentHash string                 `json:"content_hash"`
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

	prompts, err := parsePromptsFile(promptsFile)
	if err != nil {
		return fmt.Errorf("failed to parse prompts.yaml: %w", err)
	}

	if len(prompts) == 0 {
		logger.Debug("no prompts found in prompts.yaml, skipping prompts processing")
		return nil
	}

	// Validate prompts have required fields
	for _, prompt := range prompts {
		if prompt.ID == "" {
			return fmt.Errorf("prompt missing required 'id' field")
		}
		if prompt.Name == "" {
			return fmt.Errorf("prompt '%s' missing required 'name' field", prompt.ID)
		}
		if prompt.System == "" {
			return fmt.Errorf("prompt '%s' missing required 'system' field", prompt.ID)
		}
		if prompt.Prompt == "" {
			return fmt.Errorf("prompt '%s' missing required 'prompt' field", prompt.ID)
		}
	}

	// Convert to API request format
	var apiRequest PromptsAPIRequest
	for _, prompt := range prompts {
		// Convert prompt to map for JSON serialization
		contentBytes, err := json.Marshal(prompt)
		if err != nil {
			return fmt.Errorf("failed to marshal prompt %s: %w", prompt.ID, err)
		}

		var content map[string]interface{}
		if err := json.Unmarshal(contentBytes, &content); err != nil {
			return fmt.Errorf("failed to unmarshal prompt %s content: %w", prompt.ID, err)
		}

		// Calculate content hash for change detection
		contentHash := calculateContentHash(content)

		apiRequest.Prompts = append(apiRequest.Prompts, PromptRequest{
			Slug:        prompt.ID,
			Content:     content,
			ContentHash: contentHash,
		})

		logger.Debug("processing prompt: %s (hash: %s)", prompt.ID, contentHash)
	}

	// Send to API
	endpoint := fmt.Sprintf("/cli/deploy/%s/prompts", deploymentId)
	if err := client.Do("PUT", endpoint, apiRequest, nil); err != nil {
		return fmt.Errorf("failed to process prompts via API: %w", err)
	}

	logger.Info("processed %d prompts successfully", len(prompts))
	return nil
}

// parsePromptsFile parses a single prompts.yaml file
func parsePromptsFile(filename string) ([]Prompt, error) {
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

// calculateContentHash generates a SHA256 hash of the prompt content for change detection
func calculateContentHash(content map[string]interface{}) string {
	// Sort keys for consistent hashing
	contentBytes, _ := json.Marshal(content)
	hash := sha256.Sum256(contentBytes)
	return fmt.Sprintf("%x", hash)
}
