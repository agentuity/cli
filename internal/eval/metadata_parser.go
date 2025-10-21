package eval

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ParseEvalMetadata extracts metadata from TypeScript/JavaScript eval file content
func ParseEvalMetadata(content string) (*EvalMetadata, error) {
	// Find the metadata export pattern
	metadataRegex := regexp.MustCompile(`export\s+const\s+metadata\s*=\s*\{`)
	metadataStart := metadataRegex.FindStringIndex(content)
	if metadataStart == nil {
		return nil, fmt.Errorf("no metadata export found")
	}

	// Find the opening brace position
	braceStart := metadataStart[1] - 1 // Position of the opening brace
	if braceStart >= len(content) || content[braceStart] != '{' {
		return nil, fmt.Errorf("invalid metadata format: opening brace not found")
	}

	// Count braces to find the matching closing brace
	braceCount := 0
	braceEnd := -1
	for i := braceStart; i < len(content); i++ {
		if content[i] == '{' {
			braceCount++
		} else if content[i] == '}' {
			braceCount--
			if braceCount == 0 {
				braceEnd = i
				break
			}
		}
	}

	if braceEnd == -1 {
		return nil, fmt.Errorf("no matching closing brace found")
	}

	// Extract the object content and wrap with braces for valid JSON
	objectContent := content[braceStart : braceEnd+1]

	// Replace single quotes with double quotes for valid JSON
	jsonStr := regexp.MustCompile(`'([^']*)'`).ReplaceAllString(objectContent, `"$1"`)

	// Clean up the JSON string by removing extra whitespace and newlines
	jsonStr = regexp.MustCompile(`\s+`).ReplaceAllString(jsonStr, " ")
	jsonStr = regexp.MustCompile(`\s*{\s*`).ReplaceAllString(jsonStr, "{")
	jsonStr = regexp.MustCompile(`\s*}\s*`).ReplaceAllString(jsonStr, "}")
	jsonStr = regexp.MustCompile(`\s*:\s*`).ReplaceAllString(jsonStr, ":")
	jsonStr = regexp.MustCompile(`\s*,\s*`).ReplaceAllString(jsonStr, ",")

	// Remove trailing commas before closing braces
	jsonStr = regexp.MustCompile(`,\s*}`).ReplaceAllString(jsonStr, "}")

	// Quote the object keys
	jsonStr = regexp.MustCompile(`(\w+):`).ReplaceAllString(jsonStr, `"$1":`)

	// Parse the JSON
	var metadata EvalMetadata
	if err := json.Unmarshal([]byte(jsonStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata JSON: %w", err)
	}

	return &metadata, nil
}
