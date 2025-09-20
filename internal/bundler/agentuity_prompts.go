package bundler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agentuity/cli/internal/prompts"
	"github.com/agentuity/go-common/logger"
)

// generatePromptsInjection creates the JavaScript code to inject prompt methods into the server
func generatePromptsInjection(logger logger.Logger, projectDir string) string {
	// Find all prompts.yaml files in the source directory
	srcRoot := filepath.Join(projectDir, "src")
	var allPrompts []prompts.Prompt

	// Walk through src directory to find all prompts.yaml files
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		if name != "prompts.yaml" && name != "prompts.yml" {
			return nil
		}

		// Parse the YAML file
		filePrompts, err := prompts.ParsePromptsFile(path)
		if err != nil {
			logger.Warn("failed to parse %s: %v", path, err)
			return nil // Continue processing other files
		}

		allPrompts = append(allPrompts, filePrompts...)
		return nil
	})

	if err != nil {
		logger.Warn("failed to walk src directory: %v", err)
		return ""
	}

	if len(allPrompts) == 0 {
		logger.Debug("no prompts found, skipping PromptAPI injection")
		return ""
	}

	var sb strings.Builder

	// Generate the prompts data object
	sb.WriteString("// Auto-generated prompts from YAML files\n")
	sb.WriteString("const AGENTUITY_PROMPTS = {\n")
	for i, prompt := range allPrompts {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("\t'%s': {\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tslug: '%s',\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tname: '%s',\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t\tdescription: '%s',\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t\tsystem: `%s`,\n", escapeTemplateString(prompt.System)))
		sb.WriteString(fmt.Sprintf("\t\tprompt: `%s`\n", escapeTemplateString(prompt.Prompt)))
		sb.WriteString("\t}")
	}
	sb.WriteString("\n};\n\n")

	// Helper function to convert slug to camelCase method name
	sb.WriteString("function toCamelCase(slug) {\n")
	sb.WriteString("\treturn slug.replace(/-([a-z])/g, (g) => g[1].toUpperCase());\n")
	sb.WriteString("}\n\n")

	// Helper function to fill template variables
	sb.WriteString("function fillTemplate(template, variables = {}) {\n")
	sb.WriteString("\tlet filled = template;\n")
	sb.WriteString("\tfor (const [key, value] of Object.entries(variables)) {\n")
	sb.WriteString("\t\tconst regex = new RegExp(`{\\\\s*${key}\\\\s*}`, 'g');\n")
	sb.WriteString("\t\tfilled = filled.replace(regex, String(value));\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn filled;\n")
	sb.WriteString("}\n\n")

	// Generate TypeScript method signatures to be appended to PromptService interface
	sb.WriteString("\n\t// Auto-generated prompt methods - DO NOT EDIT\n")
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("\t/**\n"))
		sb.WriteString(fmt.Sprintf("\t * %s\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t * %s\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t * @param variables - Variables to substitute in the prompt template\n"))
		sb.WriteString(fmt.Sprintf("\t * @returns Prompt definition with filled template\n"))
		sb.WriteString(fmt.Sprintf("\t */\n"))
		sb.WriteString(fmt.Sprintf("\t%s(variables?: Record<string, unknown>): PromptDefinition;\n\n", methodName))
	}

	logger.Debug("generated PromptAPI injection for %d prompts", len(allPrompts))
	return sb.String()
}

// escapeString escapes a string for use in TypeScript string literals
func escapeString(s string) string {
	// Replace single quotes and backslashes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// escapeTemplateString escapes a string for use in TypeScript template literals
func escapeTemplateString(s string) string {
	// Replace backticks and dollar signs that could interfere with template literals
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")
	return s
}

// toCamelCase converts a kebab-case string to camelCase
func toCamelCase(slug string) string {
	parts := strings.Split(slug, "-")
	if len(parts) <= 1 {
		return slug
	}
	
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			result += strings.ToUpper(string(parts[i][0])) + parts[i][1:]
		}
	}
	return result
}

func init() {
	// We'll patch the types.ts file to inject prompt method definitions into PromptService interface
	patches["@agentuity/sdk-prompts-types"] = patchModule{
		Module:   "@agentuity/sdk",
		Filename: "",  // Match all files in the module
		Body: &patchAction{
			// We'll populate this during bundling with the actual prompt data
			After: "", // Will be set dynamically during bundle
		},
	}
}

// updatePromptsPatches updates the prompts patch with current project data
func updatePromptsPatches(logger logger.Logger, projectDir string) {
	injection := generatePromptsInjection(logger, projectDir)
	
	if injection != "" {
		// Update the types patch
		if patch, exists := patches["@agentuity/sdk-prompts-types"]; exists {
			patch.Body.After = injection
			patches["@agentuity/sdk-prompts-types"] = patch
		}
	}
}

// generatePromptsTypeScript creates TypeScript declaration files for prompts
func generatePromptsTypeScript(logger logger.Logger, projectDir, outdir string) error {
	// Find all prompts.yaml files in the source directory
	srcRoot := filepath.Join(projectDir, "src")
	var allPrompts []prompts.Prompt

	// Walk through src directory to find all prompts.yaml files
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		if name != "prompts.yaml" && name != "prompts.yml" {
			return nil
		}

		// Parse the YAML file
		filePrompts, err := prompts.ParsePromptsFile(path)
		if err != nil {
			logger.Warn("failed to parse %s: %v", path, err)
			return nil // Continue processing other files
		}

		allPrompts = append(allPrompts, filePrompts...)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk src directory: %w", err)
	}

	if len(allPrompts) == 0 {
		logger.Debug("no prompts found, skipping TypeScript generation")
		return nil
	}

	var sb strings.Builder

	// Generate TypeScript module augmentation
	sb.WriteString("// Auto-generated prompt definitions - DO NOT EDIT\n")
	sb.WriteString("// Generated at build time by agentuity bundler\n\n")
	sb.WriteString("declare module '@agentuity/sdk' {\n")
	sb.WriteString("\tinterface PromptService {\n")

	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("\t\t/**\n"))
		sb.WriteString(fmt.Sprintf("\t\t * %s\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t\t * %s\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t\t * @param variables - Variables to substitute in the prompt template\n"))
		sb.WriteString(fmt.Sprintf("\t\t * @returns Prompt definition with filled template\n"))
		sb.WriteString(fmt.Sprintf("\t\t */\n"))
		sb.WriteString(fmt.Sprintf("\t\t%s(variables?: Record<string, unknown>): PromptDefinition;\n\n", methodName))
	}

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	// Write TypeScript declaration file to project root
	declPath := filepath.Join(projectDir, "agentuity-prompts.d.ts")
	if err := os.WriteFile(declPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write TypeScript declaration: %w", err)
	}

	logger.Debug("generated TypeScript declarations for %d prompts at %s", len(allPrompts), declPath)
	return nil
}

// generateRuntimeInjection creates the JavaScript code to inject runtime implementations
func generateRuntimeInjection(logger logger.Logger, projectDir string) string {
	// Find all prompts.yaml files in the source directory
	srcRoot := filepath.Join(projectDir, "src")
	var allPrompts []prompts.Prompt

	// Walk through src directory to find all prompts.yaml files
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		if name != "prompts.yaml" && name != "prompts.yml" {
			return nil
		}

		// Parse the YAML file
		filePrompts, err := prompts.ParsePromptsFile(path)
		if err != nil {
			logger.Warn("failed to parse %s: %v", path, err)
			return nil // Continue processing other files
		}

		allPrompts = append(allPrompts, filePrompts...)
		return nil
	})

	if err != nil {
		logger.Warn("failed to walk src directory: %v", err)
		return ""
	}

	if len(allPrompts) == 0 {
		logger.Debug("no prompts found, skipping runtime injection")
		return ""
	}

	var sb strings.Builder

	// Generate the prompts data object
	sb.WriteString("\n// Auto-generated prompts from YAML files\n")
	sb.WriteString("const AGENTUITY_PROMPTS = {\n")
	for i, prompt := range allPrompts {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("\t'%s': {\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tslug: '%s',\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tname: '%s',\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t\tdescription: '%s',\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t\tsystem: `%s`,\n", escapeTemplateString(prompt.System)))
		sb.WriteString(fmt.Sprintf("\t\tprompt: `%s`\n", escapeTemplateString(prompt.Prompt)))
		sb.WriteString("\t}")
	}
	sb.WriteString("\n};\n\n")

	// Helper function to fill template variables
	sb.WriteString("function fillTemplate(template, variables = {}) {\n")
	sb.WriteString("\tlet filled = template;\n")
	sb.WriteString("\tfor (const [key, value] of Object.entries(variables)) {\n")
	sb.WriteString("\t\tconst regex = new RegExp(`{\\\\s*${key}\\\\s*}`, 'g');\n")
	sb.WriteString("\t\tfilled = filled.replace(regex, String(value));\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn filled;\n")
	sb.WriteString("}\n\n")

	// Inject dynamic methods into the prompt instance
	sb.WriteString("// Inject dynamic prompt methods\n")
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("prompt.%s = function(variables = {}) {\n", methodName))
		sb.WriteString(fmt.Sprintf("\tconst promptDef = AGENTUITY_PROMPTS['%s'];\n", escapeString(prompt.Slug)))
		sb.WriteString("\treturn {\n")
		sb.WriteString("\t\tslug: promptDef.slug,\n")
		sb.WriteString("\t\tname: promptDef.name,\n")
		sb.WriteString("\t\tdescription: promptDef.description,\n")
		sb.WriteString("\t\tsystem: fillTemplate(promptDef.system, variables),\n")
		sb.WriteString("\t\tprompt: fillTemplate(promptDef.prompt, variables)\n")
		sb.WriteString("\t};\n")
		sb.WriteString("};\n\n")
	}

	logger.Debug("generated runtime injection for %d prompts", len(allPrompts))
	return sb.String()
}

// patchSDKFiles directly modifies the SDK files in node_modules
func patchSDKFiles(logger logger.Logger, projectDir string) error {
	// Find all prompts.yaml files in the source directory
	srcRoot := filepath.Join(projectDir, "src")
	var allPrompts []prompts.Prompt

	// Walk through src directory to find all prompts.yaml files
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		if name != "prompts.yaml" && name != "prompts.yml" {
			return nil
		}

		// Parse the YAML file
		filePrompts, err := prompts.ParsePromptsFile(path)
		if err != nil {
			logger.Warn("failed to parse %s: %v", path, err)
			return nil // Continue processing other files
		}

		allPrompts = append(allPrompts, filePrompts...)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk src directory: %w", err)
	}

	if len(allPrompts) == 0 {
		logger.Debug("no prompts found, skipping SDK patching")
		return nil
	}

	// Patch dist/types.d.ts file (compiled declarations that TypeScript actually reads)
	distTypesPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk", "dist", "types.d.ts")
	if err := patchDistTypesFile(logger, distTypesPath, allPrompts); err != nil {
		logger.Warn("failed to patch dist/types.d.ts: %v (TypeScript autocomplete may not work)", err)
	}

	// Patch the dist/apis/prompt.d.ts file for TypeScript support
	promptTypesPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk", "dist", "apis", "prompt.d.ts")
	if err := patchPromptTypesFile(logger, promptTypesPath, allPrompts); err != nil {
		logger.Warn("failed to patch prompt.d.ts: %v (TypeScript autocomplete may not work)", err)
	}

	// Patch the compiled JavaScript file where the actual runtime code lives
	distJSPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk", "dist", "index.js")
	if err := patchDistJSFile(logger, distJSPath, allPrompts); err != nil {
		logger.Warn("failed to patch dist/index.js: %v (runtime functions may not work)", err)
	}

	logger.Debug("successfully patched SDK files for %d prompts", len(allPrompts))
	return nil
}

// patchPromptTypesFile adds method signatures to the prompt API declaration file
func patchPromptTypesFile(logger logger.Logger, filePath string, allPrompts []prompts.Prompt) error {
	// Read the existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	originalContent := string(content)
	
	// Remove any existing auto-generated content
	if strings.Contains(originalContent, "// Auto-generated prompt methods") {
		// Find and remove the existing auto-generated section
		startMarker := "    // Auto-generated prompt methods - DO NOT EDIT\n"
		start := strings.Index(originalContent, startMarker)
		if start != -1 {
			endMarker := "    // End auto-generated prompt methods\n"
			end := strings.Index(originalContent[start:], endMarker)
			if end != -1 {
				end += start + len(endMarker)
				originalContent = originalContent[:start] + originalContent[end:]
			}
		}
	}

	// Generate new method signatures for prompts
	var methodSignatures strings.Builder
	methodSignatures.WriteString("    // Auto-generated prompt methods - DO NOT EDIT\n")
	
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		methodSignatures.WriteString(fmt.Sprintf("    /**\n"))
		methodSignatures.WriteString(fmt.Sprintf("     * %s\n", prompt.Description))
		methodSignatures.WriteString(fmt.Sprintf("     * @param variables - Template variables to substitute\n"))
		methodSignatures.WriteString(fmt.Sprintf("     * @returns Object with system and prompt strings, both with attached metadata properties\n"))
		methodSignatures.WriteString(fmt.Sprintf("     */\n"))
		methodSignatures.WriteString(fmt.Sprintf("    %s(variables?: Record<string, unknown>): Promise<{\n", methodName))
		methodSignatures.WriteString("        system: string & { id: string; slug: string; name: string; description?: string; version: number; variables: Record<string, unknown>; type: 'system' };\n")
		methodSignatures.WriteString("        prompt: string & { id: string; slug: string; name: string; description?: string; version: number; variables: Record<string, unknown>; type: 'prompt' };\n")
		methodSignatures.WriteString("    }>;\n")
	}
	
	methodSignatures.WriteString("    // End auto-generated prompt methods\n")

	// Find the PromptAPI class and insert methods before the closing brace
	classStart := strings.Index(originalContent, "export default class PromptAPI")
	if classStart == -1 {
		return fmt.Errorf("could not find PromptAPI class definition")
	}

	// Find the last method in the class (should be the compile method)
	compileMethodEnd := strings.Index(originalContent[classStart:], "    }): Promise<PromptCompileResult>;")
	if compileMethodEnd == -1 {
		return fmt.Errorf("could not find compile method end")
	}
	compileMethodEnd += classStart + len("    }): Promise<PromptCompileResult>;") + 1

	// Find the closing brace of the class
	classEnd := strings.Index(originalContent[compileMethodEnd:], "}")
	if classEnd == -1 {
		return fmt.Errorf("could not find class closing brace")
	}
	classEnd += compileMethodEnd

	// Insert the new methods before the class closing brace
	newContent := originalContent[:classEnd] + methodSignatures.String() + originalContent[classEnd:]

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	logger.Debug("patched prompt.d.ts with %d method signatures", len(allPrompts))
	return nil
}

// patchDistTypesFile adds method signatures to the compiled declaration file
func patchDistTypesFile(logger logger.Logger, filePath string, allPrompts []prompts.Prompt) error {
	// Read the existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	originalContent := string(content)
	
	// Remove any existing auto-generated content
	if strings.Contains(originalContent, "// Auto-generated prompt methods") {
		// Find and remove the existing auto-generated section
		startMarker := "\n    // Auto-generated prompt methods - DO NOT EDIT\n"
		start := strings.Index(originalContent, startMarker)
		if start != -1 {
			// Find the end of the PromptService interface
			afterStart := start + len(startMarker)
			end := strings.Index(originalContent[afterStart:], "\n}")
			if end != -1 {
				// Remove the old auto-generated section
				originalContent = originalContent[:start] + originalContent[afterStart+end:]
				logger.Debug("removed existing auto-generated methods from dist/index.d.ts")
			}
		}
	}

	// Find the PromptService interface
	promptServiceStart := strings.Index(originalContent, "interface PromptService")
	if promptServiceStart == -1 {
		return fmt.Errorf("could not find PromptService interface in dist file")
	}

	// Find the closing brace of PromptService interface
	braceSearch := promptServiceStart
	openBraces := 0
	interfaceEnd := -1
	
	for i := braceSearch; i < len(originalContent); i++ {
		if originalContent[i] == '{' {
			openBraces++
		} else if originalContent[i] == '}' {
			openBraces--
			if openBraces == 0 {
				interfaceEnd = i
				break
			}
		}
	}
	
	if interfaceEnd == -1 {
		return fmt.Errorf("could not find PromptService interface closing brace")
	}

	// Generate method signatures
	var sb strings.Builder
	sb.WriteString("\n    // Auto-generated prompt methods - DO NOT EDIT\n")
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("    /**\n"))
		sb.WriteString(fmt.Sprintf("     * %s\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("     * %s\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("     * @param variables - Variables to substitute in the prompt template\n"))
		sb.WriteString(fmt.Sprintf("     * @returns Prompt definition with filled template\n"))
		sb.WriteString(fmt.Sprintf("     */\n"))
		sb.WriteString(fmt.Sprintf("    %s(variables?: Record<string, unknown>): PromptDefinition;\n", methodName))
	}

	// Insert the methods before the closing brace
	newContent := originalContent[:interfaceEnd] + sb.String() + originalContent[interfaceEnd:]

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	logger.Debug("patched dist/index.d.ts with %d prompt methods", len(allPrompts))
	return nil
}

// patchTypesFile adds method signatures to the PromptService interface
func patchTypesFile(logger logger.Logger, filePath string, allPrompts []prompts.Prompt) error {
	// Read the existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	originalContent := string(content)
	
	// Remove any existing auto-generated content
	if strings.Contains(originalContent, "// Auto-generated prompt methods") {
		// Find and remove the existing auto-generated section
		startMarker := "\n\t// Auto-generated prompt methods - DO NOT EDIT\n"
		endMarker := "\n}"
		
		start := strings.Index(originalContent, startMarker)
		if start != -1 {
			// Find the end of the PromptService interface (look for the next "}")
			afterStart := start + len(startMarker)
			end := strings.Index(originalContent[afterStart:], endMarker)
			if end != -1 {
				// Remove the old auto-generated section
				originalContent = originalContent[:start] + originalContent[afterStart+end:]
				logger.Debug("removed existing auto-generated methods from types.ts")
			}
		}
	}

	// Find the PromptService interface closing brace
	promptServiceEnd := strings.Index(originalContent, "// Dynamic methods will be injected here for each prompt")
	if promptServiceEnd == -1 {
		return fmt.Errorf("could not find injection point in PromptService interface")
	}

	// Find the actual closing brace after the comment
	braceStart := promptServiceEnd + len("// Dynamic methods will be injected here for each prompt")
	nextBrace := strings.Index(originalContent[braceStart:], "}")
	if nextBrace == -1 {
		return fmt.Errorf("could not find PromptService interface closing brace")
	}
	bracePos := braceStart + nextBrace

	// Generate method signatures
	var sb strings.Builder
	sb.WriteString("\n\t// Auto-generated prompt methods - DO NOT EDIT\n")
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("\t/**\n"))
		sb.WriteString(fmt.Sprintf("\t * %s\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t * %s\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t * @param variables - Variables to substitute in the prompt template\n"))
		sb.WriteString(fmt.Sprintf("\t * @returns Prompt definition with filled template\n"))
		sb.WriteString(fmt.Sprintf("\t */\n"))
		sb.WriteString(fmt.Sprintf("\t%s(variables?: Record<string, unknown>): PromptDefinition;\n\n", methodName))
	}

	// Insert the methods before the closing brace
	newContent := originalContent[:bracePos] + sb.String() + originalContent[bracePos:]

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	logger.Debug("patched types.ts with %d prompt methods", len(allPrompts))
	return nil
}

// patchServerFile adds runtime implementations after the prompt instance creation
func patchServerFile(logger logger.Logger, filePath string, allPrompts []prompts.Prompt) error {
	// Read the existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	originalContent := string(content)
	
	// Remove any existing auto-generated content
	if strings.Contains(originalContent, "// Auto-generated prompts from YAML files") {
		// Find and remove everything from the marker to the next function definition
		startMarker := "\n// Auto-generated prompts from YAML files\n"
		start := strings.Index(originalContent, startMarker)
		if start != -1 {
			// Find the next function or export statement to know where auto-generated content ends
			afterStart := start + len(startMarker)
			patterns := []string{"\n/**\n * Creates an agent context", "\nexport function", "\nfunction "}
			
			var end int = -1
			for _, pattern := range patterns {
				if idx := strings.Index(originalContent[afterStart:], pattern); idx != -1 {
					end = idx
					break
				}
			}
			
			if end != -1 {
				// Remove the old auto-generated section
				originalContent = originalContent[:start] + originalContent[afterStart+end:]
				logger.Debug("removed existing auto-generated content from server.ts")
			}
		}
	}

	// Find the line after "const prompt = new PromptAPI();"
	promptCreation := strings.Index(originalContent, "const prompt = new PromptAPI();")
	if promptCreation == -1 {
		return fmt.Errorf("could not find 'const prompt = new PromptAPI();' line")
	}

	// Find the end of the line
	lineEnd := strings.Index(originalContent[promptCreation:], "\n")
	if lineEnd == -1 {
		return fmt.Errorf("could not find end of prompt creation line")
	}
	insertPos := promptCreation + lineEnd + 1

	// Generate runtime implementation
	var sb strings.Builder
	sb.WriteString("\n// Auto-generated prompts from YAML files\n")
	sb.WriteString("const AGENTUITY_PROMPTS = {\n")
	for i, prompt := range allPrompts {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("\t'%s': {\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tslug: '%s',\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("\t\tname: '%s',\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("\t\tdescription: '%s',\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("\t\tsystem: `%s`,\n", escapeTemplateString(prompt.System)))
		sb.WriteString(fmt.Sprintf("\t\tprompt: `%s`\n", escapeTemplateString(prompt.Prompt)))
		sb.WriteString("\t}")
	}
	sb.WriteString("\n};\n\n")

	// Helper function to fill template variables
	sb.WriteString("function fillTemplate(template, variables = {}) {\n")
	sb.WriteString("\tlet filled = template;\n")
	sb.WriteString("\tfor (const [key, value] of Object.entries(variables)) {\n")
	sb.WriteString("\t\tconst regex = new RegExp(`{\\\\s*${key}\\\\s*}`, 'g');\n")
	sb.WriteString("\t\tfilled = filled.replace(regex, String(value));\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn filled;\n")
	sb.WriteString("}\n\n")

	// Inject dynamic methods into the prompt instance
	sb.WriteString("// Inject dynamic prompt methods\n")
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("prompt.%s = function(variables = {}) {\n", methodName))
		sb.WriteString(fmt.Sprintf("\tconst promptDef = AGENTUITY_PROMPTS['%s'];\n", escapeString(prompt.Slug)))
		sb.WriteString("\treturn {\n")
		sb.WriteString("\t\tslug: promptDef.slug,\n")
		sb.WriteString("\t\tname: promptDef.name,\n")
		sb.WriteString("\t\tdescription: promptDef.description,\n")
		sb.WriteString("\t\tsystem: fillTemplate(promptDef.system, variables),\n")
		sb.WriteString("\t\tprompt: fillTemplate(promptDef.prompt, variables)\n")
		sb.WriteString("\t};\n")
		sb.WriteString("};\n\n")
	}

	// Insert the implementation after the prompt creation
	newContent := originalContent[:insertPos] + sb.String() + originalContent[insertPos:]

	// Write back to file
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	logger.Debug("patched server.ts with %d prompt implementations", len(allPrompts))
	return nil
}

// patchDistJSFile patches the compiled JavaScript file with runtime functions
func patchDistJSFile(logger logger.Logger, filePath string, allPrompts []prompts.Prompt) error {
	// Resolve symlinks to get the real file path
	realPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		realPath = filePath // fallback to original path if symlink resolution fails
	}
	
	// Read the existing compiled file
	content, err := os.ReadFile(realPath)
	if err != nil {
		return err
	}

	originalContent := string(content)
	
	// Check if this exact content is already patched for these prompts
	if strings.Contains(originalContent, "// Auto-generated prompts from YAML files") {
		// Only remove if we can find a very specific bounded section
		startMarker := "\n// Auto-generated prompts from YAML files\n"
		endMarker := "\n// End auto-generated prompts\n"
		
		start := strings.Index(originalContent, startMarker)
		end := strings.Index(originalContent, endMarker)
		
		if start != -1 && end != -1 && end > start {
			// Remove the old bounded auto-generated section
			originalContent = originalContent[:start] + originalContent[end+len(endMarker):]
			logger.Debug("removed existing bounded auto-generated content from %s", realPath)
		} else {
			// If we can't find bounded markers, skip patching to avoid corruption
			logger.Debug("found auto-generated content but no proper bounds, skipping to avoid corruption")
			return nil
		}
	}

	// Find where to inject the prompt functions - look for prompt instance creation
	// In compiled code, it might look different, so search for patterns
	var insertPos int = -1
	patterns := []string{
		"prompt2 = new PromptAPI();",
		"prompt = new PromptAPI();", 
		"const prompt = new PromptAPI2();",
	}
	
	for _, pattern := range patterns {
		if idx := strings.Index(originalContent, pattern); idx != -1 {
			// Find the end of this line
			lineEnd := strings.Index(originalContent[idx:], "\n")
			if lineEnd != -1 {
				insertPos = idx + lineEnd + 1
				break
			}
		}
	}
	
	if insertPos == -1 {
		return fmt.Errorf("could not find prompt instance creation in compiled file")
	}

	// Generate the runtime implementation for compiled code
	var sb strings.Builder
	sb.WriteString("\n// Auto-generated prompts from YAML files\n")
	sb.WriteString("const AGENTUITY_PROMPTS = {\n")
	for i, prompt := range allPrompts {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("  '%s': {\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("    slug: '%s',\n", escapeString(prompt.Slug)))
		sb.WriteString(fmt.Sprintf("    name: '%s',\n", escapeString(prompt.Name)))
		if prompt.Description != "" {
			sb.WriteString(fmt.Sprintf("    description: '%s',\n", escapeString(prompt.Description)))
		}
		sb.WriteString(fmt.Sprintf("    system: `%s`,\n", escapeTemplateString(prompt.System)))
		sb.WriteString(fmt.Sprintf("    prompt: `%s`\n", escapeTemplateString(prompt.Prompt)))
		sb.WriteString("  }")
	}
	sb.WriteString("\n};\n")

	// Helper function
	sb.WriteString("function fillTemplate2(template, variables = {}) {\n")
	sb.WriteString("  let filled = template;\n")
	sb.WriteString("  for (const [key, value] of Object.entries(variables)) {\n")
	sb.WriteString("    const regex = new RegExp(`{\\\\s*${key}\\\\s*}`, 'g');\n")
	sb.WriteString("    filled = filled.replace(regex, String(value));\n")
	sb.WriteString("  }\n")
	sb.WriteString("  return filled;\n")
	sb.WriteString("}\n")

	// Find the actual prompt variable name in compiled code and inject methods
	// Try different possible variable names the compiler might use
	promptVars := []string{"prompt2", "prompt", "prompt3"}
	var promptVar string
	for _, v := range promptVars {
		if strings.Contains(originalContent, v+" = new PromptAPI") {
			promptVar = v
			break
		}
	}
	
	if promptVar == "" {
		return fmt.Errorf("could not find prompt variable in compiled code")
	}

	// Inject the methods
	for _, prompt := range allPrompts {
		methodName := toCamelCase(prompt.Slug)
		sb.WriteString(fmt.Sprintf("%s.%s = async function(variables = {}) {\n", promptVar, methodName))
		sb.WriteString(fmt.Sprintf("  const promptDef = AGENTUITY_PROMPTS['%s'];\n", escapeString(prompt.Slug)))
		sb.WriteString("  const systemFilled = fillTemplate2(promptDef.system, variables);\n")
		sb.WriteString("  const promptFilled = fillTemplate2(promptDef.prompt, variables);\n")
		sb.WriteString("  \n")
		sb.WriteString("  // Create String objects with metadata for both system and prompt\n")
		sb.WriteString("  const systemResult = new String(systemFilled);\n")
		sb.WriteString("  const promptResult = new String(promptFilled);\n")
		sb.WriteString("  \n")
		sb.WriteString("  // Attach metadata to system\n")
		sb.WriteString(fmt.Sprintf("  systemResult.id = '%s';\n", escapeString(prompt.Slug)))
		sb.WriteString("  systemResult.slug = promptDef.slug;\n")
		sb.WriteString("  systemResult.name = promptDef.name;\n")
		sb.WriteString("  systemResult.description = promptDef.description;\n")
		sb.WriteString("  systemResult.version = 1;\n")
		sb.WriteString("  systemResult.variables = variables;\n")
		sb.WriteString("  systemResult.type = 'system';\n")
		sb.WriteString("  \n")
		sb.WriteString("  // Attach metadata to prompt\n")
		sb.WriteString(fmt.Sprintf("  promptResult.id = '%s';\n", escapeString(prompt.Slug)))
		sb.WriteString("  promptResult.slug = promptDef.slug;\n")
		sb.WriteString("  promptResult.name = promptDef.name;\n")
		sb.WriteString("  promptResult.description = promptDef.description;\n")
		sb.WriteString("  promptResult.version = 1;\n")
		sb.WriteString("  promptResult.variables = variables;\n")
		sb.WriteString("  promptResult.type = 'prompt';\n")
		sb.WriteString("  \n")
		sb.WriteString("  return {\n")
		sb.WriteString("    system: systemResult,\n")
		sb.WriteString("    prompt: promptResult\n")
		sb.WriteString("  };\n")
		sb.WriteString("};\n")
	}
	sb.WriteString("\n")
	sb.WriteString("// End auto-generated prompts\n")

	// Insert the implementation
	newContent := originalContent[:insertPos] + sb.String() + originalContent[insertPos:]

	// Write back to the real file (not the symlink)
	if err := os.WriteFile(realPath, []byte(newContent), 0644); err != nil {
		return err
	}

	logger.Debug("patched %s with %d prompt implementations using variable %s", realPath, len(allPrompts), promptVar)
	return nil
}

// generatePromptTypeDeclarations generates only TypeScript declarations for prompt methods
func generatePromptTypeDeclarations(logger logger.Logger, projectDir string) error {
	// Find all prompts.yaml files in the source directory
	srcRoot := filepath.Join(projectDir, "src")
	var allPrompts []prompts.Prompt

	// Walk through src directory to find all prompts.yaml files
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		if name != "prompts.yaml" && name != "prompts.yml" {
			return nil
		}

		// Parse the YAML file
		filePrompts, err := prompts.ParsePromptsFile(path)
		if err != nil {
			logger.Warn("failed to parse %s: %v", path, err)
			return nil // Continue processing other files
		}

		allPrompts = append(allPrompts, filePrompts...)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk src directory: %w", err)
	}

	if len(allPrompts) == 0 {
		logger.Debug("no prompts found, skipping TypeScript declarations")
		return nil
	}

	// Only patch the TypeScript declarations, not runtime
	typesPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk", "src", "types.ts")
	if err := patchTypesFile(logger, typesPath, allPrompts); err != nil {
		logger.Warn("failed to patch types.ts: %v", err)
	}

	// Also patch the compiled declarations
	distTypesPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk", "dist", "types.d.ts")
	if err := patchDistTypesFile(logger, distTypesPath, allPrompts); err != nil {
		logger.Warn("failed to patch dist/types.d.ts: %v", err)
	}

	logger.Debug("generated TypeScript declarations for %d prompts", len(allPrompts))
	return nil
}

// rebuildSDK rebuilds the SDK dist folder after patching source files
func rebuildSDK(logger logger.Logger, projectDir string) error {
	sdkPath := filepath.Join(projectDir, "node_modules", "@agentuity", "sdk")
	
	// Check if npm is available
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = sdkPath
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	logger.Debug("rebuilding SDK at %s", sdkPath)
	if err := cmd.Run(); err != nil {
		// Try with bun if npm fails
		cmd = exec.Command("bun", "run", "build")
		cmd.Dir = sdkPath
		cmd.Stdout = nil
		cmd.Stderr = nil
		
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to rebuild SDK with npm or bun: %w", err)
		}
	}
	
	logger.Debug("successfully rebuilt SDK")
	return nil
}
