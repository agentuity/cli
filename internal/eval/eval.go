package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EvalData struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type EvalMetadata struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LoadEvalMetadataMap scans the evals directory and builds a map of slug -> eval ID
func LoadEvalMetadataMap(logger logger.Logger, projectDir string) (map[string]string, error) {
	evalsDir := filepath.Join(projectDir, "src", "evals")

	// Check if evals directory exists
	if !util.Exists(evalsDir) {
		logger.Debug("evals directory not found: %s", evalsDir)
		return make(map[string]string), nil
	}

	files, err := os.ReadDir(evalsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read evals directory: %w", err)
	}

	slugToIDMap := make(map[string]string)

	for _, file := range files {
		ext := filepath.Ext(file.Name())
		if file.IsDir() || (ext != ".ts" && ext != ".js") {
			continue
		}

		// Skip index files
		if file.Name() == "index.ts" || file.Name() == "index.js" {
			continue
		}

		filePath := filepath.Join(evalsDir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("failed to read eval file %s: %v", file.Name(), err)
			continue
		}

		// Parse metadata from file content
		metadata, err := ParseEvalMetadata(string(content))
		if err != nil {
			logger.Warn("failed to parse metadata from %s: %v", file.Name(), err)
			continue
		}

		if metadata.Slug != "" && metadata.ID != "" {
			slugToIDMap[metadata.Slug] = metadata.ID
			logger.Debug("mapped eval slug '%s' to ID '%s'", metadata.Slug, metadata.ID)
		}
	}

	logger.Debug("loaded %d eval mappings", len(slugToIDMap))
	return slugToIDMap, nil
}

// CreateEval creates a new evaluation function in the project
func CreateEval(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, slug string, name string, description string) (string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	payload := map[string]any{
		"projectId":   projectId,
		"slug":        slug,
		"name":        name,
		"description": description,
	}

	var resp Response[EvalData]
	if err := client.Do("POST", "/cli/evals", payload, &resp); err != nil {
		return "", fmt.Errorf("error creating eval: %s", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("error creating eval: %s", resp.Message)
	}
	return resp.Data.ID, nil
}
