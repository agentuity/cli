package bundler

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Prompt represents a single prompt definition from YAML
type Prompt struct {
	Name        string   `yaml:"name"`
	Slug        string   `yaml:"slug"`
	Description string   `yaml:"description"`
	System      string   `yaml:"system,omitempty"`
	Prompt      string   `yaml:"prompt"`
	Evals       []string `yaml:"evals,omitempty"`
}

// PromptsYAML represents the structure of prompts.yaml
type PromptsYAML struct {
	Prompts []Prompt `yaml:"prompts"`
}

// VariableInfo holds information about extracted variables
type VariableInfo struct {
	Names []string
}

var variableRegex = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// ParsePromptsYAML parses a prompts.yaml file and returns the prompt definitions
func ParsePromptsYAML(filePath string) ([]Prompt, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts.yaml: %w", err)
	}

	var promptsData PromptsYAML
	if err := yaml.Unmarshal(data, &promptsData); err != nil {
		return nil, fmt.Errorf("failed to parse prompts.yaml: %w", err)
	}

	if len(promptsData.Prompts) == 0 {
		return nil, fmt.Errorf("no prompts found in prompts.yaml")
	}

	// Validate prompts
	for i, prompt := range promptsData.Prompts {
		if prompt.Name == "" || prompt.Slug == "" {
			return nil, fmt.Errorf("invalid prompt at index %d: missing required fields (name, slug)", i)
		}
		// At least one of system or prompt must be present
		if prompt.System == "" && prompt.Prompt == "" {
			return nil, fmt.Errorf("invalid prompt at index %d: must have at least one of system or prompt", i)
		}
	}

	return promptsData.Prompts, nil
}

// FindPromptsYAML finds prompts.yaml in the given directory
func FindPromptsYAML(dir string) string {
	possiblePaths := []string{
		filepath.Join(dir, "src", "prompts.yaml"),
		filepath.Join(dir, "src", "prompts.yml"),
		filepath.Join(dir, "prompts.yaml"),
		filepath.Join(dir, "prompts.yml"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// ExtractVariables extracts {{variable}} patterns from a template string
func ExtractVariables(template string) []string {
	matches := variableRegex.FindAllStringSubmatch(template, -1)
	variables := make([]string, 0, len(matches))
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			varName := strings.TrimSpace(match[1])
			if !seen[varName] {
				variables = append(variables, varName)
				seen[varName] = true
			}
		}
	}

	return variables
}

// GetAllVariables extracts all variables from both system and prompt fields
func GetAllVariables(prompt Prompt) []string {
	allVars := make(map[string]bool)

	// Extract from prompt field
	for _, v := range ExtractVariables(prompt.Prompt) {
		allVars[v] = true
	}

	// Extract from system field if present
	if prompt.System != "" {
		for _, v := range ExtractVariables(prompt.System) {
			allVars[v] = true
		}
	}

	// Convert to slice
	variables := make([]string, 0, len(allVars))
	for v := range allVars {
		variables = append(variables, v)
	}

	return variables
}

// EscapeTemplateString escapes a string for use in generated TypeScript
func EscapeTemplateString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ToCamelCase converts a kebab-case slug to camelCase
func ToCamelCase(slug string) string {
	parts := strings.Split(slug, "-")
	if len(parts) == 0 {
		return slug
	}

	result := strings.ToLower(parts[0])
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
		}
	}

	return result
}

// GenerateTypeScriptTypes generates TypeScript type definitions
func GenerateTypeScriptTypes(prompts []Prompt) string {
	var promptTypes []string

	for _, prompt := range prompts {
		methodName := ToCamelCase(prompt.Slug)

		// Get variables separately for system and prompt
		systemVariables := ExtractVariables(prompt.System)
		promptVariables := ExtractVariables(prompt.Prompt)

		// Generate variable interfaces for each
		systemVariablesInterface := "{}"
		if len(systemVariables) > 0 {
			varTypes := make([]string, len(systemVariables))
			for i, v := range systemVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			systemVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		promptVariablesInterface := "{}"
		if len(promptVariables) > 0 {
			varTypes := make([]string, len(promptVariables))
			for i, v := range promptVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			promptVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		// Build the type with conditional fields
		var typeFields []string
		typeFields = append(typeFields, fmt.Sprintf(`    slug: "%s"`, prompt.Slug))
		typeFields = append(typeFields, fmt.Sprintf(`    name: "%s"`, prompt.Name))
		typeFields = append(typeFields, fmt.Sprintf(`    description: "%s"`, prompt.Description))

		// Add system field only if it exists
		if prompt.System != "" {
			typeFields = append(typeFields, fmt.Sprintf(`    system: {
      compile(variables?: %s): string;
    }`, systemVariablesInterface))
		}

		// Add prompt field only if it exists
		if prompt.Prompt != "" {
			typeFields = append(typeFields, fmt.Sprintf(`    prompt: {
      compile(variables?: %s): string;
    }`, promptVariablesInterface))
		}

		// Generate JSDoc comment
		jsdoc := fmt.Sprintf(`  /**
   * @name %s
   * @description %s`, prompt.Name, prompt.Description)

		if prompt.System != "" {
			jsdoc += fmt.Sprintf(`
   * @system %s`, strings.ReplaceAll(prompt.System, "\n", "\n   * "))
		}

		if prompt.Prompt != "" {
			jsdoc += fmt.Sprintf(`
   * @prompt %s`, strings.ReplaceAll(prompt.Prompt, "\n", "\n   * "))
		}

		jsdoc += "\n   */"

		promptType := fmt.Sprintf(`%s
  %s: {
%s
  }`, jsdoc, methodName, strings.Join(typeFields, ";\n"))

		promptTypes = append(promptTypes, promptType)
	}

	return fmt.Sprintf(`export interface PromptsCollection {
%s
}

export declare const prompts: PromptsCollection;
export type PromptConfig = any;
export type PromptName = any;
`, strings.Join(promptTypes, ";\n"))
}

// GenerateTypeScript generates TypeScript code with split system/prompt compile functions
func GenerateTypeScript(prompts []Prompt) string {
	var methods []string

	for _, prompt := range prompts {
		methodName := ToCamelCase(prompt.Slug)
		escapedPrompt := EscapeTemplateString(prompt.Prompt)
		escapedSystem := ""
		if prompt.System != "" {
			escapedSystem = EscapeTemplateString(prompt.System)
		}

		// Get variables separately for system and prompt
		systemVariables := ExtractVariables(prompt.System)
		promptVariables := ExtractVariables(prompt.Prompt)

		// Generate variable interfaces for each
		systemVariablesInterface := "{}"
		if len(systemVariables) > 0 {
			varTypes := make([]string, len(systemVariables))
			for i, v := range systemVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			systemVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		promptVariablesInterface := "{}"
		if len(promptVariables) > 0 {
			varTypes := make([]string, len(promptVariables))
			for i, v := range promptVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			promptVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		_ = systemVariablesInterface // suppress unused warning
		_ = promptVariablesInterface // suppress unused warning

		// Generate function signatures - always make variables optional
		systemFunctionSignature := "(variables = {})"
		promptFunctionSignature := "(variables = {})"

		// Build the method with conditional fields
		var fields []string
		fields = append(fields, fmt.Sprintf(`    slug: "%s"`, prompt.Slug))
		fields = append(fields, fmt.Sprintf(`    name: "%s"`, prompt.Name))
		fields = append(fields, fmt.Sprintf(`    description: "%s"`, prompt.Description))

		// Add system field only if it exists
		if prompt.System != "" {
			fields = append(fields, fmt.Sprintf(`    system: {
      compile%s {
        const template = "%s";
        return template.replace(/\{\{([^}]+)\}\}/g, (match, varName) => {
          return (variables as any)[varName] || match;
        });
      }
    }`, systemFunctionSignature, escapedSystem))
		}

		// Add prompt field only if it exists
		if prompt.Prompt != "" {
			fields = append(fields, fmt.Sprintf(`    prompt: {
      compile%s {
        const template = "%s";
        return template.replace(/\{\{([^}]+)\}\}/g, (match, varName) => {
          return (variables as any)[varName] || match;
        });
      }
    }`, promptFunctionSignature, escapedPrompt))
		}

		// Generate JSDoc comment
		jsdoc := fmt.Sprintf(`  /**
   * @name %s
   * @description %s`, prompt.Name, prompt.Description)

		if prompt.System != "" {
			jsdoc += fmt.Sprintf(`
   * @system %s`, strings.ReplaceAll(prompt.System, "\n", "\n   * "))
		}

		if prompt.Prompt != "" {
			jsdoc += fmt.Sprintf(`
   * @prompt %s`, strings.ReplaceAll(prompt.Prompt, "\n", "\n   * "))
		}

		jsdoc += "\n   */"

		method := fmt.Sprintf(`%s
  %s: {
%s
  }`, jsdoc, methodName, strings.Join(fields, ",\n"))

		methods = append(methods, method)
	}

	return fmt.Sprintf(`export const prompts = {
%s
};

// Export function that SDK will use
// Note: All compile functions return string (never undefined/null)
// This ensures no optional chaining is needed in agent code
export function createPromptsAPI() {
  return prompts;
}
`, strings.Join(methods, ",\n"))
}

// GenerateJavaScript generates JavaScript version (for runtime)
func GenerateJavaScript(prompts []Prompt) string {
	var methods []string

	for _, prompt := range prompts {
		methodName := ToCamelCase(prompt.Slug)
		escapedPrompt := EscapeTemplateString(prompt.Prompt)
		escapedSystem := ""
		if prompt.System != "" {
			escapedSystem = EscapeTemplateString(prompt.System)
		}

		// Get variables separately for system and prompt
		systemVariables := ExtractVariables(prompt.System)
		promptVariables := ExtractVariables(prompt.Prompt)

		// Generate variable interfaces for each
		systemVariablesInterface := "{}"
		if len(systemVariables) > 0 {
			varTypes := make([]string, len(systemVariables))
			for i, v := range systemVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			systemVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		promptVariablesInterface := "{}"
		if len(promptVariables) > 0 {
			varTypes := make([]string, len(promptVariables))
			for i, v := range promptVariables {
				varTypes[i] = fmt.Sprintf("%s: string", v)
			}
			promptVariablesInterface = fmt.Sprintf("{ %s }", strings.Join(varTypes, ", "))
		}

		_ = systemVariablesInterface // suppress unused warning
		_ = promptVariablesInterface // suppress unused warning

		// Generate function signatures - always make variables optional
		systemFunctionSignature := "(variables = {})"
		promptFunctionSignature := "(variables = {})"

		// Build the method with conditional fields
		var fields []string
		fields = append(fields, fmt.Sprintf(`    slug: "%s"`, prompt.Slug))
		fields = append(fields, fmt.Sprintf(`    name: "%s"`, prompt.Name))
		fields = append(fields, fmt.Sprintf(`    description: "%s"`, prompt.Description))

		// Add system field only if it exists
		if prompt.System != "" {
			fields = append(fields, fmt.Sprintf(`    system: {
      compile%s {
        const template = "%s";
        return template.replace(/\{\{([^}]+)\}\}/g, (match, varName) => {
          return variables[varName] || match;
        });
      }
    }`, systemFunctionSignature, escapedSystem))
		}

		// Add prompt field only if it exists
		if prompt.Prompt != "" {
			fields = append(fields, fmt.Sprintf(`    prompt: {
      compile%s {
        const template = "%s";
        return template.replace(/\{\{([^}]+)\}\}/g, (match, varName) => {
          return variables[varName] || match;
        });
      }
    }`, promptFunctionSignature, escapedPrompt))
		}

		// Generate JSDoc comment
		jsdoc := fmt.Sprintf(`  /**
   * @name %s
   * @description %s`, prompt.Name, prompt.Description)

		if prompt.System != "" {
			jsdoc += fmt.Sprintf(`
   * @system %s`, strings.ReplaceAll(prompt.System, "\n", "\n   * "))
		}

		if prompt.Prompt != "" {
			jsdoc += fmt.Sprintf(`
   * @prompt %s`, strings.ReplaceAll(prompt.Prompt, "\n", "\n   * "))
		}

		jsdoc += "\n   */"

		method := fmt.Sprintf(`%s
  %s: {
%s
  }`, jsdoc, methodName, strings.Join(fields, ",\n"))

		methods = append(methods, method)
	}

	return fmt.Sprintf(`export const prompts = {
%s
};
`, strings.Join(methods, ",\n"))
}

// FindSDKGeneratedDir finds the SDK's generated directory in node_modules
func FindSDKGeneratedDir(ctx BundleContext, projectDir string) (string, error) {
	// Try workspace root first, then project dir
	possibleRoots := []string{
		findWorkspaceInstallDir(ctx.Logger, projectDir), // Use existing workspace detection
		projectDir,
	}

	for _, root := range possibleRoots {
		// For production SDK, generate into the new prompt folder structure
		sdkPath := filepath.Join(root, "node_modules", "@agentuity", "sdk", "dist", "apis", "prompt", "generated")
		if _, err := os.Stat(filepath.Join(root, "node_modules", "@agentuity", "sdk")); err == nil {
			// SDK exists, ensure generated directory exists
			if err := os.MkdirAll(sdkPath, 0755); err == nil {
				return sdkPath, nil
			}
		}
		// Fallback to src directory (development)
		sdkPath = filepath.Join(root, "node_modules", "@agentuity", "sdk", "src", "apis", "prompt", "generated")
		if _, err := os.Stat(filepath.Join(root, "node_modules", "@agentuity", "sdk", "src", "apis", "prompt")); err == nil {
			if err := os.MkdirAll(sdkPath, 0755); err == nil {
				return sdkPath, nil
			}
		}
	}

	return "", fmt.Errorf("could not find @agentuity/sdk in node_modules")
}

// ProcessPrompts finds, parses, and generates prompt files into the SDK
func ProcessPrompts(ctx BundleContext, projectDir string) error {
	// Find prompts.yaml
	promptsPath := FindPromptsYAML(projectDir)
	if promptsPath == "" {
		// No prompts.yaml found - this is OK, not all projects will have prompts
		ctx.Logger.Debug("No prompts.yaml found in project, skipping prompt generation")
		return nil
	}

	ctx.Logger.Debug("Found prompts.yaml at: %s", promptsPath)

	// Parse prompts.yaml
	prompts, err := ParsePromptsYAML(promptsPath)
	if err != nil {
		return fmt.Errorf("failed to parse prompts: %w", err)
	}

	ctx.Logger.Debug("Parsed %d prompts from YAML", len(prompts))

	// Find SDK generated directory
	sdkGeneratedDir, err := FindSDKGeneratedDir(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("failed to find SDK directory: %w", err)
	}

	ctx.Logger.Debug("Found SDK generated directory: %s", sdkGeneratedDir)

	// Generate index.js file (overwrite SDK's placeholder, following POC pattern)
	jsContent := GenerateJavaScript(prompts)
	jsPath := filepath.Join(sdkGeneratedDir, "_index.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.js: %w", err)
	}

	// Generate index.d.ts file (overwrite SDK's placeholder, following POC pattern)
	dtsContent := GenerateTypeScriptTypes(prompts)
	dtsPath := filepath.Join(sdkGeneratedDir, "index.d.ts")
	if err := os.WriteFile(dtsPath, []byte(dtsContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.d.ts: %w", err)
	}

	ctx.Logger.Info("Generated prompts into SDK: %s and %s", jsPath, dtsPath)

	return nil
}
