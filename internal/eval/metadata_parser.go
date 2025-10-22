package eval

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ParseEvalMetadata extracts metadata from TypeScript/JavaScript eval file content
func ParseEvalMetadata(content string) (*EvalMetadata, error) {
	// Find the metadata export pattern with optional type annotations
	metadataRegex := regexp.MustCompile(`export\s+const\s+metadata(?:\s*:[^=]+)?\s*=\s*\{`)
	metadataStart := metadataRegex.FindStringIndex(content)
	if metadataStart == nil {
		return nil, fmt.Errorf("no metadata export found")
	}

	// Find the opening brace position
	braceStart := metadataStart[1] - 1 // Position of the opening brace
	if braceStart >= len(content) || content[braceStart] != '{' {
		return nil, fmt.Errorf("invalid metadata format: opening brace not found")
	}

	// Find the matching closing brace using string-aware parsing
	braceEnd, err := findMatchingBrace(content, braceStart)
	if err != nil {
		return nil, fmt.Errorf("no matching closing brace found: %w", err)
	}

	// Extract the object content
	objectContent := content[braceStart : braceEnd+1]

	// Convert single quotes to double quotes for JSON compatibility
	jsonStr := normalizeToJSON(objectContent)

	// Parse the JSON
	var metadata EvalMetadata
	if err := json.Unmarshal([]byte(jsonStr), &metadata); err != nil {
		return nil, fmt.Errorf("expect valid JSON object after `export const metadata =`: %w", err)
	}

	return &metadata, nil
}

// findMatchingBrace finds the matching closing brace using string-aware parsing
func findMatchingBrace(content string, start int) (int, error) {
	braceCount := 0
	inString := false
	escapeNext := false

	for i := start; i < len(content); i++ {
		char := content[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' || char == '\'' {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					return i, nil
				}
			}
		}
	}

	return -1, fmt.Errorf("no matching closing brace found")
}

// normalizeToJSON converts JavaScript object syntax to valid JSON
func normalizeToJSON(content string) string {
	result := make([]rune, 0, len(content))
	inString := false
	escapeNext := false

	for _, char := range content {
		if escapeNext {
			escapeNext = false
			result = append(result, char)
			continue
		}

		if char == '\\' {
			escapeNext = true
			result = append(result, char)
			continue
		}

		if char == '"' || char == '\'' {
			if !inString {
				// Opening quote - always use double quote
				result = append(result, '"')
				inString = true
			} else {
				// Closing quote - always use double quote
				result = append(result, '"')
				inString = false
			}
			continue
		}

		if !inString && char == '\'' {
			// Single quote outside string - convert to double quote
			result = append(result, '"')
			inString = true
			continue
		}

		result = append(result, char)
	}

	// Now quote unquoted keys, but only outside of strings
	jsonStr := string(result)
	jsonStr = quoteUnquotedKeys(jsonStr)

	return jsonStr
}

// quoteUnquotedKeys quotes unquoted keys in JSON, but only outside of strings
func quoteUnquotedKeys(content string) string {
	result := make([]rune, 0, len(content))
	inString := false
	escapeNext := false

	for i, char := range content {
		if escapeNext {
			escapeNext = false
			result = append(result, char)
			continue
		}

		if char == '\\' {
			escapeNext = true
			result = append(result, char)
			continue
		}

		if char == '"' {
			inString = !inString
			result = append(result, char)
			continue
		}

		if !inString && char == ':' {
			// Look backwards to find the start of the key
			keyStart := i
			for j := i - 1; j >= 0; j-- {
				if content[j] == ' ' || content[j] == '\t' || content[j] == '\n' {
					keyStart = j + 1
					break
				}
				if content[j] == ',' || content[j] == '{' {
					keyStart = j + 1
					break
				}
			}

			// Check if the key is already quoted
			if keyStart < i && content[keyStart] != '"' {
				// Key is not quoted, add quotes around the key we already added
				// Remove the key from result and add it quoted
				keyLength := i - keyStart
				result = result[:len(result)-keyLength]
				result = append(result, '"')
				result = append(result, []rune(content[keyStart:i])...)
				result = append(result, '"')
			}
			result = append(result, char)
			continue
		}

		result = append(result, char)
	}

	return string(result)
}
