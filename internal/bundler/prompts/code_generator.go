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
	systemVariables := cg.getSystemVariables(prompt)
	var systemParams []string
	if len(systemVariables) > 0 {
		systemParams = append(systemParams, "variables")
	}
	systemParamStr := strings.Join(systemParams, ", ")

	// Get variables from prompt template
	promptVariables := cg.getPromptVariables(prompt)
	var promptParams []string
	if len(promptVariables) > 0 {
		promptParams = append(promptParams, "variables")
	}
	promptParamStr := strings.Join(promptParams, ", ")

	// Generate docstring with original templates
	docstring := cg.generateDocstring(prompt)

	return fmt.Sprintf(`%sconst %s = {
    slug: %q,
    system: {
        compile: (%s) => {
            return %s
        }
    },
    prompt: {
        compile: (%s) => {
            return %s
        }
    }
};`, docstring, strcase.ToLowerCamel(prompt.Slug), prompt.Slug, systemParamStr, cg.generateTemplateValue(prompt.System, systemVariables), promptParamStr, cg.generateTemplateValue(prompt.Prompt, promptVariables))
}

// generateTemplateValue generates the value for a template (either compile function or direct interpolateTemplate call)
func (cg *CodeGenerator) generateTemplateValue(template string, allVariables []string) string {
	if template == "" {
		return "interpolateTemplate('', variables)"
	}

	return fmt.Sprintf("interpolateTemplate(%q, variables)", template)
}

// generateSystemCompile generates the system compile function body
func (cg *CodeGenerator) generateSystemCompile(template string, allVariables []string) string {
	if template == "" {
		return "interpolateTemplate('', variables);"
	}

	return fmt.Sprintf("interpolateTemplate(%q, variables);", template)
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
	systemCompileType := fmt.Sprintf("(%s) => string", systemParamStr)

	// Get variables from prompt template
	promptVariables := cg.getPromptVariableObjects(prompt)
	var promptParams []string
	if len(promptVariables) > 0 {
		promptParams = append(promptParams, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypesFromObjects(promptVariables)))
	}
	promptParamStr := strings.Join(promptParams, ", ")
	promptCompileType := fmt.Sprintf("(%s) => string", promptParamStr)

	// Generate docstring for TypeScript
	docstring := cg.generateDocstring(prompt)

	return fmt.Sprintf(`%sexport type %s = {
    slug: string;
    system: { compile: %s };
    prompt: { compile: %s };
};`,
		docstring, strcase.ToCamel(prompt.Slug), systemCompileType, promptCompileType)
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

// generateDocstring generates a JSDoc-style docstring for a prompt
func (cg *CodeGenerator) generateDocstring(prompt Prompt) string {
	var docLines []string
	docLines = append(docLines, "/**")

	// Add name and description if available
	if prompt.Name != "" {
		docLines = append(docLines, fmt.Sprintf(" * %s", prompt.Name))
	}
	if prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf(" * %s", prompt.Description))
	}

	// Add original templates
	if prompt.System != "" {
		docLines = append(docLines, " *")
		docLines = append(docLines, " * @system")
		// Escape the template for JSDoc
		escapedSystem := strings.ReplaceAll(prompt.System, "*/", "* /")
		docLines = append(docLines, fmt.Sprintf(" * %s", escapedSystem))
	}

	if prompt.Prompt != "" {
		docLines = append(docLines, " *")
		docLines = append(docLines, " * @prompt")
		// Escape the template for JSDoc
		escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "* /")
		docLines = append(docLines, fmt.Sprintf(" * %s", escapedPrompt))
	}

	docLines = append(docLines, " */")
	docLines = append(docLines, "")

	return strings.Join(docLines, "\n")
}

// generatePromptExports generates the exports object for JavaScript
func (cg *CodeGenerator) generatePromptExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		exports = append(exports, fmt.Sprintf("    %s,", strcase.ToLowerCamel(prompt.Slug)))
	}
	return strings.Join(exports, "\n")
}

// generatePromptTypeExports generates the exports object for TypeScript types
func (cg *CodeGenerator) generatePromptTypeExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		exports = append(exports, fmt.Sprintf("    %s: %s,", strcase.ToLowerCamel(prompt.Slug), strcase.ToCamel(prompt.Slug)))
	}
	return strings.Join(exports, "\n")
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
