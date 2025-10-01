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
	// Get all variables from both system and prompt templates
	allVariables := cg.getAllVariables(prompt)

	var params []string
	if len(allVariables) > 0 {
		params = append(params, "variables")
	}

	paramStr := strings.Join(params, ", ")

	return fmt.Sprintf(`const %s = {
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
};`, strcase.ToLowerCamel(prompt.Slug), prompt.Slug, paramStr, cg.generateTemplateValue(prompt.System, allVariables), paramStr, cg.generateTemplateValue(prompt.Prompt, allVariables))
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
	// Get all variables from both system and prompt templates
	allVariables := cg.getAllVariables(prompt)

	var params []string
	if len(allVariables) > 0 {
		params = append(params, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypes(allVariables)))
	}

	paramStr := strings.Join(params, ", ")

	compileType := fmt.Sprintf("(%s) => string", paramStr)

	return fmt.Sprintf(`export type %s = {
    slug: string;
    system: { compile: %s };
    prompt: { compile: %s };
};`,
		strcase.ToCamel(prompt.Slug), compileType, compileType)
}

// generatePromptInterface generates a TypeScript interface for a prompt
func (cg *CodeGenerator) generatePromptInterface(prompt Prompt) string {
	// Get all variables from both system and prompt templates
	allVariables := cg.getAllVariables(prompt)

	var params []string
	if len(allVariables) > 0 {
		params = append(params, fmt.Sprintf("variables?: { %s }", cg.generateVariableTypes(allVariables)))
	}

	paramStr := strings.Join(params, ", ")
	compileType := fmt.Sprintf("(%s) => string", paramStr)

	return fmt.Sprintf(`export interface %s {
    slug: string;
    system: { compile: %s };
    prompt: { compile: %s };
}`, strcase.ToCamel(prompt.Slug), compileType, compileType)
}

// generateVariableTypes generates TypeScript types for variables
func (cg *CodeGenerator) generateVariableTypes(variables []string) string {
	var types []string
	for _, variable := range variables {
		types = append(types, fmt.Sprintf("%s: string", variable))
	}
	return strings.Join(types, "; ")
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

// getAllVariables gets all unique variables from both system and prompt templates
func (cg *CodeGenerator) getAllVariables(prompt Prompt) []string {
	allVars := make(map[string]bool)

	// Parse system template if not already parsed
	systemTemplate := prompt.SystemTemplate
	if len(systemTemplate.Variables) == 0 && prompt.System != "" {
		systemTemplate = ParseTemplate(prompt.System)
	}

	// Parse prompt template if not already parsed
	promptTemplate := prompt.PromptTemplate
	if len(promptTemplate.Variables) == 0 && prompt.Prompt != "" {
		promptTemplate = ParseTemplate(prompt.Prompt)
	}

	// Add variables from system template
	for _, variable := range systemTemplate.VariableNames() {
		allVars[variable] = true
	}

	// Add variables from prompt template
	for _, variable := range promptTemplate.VariableNames() {
		allVars[variable] = true
	}

	// Convert map to slice
	var variables []string
	for variable := range allVars {
		variables = append(variables, variable)
	}

	return variables
}
