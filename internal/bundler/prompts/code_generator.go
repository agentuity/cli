package prompts

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
)

// CodeGenerator handles generating JavaScript and TypeScript code from prompts
type CodeGenerator struct {
	prompts []Prompt
}

// NewCodeGenerator creates a new code generator
func NewCodeGenerator(prompts []Prompt) *CodeGenerator {
	return &CodeGenerator{
		prompts: prompts,
	}
}

// GenerateJavaScript generates the main JavaScript file
func (cg *CodeGenerator) GenerateJavaScript() string {
	var objects []string

	for _, prompt := range cg.prompts {
		objects = append(objects, cg.generatePromptObject(prompt))
	}

	return fmt.Sprintf(`// Generated prompts - do not edit manually
import { interpolateTemplate } from '@agentuity/sdk';

%s

/**
 * Collection of all available prompts with JSDoc documentation
 * Each prompt includes original system and prompt templates for reference
 */
export const prompts = {
%s
};`, strings.Join(objects, "\n\n"), cg.generatePromptExports())
}

// GenerateTypeScriptTypes generates the TypeScript definitions file
func (cg *CodeGenerator) GenerateTypeScriptTypes() string {
	var promptTypes []string

	for _, prompt := range cg.prompts {
		promptType := cg.generatePromptType(prompt)
		promptTypes = append(promptTypes, promptType)
	}

	return fmt.Sprintf(`// Generated prompt types - do not edit manually
import { interpolateTemplate } from '@agentuity/sdk';

%s

export type PromptsCollection = {
%s
};

export const prompts: PromptsCollection = {} as any;`, strings.Join(promptTypes, "\n\n"), cg.generatePromptTypeExports())
}

// GenerateTypeScriptInterfaces generates the TypeScript interfaces file
func (cg *CodeGenerator) GenerateTypeScriptInterfaces() string {
	var interfaces []string

	for _, prompt := range cg.prompts {
		interfaceDef := cg.generatePromptInterface(prompt)
		interfaces = append(interfaces, interfaceDef)
	}

	return fmt.Sprintf(`// Generated prompt interfaces - do not edit manually
%s`, strings.Join(interfaces, "\n\n"))
}

// generatePromptObject generates a single prompt object with system and prompt properties
func (cg *CodeGenerator) generatePromptObject(prompt Prompt) string {
	// Get variables from system template
	systemVariables := cg.getSystemVariableObjects(prompt)

	// Get variables from prompt template
	promptVariables := cg.getPromptVariableObjects(prompt)

	// Generate system function signature
	var systemParamStr string
	if len(systemVariables) == 0 {
		systemParamStr = ""
	} else {
		systemParamStr = "variables = {}"
	}

	// Generate prompt function signature
	var promptParamStr string
	if len(promptVariables) == 0 {
		promptParamStr = ""
	} else {
		promptParamStr = "variables = {}"
	}

	return fmt.Sprintf(`const %s = {
    slug: %q,
    system: (%s) => {
        return %s
    },
    prompt: (%s) => {
        return %s
    }
};`, strcase.ToLowerCamel(prompt.Slug), prompt.Slug, systemParamStr, cg.generateTemplateValueWithVars(prompt.System, len(systemVariables) > 0), promptParamStr, cg.generateTemplateValueWithVars(prompt.Prompt, len(promptVariables) > 0))
}

// generateTemplateValue generates the value for a template (either compile function or direct interpolateTemplate call)
func (cg *CodeGenerator) generateTemplateValue(template string) string {
	if template == "" {
		return `""`
	}

	return fmt.Sprintf("interpolateTemplate(%q, variables)", template)
}

// generateTemplateValueWithVars generates the value for a template with variable awareness
func (cg *CodeGenerator) generateTemplateValueWithVars(template string, hasVariables bool) string {
	if template == "" {
		return `""`
	}

	if hasVariables {
		return fmt.Sprintf("interpolateTemplate(%q, variables)", template)
	} else {
		return fmt.Sprintf("interpolateTemplate(%q, {})", template)
	}
}

// generatePromptType generates a TypeScript type for a prompt object
func (cg *CodeGenerator) generatePromptType(prompt Prompt) string {
	// Get variables from system template
	systemVariables := cg.getSystemVariableObjects(prompt)
	var systemParams []string
	if len(systemVariables) > 0 {
		systemParams = append(systemParams, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypesFromObjects(systemVariables)))
	}
	systemParamStr := strings.Join(systemParams, ", ")

	// Get variables from prompt template
	promptVariables := cg.getPromptVariableObjects(prompt)
	var promptParams []string
	if len(promptVariables) > 0 {
		promptParams = append(promptParams, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypesFromObjects(promptVariables)))
	}
	promptParamStr := strings.Join(promptParams, ", ")

	// Generate separate system and prompt types with docstrings
	systemTypeName := fmt.Sprintf("%sSystem", strcase.ToCamel(prompt.Slug))
	promptTypeName := fmt.Sprintf("%sPrompt", strcase.ToCamel(prompt.Slug))
	mainTypeName := strcase.ToCamel(prompt.Slug)

	systemTypeWithDocstring := cg.generateTypeWithDocstring(prompt.System, systemTypeName, systemParamStr, mainTypeName)
	promptTypeWithDocstring := cg.generateTypeWithDocstring(prompt.Prompt, promptTypeName, promptParamStr, mainTypeName)

	return fmt.Sprintf(`%s

%s

export type %s = {
  slug: string;
  /**
%s
   */
  system: (%s) => string;
  /**
%s
   */
  prompt: (%s) => string;
};`,
		systemTypeWithDocstring, promptTypeWithDocstring, mainTypeName, cg.generateTemplateDocstring(prompt.System), systemParamStr, cg.generateTemplateDocstring(prompt.Prompt), promptParamStr)
}

// generatePromptInterface generates a TypeScript interface for a prompt
func (cg *CodeGenerator) generatePromptInterface(prompt Prompt) string {
	// Get variables from system template
	systemVariables := cg.getSystemVariableObjects(prompt)
	var systemParams []string
	if len(systemVariables) > 0 {
		systemParams = append(systemParams, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypesFromObjects(systemVariables)))
	}
	systemParamStr := strings.Join(systemParams, ", ")
	systemCompileType := fmt.Sprintf("(%s) => string", systemParamStr)

	// Get variables from prompt template
	promptVariables := cg.getPromptVariableObjects(prompt)
	var promptParams []string
	if len(promptVariables) > 0 {
		promptParams = append(promptParams, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypesFromObjects(promptVariables)))
	}
	promptParamStr := strings.Join(promptParams, ", ")
	promptCompileType := fmt.Sprintf("(%s) => string", promptParamStr)

	return fmt.Sprintf(`export interface %s {
    slug: string;
    system: { compile: %s };
    prompt: { compile: %s };
}`, strcase.ToCamel(prompt.Slug), systemCompileType, promptCompileType)
}

// generateVariableTypes generates TypeScript types for variables
func (cg *CodeGenerator) generateVariableTypes(variables []string) string {
	var types []string
	for _, variable := range variables {
		types = append(types, fmt.Sprintf("%s: string", variable))
	}
	return strings.Join(types, "; ")
}

// generateVariableTypesFromObjects generates TypeScript types for variables with default values
func (cg *CodeGenerator) generateVariableTypesFromObjects(variables []Variable) string {
	var types []string
	for _, variable := range variables {
		if variable.IsRequired {
			// Required variables are always string
			types = append(types, fmt.Sprintf("%s: string", variable.Name))
		} else if variable.HasDefault {
			// Optional variables with defaults: string | "defaultValue"
			types = append(types, fmt.Sprintf("%s?: string | %q", variable.Name, variable.DefaultValue))
		} else {
			// Optional variables without defaults: string (but parameter is optional)
			types = append(types, fmt.Sprintf("%s?: string", variable.Name))
		}
	}
	return strings.Join(types, "; ")
}

// hasRequiredVariables checks if any variables in the list are required
func (cg *CodeGenerator) hasRequiredVariables(variables []Variable) bool {
	for _, variable := range variables {
		if variable.IsRequired {
			return true
		}
	}
	return false
}

// allVariablesRequired checks if all variables in the list are required
func (cg *CodeGenerator) allVariablesRequired(variables []Variable) bool {
	if len(variables) == 0 {
		return false
	}
	for _, variable := range variables {
		if !variable.IsRequired {
			return false
		}
	}
	return true
}

// generateDocstring generates a JSDoc-style docstring for a prompt
func (cg *CodeGenerator) generateDocstring(prompt Prompt) string {
	var docLines []string
	docLines = append(docLines, "/**")

	// Add name and description with separate tags
	if prompt.Name != "" {
		docLines = append(docLines, fmt.Sprintf(" * @name %s", prompt.Name))
	} else {
		// Fallback to slug-based name
		docLines = append(docLines, fmt.Sprintf(" * @name %s", strcase.ToCamel(prompt.Slug)))
	}

	if prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf(" * @description %s", prompt.Description))
	}

	// Add original templates
	if prompt.System != "" {
		docLines = append(docLines, " *")
		docLines = append(docLines, " * @system")
		// Escape the template for JSDoc and add proper line breaks
		escapedSystem := strings.ReplaceAll(prompt.System, "*/", "* /")
		// Split by newlines and add proper JSDoc formatting
		systemLines := strings.Split(escapedSystem, "\n")
		for _, line := range systemLines {
			docLines = append(docLines, fmt.Sprintf(" * %s", line))
		}
	}

	if prompt.Prompt != "" {
		docLines = append(docLines, " *")
		docLines = append(docLines, " * @prompt")
		// Escape the template for JSDoc and add proper line breaks
		escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "* /")
		// Split by newlines and add proper JSDoc formatting
		promptLines := strings.Split(escapedPrompt, "\n")
		for _, line := range promptLines {
			docLines = append(docLines, fmt.Sprintf(" * %s", line))
		}
	}

	docLines = append(docLines, " */")
	docLines = append(docLines, "")

	return strings.Join(docLines, "\n")
}

// generatePromptExports generates the exports object for JavaScript
func (cg *CodeGenerator) generatePromptExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		// Generate JSDoc comment for each prompt property
		jsdocComment := cg.generatePromptPropertyJSDoc(prompt)
		exports = append(exports, jsdocComment)
		exports = append(exports, fmt.Sprintf("    [%q]: %s,", prompt.Slug, strcase.ToLowerCamel(prompt.Slug)))
	}
	return strings.Join(exports, "\n")
}

// generatePromptTypeExports generates the exports object for TypeScript types
func (cg *CodeGenerator) generatePromptTypeExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		// Generate JSDoc comment for each prompt property
		jsdocComment := cg.generatePromptPropertyJSDoc(prompt)
		exports = append(exports, jsdocComment)
		// Get variables from system template
		systemVariables := cg.getSystemVariableObjects(prompt)
		systemTypeStr := cg.generateVariableTypesFromObjects(systemVariables)

		// Get variables from prompt template
		promptVariables := cg.getPromptVariableObjects(prompt)
		promptTypeStr := cg.generateVariableTypesFromObjects(promptVariables)

		// Generate system function signature
		var systemSignature string
		if len(systemVariables) == 0 {
			systemSignature = "() => string"
		} else if cg.hasRequiredVariables(systemVariables) {
			systemSignature = fmt.Sprintf("(variables: {%s}) => string", systemTypeStr)
		} else {
			systemSignature = fmt.Sprintf("(variables?: {%s}) => string", systemTypeStr)
		}

		// Generate prompt function signature
		var promptSignature string
		if len(promptVariables) == 0 {
			promptSignature = "() => string"
		} else if cg.hasRequiredVariables(promptVariables) {
			promptSignature = fmt.Sprintf("(variables: {%s}) => string", promptTypeStr)
		} else {
			promptSignature = fmt.Sprintf("(variables?: {%s}) => string", promptTypeStr)
		}

		exports = append(exports, fmt.Sprintf("  [%q]: {\n    slug: string;\n    system: %s;\n    prompt: %s;\n  };", prompt.Slug, systemSignature, promptSignature))
	}
	return strings.Join(exports, "\n")
}

// generatePromptPropertyJSDoc generates JSDoc comments for prompt properties in PromptsCollection
func (cg *CodeGenerator) generatePromptPropertyJSDoc(prompt Prompt) string {
	var docLines []string

	// Create JSDoc comment with name, description, and templates
	docLines = append(docLines, "  /**")

	// Add name and description with separate tags
	if prompt.Name != "" {
		docLines = append(docLines, fmt.Sprintf("   * @name %s", prompt.Name))
	} else {
		// Fallback to slug-based name
		docLines = append(docLines, fmt.Sprintf("   * @name %s", strcase.ToCamel(prompt.Slug)))
	}

	if prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf("   * @description %s", prompt.Description))
	}

	// Add original templates
	if prompt.System != "" {
		docLines = append(docLines, "   *")
		docLines = append(docLines, "   * @system")
		// Escape the template for JSDoc and add proper line breaks
		escapedSystem := strings.ReplaceAll(prompt.System, "*/", "* /")
		// Split by newlines and add proper JSDoc formatting
		systemLines := strings.Split(escapedSystem, "\n")
		for _, line := range systemLines {
			docLines = append(docLines, fmt.Sprintf("   * %s", line))
		}
	}

	if prompt.Prompt != "" {
		docLines = append(docLines, "   *")
		docLines = append(docLines, "   * @prompt")
		// Escape the template for JSDoc and add proper line breaks
		escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "* /")
		// Split by newlines and add proper JSDoc formatting
		promptLines := strings.Split(escapedPrompt, "\n")
		for _, line := range promptLines {
			docLines = append(docLines, fmt.Sprintf("   * %s", line))
		}
	}

	docLines = append(docLines, "   */")

	return strings.Join(docLines, "\n")
}

// generatePromptTypeJSDoc generates JSDoc comments for individual prompt types
func (cg *CodeGenerator) generatePromptTypeJSDoc(prompt Prompt) string {
	var docLines []string

	// Create JSDoc comment with name, description, and prompt template only
	docLines = append(docLines, "/**")

	// Add name and description with separate tags
	if prompt.Name != "" {
		docLines = append(docLines, fmt.Sprintf(" * @name %s", prompt.Name))
	} else {
		// Fallback to slug-based name
		docLines = append(docLines, fmt.Sprintf(" * @name %s", strcase.ToCamel(prompt.Slug)))
	}

	if prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf(" * @description %s", prompt.Description))
	}

	// Add only the prompt template
	if prompt.Prompt != "" {
		docLines = append(docLines, " *")
		docLines = append(docLines, " * @prompt")
		// Escape the template for JSDoc and add proper line breaks
		escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "* /")
		// Split by newlines and add proper JSDoc formatting
		promptLines := strings.Split(escapedPrompt, "\n")
		for _, line := range promptLines {
			docLines = append(docLines, fmt.Sprintf(" * %s", line))
		}
	}

	docLines = append(docLines, " */")
	docLines = append(docLines, "")

	return strings.Join(docLines, "\n")
}

// getSystemVariables gets variables from the system template only
func (cg *CodeGenerator) getSystemVariables(prompt Prompt) []string {
	// Parse system template if not already parsed
	systemTemplate := prompt.SystemTemplate
	if len(systemTemplate.Variables) == 0 && prompt.System != "" {
		systemTemplate = ParseTemplate(prompt.System)
	}

	return systemTemplate.VariableNames()
}

// getPromptVariables gets variables from the prompt template only
func (cg *CodeGenerator) getPromptVariables(prompt Prompt) []string {
	// Parse prompt template if not already parsed
	promptTemplate := prompt.PromptTemplate
	if len(promptTemplate.Variables) == 0 && prompt.Prompt != "" {
		promptTemplate = ParseTemplate(prompt.Prompt)
	}

	return promptTemplate.VariableNames()
}

// getSystemVariableObjects gets variable objects from the system template only
func (cg *CodeGenerator) getSystemVariableObjects(prompt Prompt) []Variable {
	// Parse system template if not already parsed
	systemTemplate := prompt.SystemTemplate
	if len(systemTemplate.Variables) == 0 && prompt.System != "" {
		systemTemplate = ParseTemplate(prompt.System)
	}

	return systemTemplate.Variables
}

// getPromptVariableObjects gets variable objects from the prompt template only
func (cg *CodeGenerator) getPromptVariableObjects(prompt Prompt) []Variable {
	// Parse prompt template if not already parsed
	promptTemplate := prompt.PromptTemplate
	if len(promptTemplate.Variables) == 0 && prompt.Prompt != "" {
		promptTemplate = ParseTemplate(prompt.Prompt)
	}

	return promptTemplate.Variables
}

// generateTypeWithDocstring generates a separate type with docstring
func (cg *CodeGenerator) generateTypeWithDocstring(template, typeName, paramStr, mainTypeName string) string {
	if template == "" {
		return fmt.Sprintf(`export type %s = (%s) => string;`,
			typeName, paramStr)
	}

	// Generate JSDoc comment for the type with @memberof
	docstring := cg.generateTemplateDocstring(template)

	return fmt.Sprintf(`/**
%s
 * @memberof %s
 * @type {object}
 */
export type %s = (%s) => string;`,
		docstring, mainTypeName, typeName, paramStr)
}

// generateTemplateDocstring generates the docstring content for any template
func (cg *CodeGenerator) generateTemplateDocstring(template string) string {
	if template == "" {
		return ""
	}

	// Escape the template for docstring and add proper line breaks
	escapedTemplate := strings.ReplaceAll(template, "*/", "* /")
	// Split by newlines and add proper docstring formatting
	templateLines := strings.Split(escapedTemplate, "\n")
	var docLines []string
	for _, line := range templateLines {
		// Add line breaks at natural break points (sentences, periods, etc.)
		formattedLine := cg.addNaturalLineBreaks(line)
		docLines = append(docLines, fmt.Sprintf("  * %s", formattedLine))
	}

	return strings.Join(docLines, "\n")
}

// addNaturalLineBreaks adds line breaks at natural break points
func (cg *CodeGenerator) addNaturalLineBreaks(line string) string {
	// If line is short enough, return as is
	if len(line) <= 60 {
		return line
	}

	// Look for natural break points: periods, commas, or spaces
	// Split at periods followed by space
	parts := strings.Split(line, ". ")
	if len(parts) > 1 {
		var result []string
		for i, part := range parts {
			if i > 0 {
				part = part + "."
			}
			if len(part) > 60 {
				// Further split long parts at commas
				commaParts := strings.Split(part, ", ")
				if len(commaParts) > 1 {
					for j, commaPart := range commaParts {
						if j > 0 {
							commaPart = commaPart + ","
						}
						result = append(result, commaPart)
					}
				} else {
					result = append(result, part)
				}
			} else {
				result = append(result, part)
			}
		}
		// Use HTML line breaks instead of newlines
		return strings.Join(result, ".<br/>  * ")
	}

	return line
}

// wrapLine wraps a long line at the specified width
func (cg *CodeGenerator) wrapLine(line string, width int) []string {
	if len(line) <= width {
		return []string{line}
	}

	var wrapped []string
	words := strings.Fields(line)
	var currentLine strings.Builder

	for _, word := range words {
		// If adding this word would exceed the width, start a new line
		if currentLine.Len() > 0 && currentLine.Len()+len(word)+1 > width {
			wrapped = append(wrapped, currentLine.String())
			currentLine.Reset()
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		wrapped = append(wrapped, currentLine.String())
	}

	return wrapped
}
