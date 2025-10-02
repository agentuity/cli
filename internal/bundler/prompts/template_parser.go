package prompts

import (
	"regexp"
	"strings"
)

// Variable represents a single variable found in a template
type Variable struct {
	Name           string `json:"name"`
	IsRequired     bool   `json:"is_required"`
	HasDefault     bool   `json:"has_default"`
	DefaultValue   string `json:"default_value,omitempty"`
	OriginalSyntax string `json:"original_syntax"`
}

// Template represents a parsed template with its variables
type Template struct {
	OriginalTemplate string     `json:"original_template"`
	Variables        []Variable `json:"variables"`
}

// RequiredVariables returns all required variables
func (t Template) RequiredVariables() []Variable {
	var result []Variable
	for _, v := range t.Variables {
		if v.IsRequired {
			result = append(result, v)
		}
	}
	return result
}

// OptionalVariables returns all optional variables
func (t Template) OptionalVariables() []Variable {
	var result []Variable
	for _, v := range t.Variables {
		if !v.IsRequired {
			result = append(result, v)
		}
	}
	return result
}

// VariablesWithDefaults returns all variables that have default values
func (t Template) VariablesWithDefaults() []Variable {
	var result []Variable
	for _, v := range t.Variables {
		if v.HasDefault {
			result = append(result, v)
		}
	}
	return result
}

// VariablesWithoutDefaults returns all variables that don't have default values
func (t Template) VariablesWithoutDefaults() []Variable {
	var result []Variable
	for _, v := range t.Variables {
		if !v.HasDefault {
			result = append(result, v)
		}
	}
	return result
}

// VariableNames returns just the names of all variables
func (t Template) VariableNames() []string {
	names := make([]string, len(t.Variables))
	for i, v := range t.Variables {
		names[i] = v.Name
	}
	return names
}

// RequiredVariableNames returns just the names of required variables
func (t Template) RequiredVariableNames() []string {
	var names []string
	for _, v := range t.Variables {
		if v.IsRequired {
			names = append(names, v.Name)
		}
	}
	return names
}

// OptionalVariableNames returns just the names of optional variables
func (t Template) OptionalVariableNames() []string {
	var names []string
	for _, v := range t.Variables {
		if !v.IsRequired {
			names = append(names, v.Name)
		}
	}
	return names
}

// Regex for extracting variables from both {{variable}} and {variable:default} syntax (used in YAML)
// This supports both legacy {{variable}} and new {variable:default} syntax
// Also supports {!variable:-default} syntax for required variables with defaults
var variableRegex = regexp.MustCompile(`\{\{([^}]+)\}\}|\{([!]?[^}:]+)(?::(-?[^}]*))?\}`)

// ParseTemplate parses a template string and returns structured information about all variables
// Supports {{variable}}, {{!required}}, {variable:default}, {!variable:default} syntax
func ParseTemplate(template string) Template {
	matches := variableRegex.FindAllStringSubmatch(template, -1)
	variables := make([]Variable, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		// Ensure we have at least 4 elements: full match + 3 capture groups
		if len(match) < 4 {
			continue // Skip malformed matches
		}

		var varName string
		var isRequired bool
		var hasDefault bool
		var defaultValue string
		var originalSyntax string

		// Handle {{variable}} syntax (match[1])
		if match[1] != "" {
			varName = strings.TrimSpace(match[1])
			isRequired = false // {{variable}} is always optional
			hasDefault = false // {{variable}} has no default
			originalSyntax = "{{" + varName + "}}"
		} else if match[2] != "" {
			// Handle {variable:default} syntax (match[2])
			originalSyntax = match[0] // Full match including braces
			varName = strings.TrimSpace(match[2])
			isRequired = strings.HasPrefix(varName, "!")
			hasDefault = match[3] != "" // Has default if match[3] is not empty
			defaultValue = match[3]

			// Clean up the variable name
			if isRequired && len(varName) > 1 {
				varName = varName[1:] // Remove ! prefix
			}
			if hasDefault {
				// Remove :default suffix
				if idx := strings.Index(varName, ":"); idx != -1 {
					varName = varName[:idx]
				}
				// Handle :- syntax for required variables with defaults
				if len(defaultValue) > 0 && strings.HasPrefix(defaultValue, "-") {
					defaultValue = defaultValue[1:] // Remove leading dash
				}
			}
		}

		if varName != "" && !seen[varName] {
			seen[varName] = true
			variables = append(variables, Variable{
				Name:           varName,
				IsRequired:     isRequired,
				HasDefault:     hasDefault,
				DefaultValue:   defaultValue,
				OriginalSyntax: originalSyntax,
			})
		}
	}

	return Template{
		OriginalTemplate: template,
		Variables:        variables,
	}
}

// ParseTemplateVariables parses {{variable}} and {variable:default} patterns from a template string
// Supports {{variable}}, {{!required}}, {variable:default}, {!variable:default} syntax
func ParseTemplateVariables(template string) []string {
	matches := variableRegex.FindAllStringSubmatch(template, -1)
	variables := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			var varName string

			// Handle {{variable}} syntax (match[1])
			if match[1] != "" {
				varName = strings.TrimSpace(match[1])
			} else if match[2] != "" {
				// Handle {variable:default} syntax (match[2])
				varName = strings.TrimSpace(match[2])
			}

			if varName != "" {
				// Remove ! prefix if present
				if strings.HasPrefix(varName, "!") {
					varName = varName[1:]
				}
				// Remove :default suffix if present
				if idx := strings.Index(varName, ":"); idx != -1 {
					varName = varName[:idx]
				}
				if !seen[varName] {
					variables = append(variables, varName)
					seen[varName] = true
				}
			}
		}
	}

	return variables
}

// ParsedPrompt represents a parsed prompt with system and prompt template information
type ParsedPrompt struct {
	System Template `json:"system"`
	Prompt Template `json:"prompt"`
}

// ParsePrompt parses both system and prompt templates and returns structured information
func ParsePrompt(prompt Prompt) ParsedPrompt {
	systemParsed := ParseTemplate(prompt.System)
	promptParsed := ParseTemplate(prompt.Prompt)

	return ParsedPrompt{
		System: systemParsed,
		Prompt: promptParsed,
	}
}
