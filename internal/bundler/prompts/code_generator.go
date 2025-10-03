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
import { interpolateTemplate } from '../../../index.js';

%s

/**
 * Collection of all available prompts with JSDoc documentation
 * Each prompt includes original system and prompt templates for reference
 */
export const prompts = {
%s
};

`, strings.Join(objects, "\n\n"), cg.generatePromptExports())
}

// generateCompileFunctionSignature generates the compile function signature dynamically
func (cg *CodeGenerator) generateCompileFunctionSignature() string {
	var cases []string

	for _, prompt := range cg.prompts {
		hasSystem := prompt.System != ""
		hasPrompt := prompt.Prompt != ""
		hasSystemVars := len(cg.getSystemVariableObjects(prompt)) > 0
		hasPromptVars := len(cg.getPromptVariableObjects(prompt)) > 0

		// Check if all variables are optional
		systemVars := cg.getSystemVariableObjects(prompt)
		promptVars := cg.getPromptVariableObjects(prompt)
		allSystemOptional := cg.areAllVariablesOptional(systemVars)
		allPromptOptional := cg.areAllVariablesOptional(promptVars)

		var caseStr string
		if !hasSystemVars && !hasPromptVars {
			// No variables at all
			caseStr = fmt.Sprintf("T extends '%s' ? []", prompt.Slug)
		} else if allSystemOptional && allPromptOptional {
			// All variables are optional - make params optional
			var params []string
			if hasSystem && hasSystemVars {
				systemType := cg.generateParameterInterface(systemVars, allSystemOptional)
				params = append(params, fmt.Sprintf("system?: %s", systemType))
			}
			if hasPrompt && hasPromptVars {
				promptType := cg.generateParameterInterface(promptVars, allPromptOptional)
				params = append(params, fmt.Sprintf("prompt?: %s", promptType))
			}

			paramStr := strings.Join(params, ", ")
			caseStr = fmt.Sprintf("T extends '%s' ? [] | [{ %s }]", prompt.Slug, paramStr)
		} else {
			// Has required variables
			var params []string
			if hasSystem && hasSystemVars {
				systemType := cg.generateParameterInterface(systemVars, allSystemOptional)
				params = append(params, fmt.Sprintf("system: %s", systemType))
			}
			if hasPrompt && hasPromptVars {
				promptType := cg.generateParameterInterface(promptVars, allPromptOptional)
				params = append(params, fmt.Sprintf("prompt: %s", promptType))
			}

			paramStr := strings.Join(params, ", ")
			caseStr = fmt.Sprintf("T extends '%s' ? [{ %s }]", prompt.Slug, paramStr)
		}

		cases = append(cases, caseStr)
	}

	if len(cases) == 0 {
		return "[]"
	}

	return strings.Join(cases, "\n      : ") + "\n      : []"
}

// GenerateTypeScriptTypes generates the TypeScript definitions file
func (cg *CodeGenerator) GenerateTypeScriptTypes() string {
	var promptTypes []string

	for _, prompt := range cg.prompts {
		promptType := cg.generateDetailedPromptType(prompt)
		promptTypes = append(promptTypes, promptType)
	}

	return fmt.Sprintf(`// Generated prompt types - do not edit manually
import { interpolateTemplate, Prompt } from '@agentuity/sdk';

%s

export interface GeneratedPromptsCollection {
%s
}

export type PromptsCollection = GeneratedPromptsCollection;

export const prompts: PromptsCollection = {} as any;`, strings.Join(promptTypes, "\n\n"), cg.generatePromptTypeExports())
}

// GenerateStubsFile generates the stubs file with actual generated types
func (cg *CodeGenerator) GenerateStubsFile() string {
	var promptTypes []string
	for _, prompt := range cg.prompts {
		promptTypes = append(promptTypes, cg.generatePromptType(prompt))
	}
	promptCollection := cg.generatePromptTypeExports()

	return fmt.Sprintf(`// Generated prompt types - do not edit manually
import { interpolateTemplate, Prompt } from '@agentuity/sdk';

%s

export interface GeneratedPromptsCollection {
%s
}

export type PromptsCollection = GeneratedPromptsCollection;
`, strings.Join(promptTypes, "\n\n"), promptCollection)
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
	// Determine if prompt has system, prompt, and variables
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""
	hasSystemVars := len(cg.getSystemVariables(prompt)) > 0
	hasPromptVars := len(cg.getPromptVariables(prompt)) > 0
	hasVariables := hasSystemVars || hasPromptVars

	// Generate the prompt object with conditional fields
	var fields []string
	fields = append(fields, fmt.Sprintf("slug: %q", prompt.Slug))

	if hasSystem {
		fields = append(fields, cg.generateSystemField(prompt))
	}

	if hasPrompt {
		fields = append(fields, cg.generatePromptField(prompt))
	}

	if hasVariables {
		fields = append(fields, cg.generateVariablesField(prompt))
	}

	return fmt.Sprintf(`const %s = {
    %s
};`, strcase.ToLowerCamel(prompt.Slug), strings.Join(fields, ",\n    "))
}

// generateSystemField generates the system field for a prompt
func (cg *CodeGenerator) generateSystemField(prompt Prompt) string {
	systemVars := cg.getSystemVariableObjects(prompt)

	// Generate JSDoc comment with the system prompt
	jsdoc := cg.generateSystemJSDoc(prompt)

	if len(systemVars) > 0 {
		allOptional := cg.areAllVariablesOptional(systemVars)

		// Generate parameter destructuring for variables
		var paramNames []string
		for _, variable := range systemVars {
			paramNames = append(paramNames, variable.Name)
		}
		paramStr := strings.Join(paramNames, ", ")

		if allOptional {
			// Make parameters optional
			return fmt.Sprintf(`system: %s({ %s } = {}) => {
        return interpolateTemplate(%q, { %s })
    }`, jsdoc, paramStr, prompt.System, paramStr)
		} else {
			// Parameters are required
			return fmt.Sprintf(`system: %s({ %s }) => {
        return interpolateTemplate(%q, { %s })
    }`, jsdoc, paramStr, prompt.System, paramStr)
		}
	}
	return fmt.Sprintf(`system: %s() => {
        return interpolateTemplate(%q, {})
    }`, jsdoc, prompt.System)
}

// generatePromptField generates the prompt field for a prompt
func (cg *CodeGenerator) generatePromptField(prompt Prompt) string {
	promptVars := cg.getPromptVariableObjects(prompt)

	// Generate JSDoc comment with the prompt content
	jsdoc := cg.generatePromptJSDoc(prompt)

	if len(promptVars) > 0 {
		allOptional := cg.areAllVariablesOptional(promptVars)

		// Generate parameter destructuring for variables
		var paramNames []string
		for _, variable := range promptVars {
			paramNames = append(paramNames, variable.Name)
		}
		paramStr := strings.Join(paramNames, ", ")

		if allOptional {
			// Make parameters optional
			return fmt.Sprintf(`prompt: %s({ %s } = {}) => {
        return interpolateTemplate(%q, { %s })
    }`, jsdoc, paramStr, prompt.Prompt, paramStr)
		} else {
			// Parameters are required
			return fmt.Sprintf(`prompt: %s({ %s }) => {
        return interpolateTemplate(%q, { %s })
    }`, jsdoc, paramStr, prompt.Prompt, paramStr)
		}
	}
	return fmt.Sprintf(`prompt: %s() => {
        return interpolateTemplate(%q, {})
    }`, jsdoc, prompt.Prompt)
}

// generateVariablesField generates the variables field for a prompt
func (cg *CodeGenerator) generateVariablesField(prompt Prompt) string {
	var allVars []Variable
	systemVars := cg.getSystemVariableObjects(prompt)
	promptVars := cg.getPromptVariableObjects(prompt)

	// Combine system and prompt variables
	allVars = append(allVars, systemVars...)
	allVars = append(allVars, promptVars...)

	if len(allVars) == 0 {
		return ""
	}

	var varDefs []string
	for _, variable := range allVars {
		varDefs = append(varDefs, fmt.Sprintf("%s: %q", variable.Name, "string"))
	}

	return fmt.Sprintf("variables: { %s }", strings.Join(varDefs, ", "))
}

// generateSignatureFunction generates a signature function for a prompt
func (cg *CodeGenerator) generateSignatureFunction(prompt Prompt) string {
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""
	hasSystemVars := len(cg.getSystemVariableObjects(prompt)) > 0
	hasPromptVars := len(cg.getPromptVariableObjects(prompt)) > 0

	// Check if all variables are optional
	systemVars := cg.getSystemVariableObjects(prompt)
	promptVars := cg.getPromptVariableObjects(prompt)
	allSystemOptional := cg.areAllVariablesOptional(systemVars)
	allPromptOptional := cg.areAllVariablesOptional(promptVars)
	allOptional := allSystemOptional && allPromptOptional

	// Generate the function signature based on what the prompt has
	var params []string
	if hasSystem && hasSystemVars {
		params = append(params, "system")
	}
	if hasPrompt && hasPromptVars {
		params = append(params, "prompt")
	}

	paramStr := strings.Join(params, ", ")
	if paramStr != "" {
		if allOptional {
			paramStr = fmt.Sprintf("{ %s } = {}", paramStr)
		} else {
			paramStr = fmt.Sprintf("{ %s }", paramStr)
		}
	}

	// Generate the function body
	var bodyParts []string
	if hasSystem {
		if hasSystemVars {
			if allSystemOptional {
				bodyParts = append(bodyParts, fmt.Sprintf("const systemResult = prompts['%s'].system(system)", prompt.Slug))
			} else {
				bodyParts = append(bodyParts, fmt.Sprintf("const systemResult = prompts['%s'].system(system)", prompt.Slug))
			}
		} else {
			bodyParts = append(bodyParts, fmt.Sprintf("const systemResult = prompts['%s'].system()", prompt.Slug))
		}
	}
	if hasPrompt {
		if hasPromptVars {
			if allPromptOptional {
				bodyParts = append(bodyParts, fmt.Sprintf("const promptResult = prompts['%s'].prompt(prompt)", prompt.Slug))
			} else {
				bodyParts = append(bodyParts, fmt.Sprintf("const promptResult = prompts['%s'].prompt(prompt)", prompt.Slug))
			}
		} else {
			bodyParts = append(bodyParts, fmt.Sprintf("const promptResult = prompts['%s'].prompt()", prompt.Slug))
		}
	}

	// Combine results
	if len(bodyParts) == 1 {
		bodyParts = append(bodyParts, "return systemResult || promptResult")
	} else if len(bodyParts) == 2 {
		bodyParts = append(bodyParts, "return `${systemResult}\\n${promptResult}`")
	} else {
		bodyParts = append(bodyParts, "return ''")
	}

	body := strings.Join(bodyParts, ";\n    ")

	if paramStr == "" {
		return fmt.Sprintf(`'%s': () => {
    %s
}`, prompt.Slug, body)
	}

	return fmt.Sprintf(`'%s': (%s) => {
    %s
}`, prompt.Slug, paramStr, body)
}

// generateTemplateValue generates the value for a template (either compile function or direct interpolateTemplate call)
func (cg *CodeGenerator) generateTemplateValue(template string) string {
	if template == "" {
		return `""`
	}

	return fmt.Sprintf("interpolateTemplate(%q, variables)", template)
}

// generatePromptType generates a TypeScript type for a prompt object using generics
func (cg *CodeGenerator) generatePromptType(prompt Prompt) string {
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""
	hasSystemVars := len(cg.getSystemVariables(prompt)) > 0
	hasPromptVars := len(cg.getPromptVariables(prompt)) > 0
	hasVariables := hasSystemVars || hasPromptVars

	// Generate the generic type parameters
	var genericParams []string
	if hasSystem {
		genericParams = append(genericParams, "true")
	} else {
		genericParams = append(genericParams, "false")
	}
	if hasPrompt {
		genericParams = append(genericParams, "true")
	} else {
		genericParams = append(genericParams, "false")
	}
	if hasVariables {
		genericParams = append(genericParams, "true")
	} else {
		genericParams = append(genericParams, "false")
	}

	genericStr := strings.Join(genericParams, ", ")
	mainTypeName := strcase.ToCamel(prompt.Slug)

	// Generate the type definition
	var fields []string
	fields = append(fields, "slug: string")

	if hasSystem {
		fields = append(fields, "system: (params: { system: Record<string, unknown> }) => string")
	}

	if hasPrompt {
		fields = append(fields, "prompt: (params: { prompt: Record<string, unknown> }) => string")
	}

	if hasVariables {
		// Generate variable types
		var allVars []Variable
		allVars = append(allVars, cg.getSystemVariableObjects(prompt)...)
		allVars = append(allVars, cg.getPromptVariableObjects(prompt)...)

		var varDefs []string
		for _, variable := range allVars {
			varDefs = append(varDefs, fmt.Sprintf("%s: string", variable.Name))
		}
		fields = append(fields, fmt.Sprintf("variables: { %s }", strings.Join(varDefs, ", ")))
	}

	return fmt.Sprintf(`export type %s = Prompt<%s>;`, mainTypeName, genericStr)
}

// generateDetailedPromptType generates detailed TypeScript types with specific parameter interfaces
func (cg *CodeGenerator) generateDetailedPromptType(prompt Prompt) string {
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""
	hasSystemVars := len(cg.getSystemVariables(prompt)) > 0
	hasPromptVars := len(cg.getPromptVariables(prompt)) > 0

	mainTypeName := strcase.ToCamel(prompt.Slug)

	// Generate parameter interfaces
	var systemParams string
	var promptParams string

	if hasSystemVars {
		systemVars := cg.getSystemVariableObjects(prompt)
		allSystemOptional := cg.areAllVariablesOptional(systemVars)
		systemParams = cg.generateParameterInterface(systemVars, allSystemOptional)
	}

	if hasPromptVars {
		promptVars := cg.getPromptVariableObjects(prompt)
		allPromptOptional := cg.areAllVariablesOptional(promptVars)
		promptParams = cg.generateParameterInterface(promptVars, allPromptOptional)
	}

	// Generate the main prompt type with JSDoc comments
	var fields []string
	fields = append(fields, "slug: string")

	if hasSystem {
		systemJSDoc := cg.generateSystemJSDocForType(prompt)
		if hasSystemVars {
			systemVars := cg.getSystemVariableObjects(prompt)
			allSystemOptional := cg.areAllVariablesOptional(systemVars)
			if allSystemOptional {
				fields = append(fields, fmt.Sprintf("%s  system: (variables?: %s) => string", systemJSDoc, systemParams))
			} else {
				fields = append(fields, fmt.Sprintf("%s  system: (variables: %s) => string", systemJSDoc, systemParams))
			}
		} else {
			fields = append(fields, fmt.Sprintf("%s  system: () => string", systemJSDoc))
		}
	}

	if hasPrompt {
		promptJSDoc := cg.generatePromptJSDocForType(prompt)
		if hasPromptVars {
			promptVars := cg.getPromptVariableObjects(prompt)
			allPromptOptional := cg.areAllVariablesOptional(promptVars)
			if allPromptOptional {
				fields = append(fields, fmt.Sprintf("%s  prompt: (variables?: %s) => string", promptJSDoc, promptParams))
			} else {
				fields = append(fields, fmt.Sprintf("%s  prompt: (variables: %s) => string", promptJSDoc, promptParams))
			}
		} else {
			fields = append(fields, fmt.Sprintf("%s  prompt: () => string", promptJSDoc))
		}
	}

	fieldsStr := strings.Join(fields, ";\n  ")

	// Generate parameter interface for compile function
	var compileParams string
	if hasSystemVars || hasPromptVars {
		var paramFields []string
		if hasSystemVars {
			paramFields = append(paramFields, fmt.Sprintf("system: %s", systemParams))
		}
		if hasPromptVars {
			paramFields = append(paramFields, fmt.Sprintf("prompt: %s", promptParams))
		}
		compileParams = fmt.Sprintf("{\n    %s\n  }", strings.Join(paramFields, ";\n    "))
	} else {
		compileParams = "never"
	}

	// Generate JSDoc typedef for the prompt type
	typedefJSDoc := cg.generateTypedefJSDoc(prompt)

	return fmt.Sprintf(`%sexport type %s = {
  %s
};

export type %sParams = %s;`, typedefJSDoc, mainTypeName, fieldsStr, mainTypeName, compileParams)
}

// areAllVariablesOptional checks if all variables in a list are optional
func (cg *CodeGenerator) areAllVariablesOptional(variables []Variable) bool {
	for _, variable := range variables {
		if variable.IsRequired {
			return false
		}
	}
	return true
}

// generateParameterInterface generates a TypeScript interface for variables
func (cg *CodeGenerator) generateParameterInterface(variables []Variable, isOptional bool) string {
	if len(variables) == 0 {
		return "{}"
	}

	var fields []string
	for _, variable := range variables {
		if variable.IsRequired {
			fields = append(fields, fmt.Sprintf("%s: string", variable.Name))
		} else {
			// Include default value as union type if it exists
			if variable.HasDefault {
				fields = append(fields, fmt.Sprintf("%s?: string | %q", variable.Name, variable.DefaultValue))
			} else {
				fields = append(fields, fmt.Sprintf("%s?: string", variable.Name))
			}
		}
	}

	return fmt.Sprintf("{\n    %s\n  }", strings.Join(fields, ";\n    "))
}

// generateSignatureType generates a TypeScript type for a signature function
func (cg *CodeGenerator) generateSignatureType(prompt Prompt) string {
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""

	// Generate the function signature based on what the prompt has
	var params []string
	if hasSystem {
		params = append(params, "system: Record<string, unknown>")
	}
	if hasPrompt {
		params = append(params, "prompt: Record<string, unknown>")
	}

	paramStr := strings.Join(params, ", ")
	if paramStr != "" {
		paramStr = fmt.Sprintf("params: { %s }", paramStr)
	} else {
		paramStr = ""
	}

	if paramStr == "" {
		return fmt.Sprintf(`%s: () => string`, prompt.Slug)
	}

	return fmt.Sprintf(`%s: (%s) => string`, prompt.Slug, paramStr)
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
		// Combine JSDoc and property on the same line
		exports = append(exports, fmt.Sprintf("%s\n  '%s': %s", jsdocComment, prompt.Slug, strcase.ToLowerCamel(prompt.Slug)))
	}
	return strings.Join(exports, ",\n")
}

// generatePromptTypeExports generates the exports object for TypeScript types
func (cg *CodeGenerator) generatePromptTypeExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		// Generate JSDoc comment for each prompt property
		jsdocComment := cg.generatePromptPropertyJSDoc(prompt)
		exports = append(exports, jsdocComment)
		exports = append(exports, fmt.Sprintf("  '%s': %s;", prompt.Slug, strcase.ToCamel(prompt.Slug)))
	}
	return strings.Join(exports, "\n")
}

// generateSignatureTypeExports generates the exports object for signature types
func (cg *CodeGenerator) generateSignatureTypeExports() string {
	var exports []string
	for _, prompt := range cg.prompts {
		exports = append(exports, fmt.Sprintf("  '%s': (%s) => string", prompt.Slug, cg.generateSignatureFunctionParams(prompt)))
	}
	return strings.Join(exports, "\n")
}

// generateSignatureFunctionParams generates the parameter string for signature functions
func (cg *CodeGenerator) generateSignatureFunctionParams(prompt Prompt) string {
	hasSystem := prompt.System != ""
	hasPrompt := prompt.Prompt != ""
	hasSystemVars := len(cg.getSystemVariableObjects(prompt)) > 0
	hasPromptVars := len(cg.getPromptVariableObjects(prompt)) > 0

	var params []string
	if hasSystem && hasSystemVars {
		systemVars := cg.getSystemVariableObjects(prompt)
		allOptional := cg.areAllVariablesOptional(systemVars)
		systemType := cg.generateParameterInterface(systemVars, allOptional)
		params = append(params, fmt.Sprintf("system: %s", systemType))
	}
	if hasPrompt && hasPromptVars {
		promptVars := cg.getPromptVariableObjects(prompt)
		allOptional := cg.areAllVariablesOptional(promptVars)
		promptType := cg.generateParameterInterface(promptVars, allOptional)
		params = append(params, fmt.Sprintf("prompt: %s", promptType))
	}

	if len(params) == 0 {
		return ""
	}

	return fmt.Sprintf("params: { %s }", strings.Join(params, ", "))
}

// generatePromptPropertyJSDoc generates JSDoc comments for prompt properties in PromptsCollection
func (cg *CodeGenerator) generatePromptPropertyJSDoc(prompt Prompt) string {
	var docLines []string

	// Create JSDoc comment with name, description, and templates
	docLines = append(docLines, "  /**")

	// Add name and description in the main comment
	if prompt.Name != "" && prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf("   * %s - %s", prompt.Name, prompt.Description))
	} else if prompt.Name != "" {
		docLines = append(docLines, fmt.Sprintf("   * %s", prompt.Name))
	} else if prompt.Description != "" {
		docLines = append(docLines, fmt.Sprintf("   * %s", prompt.Description))
	} else {
		// Fallback to slug-based name
		docLines = append(docLines, fmt.Sprintf("   * %s", strcase.ToCamel(prompt.Slug)))
	}

	// Add function signatures in the description
	docLines = append(docLines, "   *")
	docLines = append(docLines, "   * Functions:")

	if prompt.System != "" {
		docLines = append(docLines, "   * - system(): Returns the system prompt")
	}
	if prompt.Prompt != "" {
		docLines = append(docLines, "   * - prompt(): Returns the user prompt")
	}

	// Add original templates
	if prompt.System != "" {
		docLines = append(docLines, "   *")
		docLines = append(docLines, "   * System prompt:")
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
		docLines = append(docLines, "   * User prompt:")
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

// getSystemVariableObjects gets Variable objects from the system template
func (cg *CodeGenerator) getSystemVariableObjects(prompt Prompt) []Variable {
	// Parse system template if not already parsed
	systemTemplate := prompt.SystemTemplate
	if len(systemTemplate.Variables) == 0 && prompt.System != "" {
		systemTemplate = ParseTemplate(prompt.System)
	}

	return systemTemplate.Variables
}

// getPromptVariableObjects gets Variable objects from the prompt template
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
		return fmt.Sprintf(`export type %s = { compile: (%s) => string };`,
			typeName, paramStr)
	}

	// Generate JSDoc comment for the type with @memberof
	docstring := cg.generateTemplateDocstring(template)

	return fmt.Sprintf(`/**
%s
 * @memberof %s
 * @type {object}
 */
export type %s = { compile: (%s) => string };`,
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

// generateSystemJSDoc generates JSDoc comment for the system function
func (cg *CodeGenerator) generateSystemJSDoc(prompt Prompt) string {
	if prompt.System == "" {
		return ""
	}

	// Escape the system prompt for JSDoc
	escapedSystem := strings.ReplaceAll(prompt.System, "*/", "*\\/")

	// Clean up the system prompt for single line display but keep variable placeholders
	cleanSystem := strings.ReplaceAll(escapedSystem, "\n", " ")
	cleanSystem = strings.TrimSpace(cleanSystem)

	// Get system variables for parameter documentation
	systemVars := cg.getSystemVariableObjects(prompt)
	allOptional := cg.areAllVariablesOptional(systemVars)

	// Build JSDoc
	var jsdoc strings.Builder
	jsdoc.WriteString("/**\n")
	jsdoc.WriteString(fmt.Sprintf(" * System prompt: %s\n", cleanSystem))

	// Add parameter documentation
	if len(systemVars) > 0 {
		jsdoc.WriteString(" * @param {Object} variables - System prompt variables\n")
		for _, variable := range systemVars {
			if allOptional {
				jsdoc.WriteString(fmt.Sprintf(" * @param {string} [variables.%s] - System prompt variable\n", variable.Name))
			} else {
				jsdoc.WriteString(fmt.Sprintf(" * @param {string} variables.%s - System prompt variable\n", variable.Name))
			}
		}
	}

	jsdoc.WriteString(" * @returns {string} The compiled system prompt\n")
	jsdoc.WriteString(" */\n    ")

	return jsdoc.String()
}

// generatePromptJSDoc generates JSDoc comment for the prompt function
func (cg *CodeGenerator) generatePromptJSDoc(prompt Prompt) string {
	if prompt.Prompt == "" {
		return ""
	}

	// Escape the prompt for JSDoc
	escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "*\\/")

	// Clean up the prompt for single line display
	cleanPrompt := strings.ReplaceAll(escapedPrompt, "\n", " ")
	cleanPrompt = strings.TrimSpace(cleanPrompt)

	// Get prompt variables for parameter documentation
	promptVars := cg.getPromptVariableObjects(prompt)
	allOptional := cg.areAllVariablesOptional(promptVars)

	// Build JSDoc
	var jsdoc strings.Builder
	jsdoc.WriteString("/**\n")
	jsdoc.WriteString(fmt.Sprintf(" * User prompt: %s\n", cleanPrompt))

	// Add parameter documentation
	if len(promptVars) > 0 {
		jsdoc.WriteString(" * @param {Object} variables - User prompt variables\n")
		for _, variable := range promptVars {
			if allOptional {
				jsdoc.WriteString(fmt.Sprintf(" * @param {string} [variables.%s] - User prompt variable\n", variable.Name))
			} else {
				jsdoc.WriteString(fmt.Sprintf(" * @param {string} variables.%s - User prompt variable\n", variable.Name))
			}
		}
	}

	jsdoc.WriteString(" * @returns {string} The compiled user prompt\n")
	jsdoc.WriteString(" */\n    ")

	return jsdoc.String()
}

// generateSystemJSDocForType generates JSDoc comment for the system function in TypeScript types
func (cg *CodeGenerator) generateSystemJSDocForType(prompt Prompt) string {
	if prompt.System == "" {
		return ""
	}

	// Escape the system prompt for JSDoc
	escapedSystem := strings.ReplaceAll(prompt.System, "*/", "*\\/")

	// Clean up the system prompt for single line display but keep variable placeholders
	cleanSystem := strings.ReplaceAll(escapedSystem, "\n", " ")
	cleanSystem = strings.TrimSpace(cleanSystem)

	// Convert slug to the destructured variable name pattern
	variableName := strcase.ToLowerCamel(prompt.Slug) + "System"

	// Build JSDoc with actual prompt content and variable name
	var jsdoc strings.Builder
	jsdoc.WriteString("/**\n")
	jsdoc.WriteString(fmt.Sprintf(" * %s - System prompt: %s\n", variableName, cleanSystem))
	jsdoc.WriteString(" * @param variables - System prompt variables\n")
	jsdoc.WriteString(" * @returns The compiled system prompt string\n")
	jsdoc.WriteString(" */\n")

	return jsdoc.String()
}

// generatePromptJSDocForType generates JSDoc comment for the prompt function in TypeScript types
func (cg *CodeGenerator) generatePromptJSDocForType(prompt Prompt) string {
	if prompt.Prompt == "" {
		return ""
	}

	// Escape the prompt for JSDoc
	escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "*\\/")

	// Clean up the prompt for single line display but keep variable placeholders
	cleanPrompt := strings.ReplaceAll(escapedPrompt, "\n", " ")
	cleanPrompt = strings.TrimSpace(cleanPrompt)

	// Convert slug to the destructured variable name pattern
	variableName := strcase.ToLowerCamel(prompt.Slug) + "Prompt"

	// Build JSDoc with actual prompt content and variable name
	var jsdoc strings.Builder
	jsdoc.WriteString("/**\n")
	jsdoc.WriteString(fmt.Sprintf(" * %s - User prompt: %s\n", variableName, cleanPrompt))
	jsdoc.WriteString(" * @param variables - User prompt variables\n")
	jsdoc.WriteString(" * @returns The compiled user prompt string\n")
	jsdoc.WriteString(" */\n")

	return jsdoc.String()
}

// generateTypedefJSDoc generates JSDoc typedef for the prompt type
func (cg *CodeGenerator) generateTypedefJSDoc(prompt Prompt) string {
	var jsdoc strings.Builder
	jsdoc.WriteString("/**\n")
	jsdoc.WriteString(" * ")
	jsdoc.WriteString(prompt.Name)
	jsdoc.WriteString(" - ")
	jsdoc.WriteString(prompt.Description)
	jsdoc.WriteString("\n * @typedef {Object} ")
	jsdoc.WriteString(strcase.ToCamel(prompt.Slug))
	jsdoc.WriteString("\n * @property {string} slug - The prompt slug\n")

	if prompt.System != "" {
		jsdoc.WriteString(" * @property {Function} system - System prompt function\n")
		// Add system prompt content
		escapedSystem := strings.ReplaceAll(prompt.System, "*/", "*\\/")
		lines := strings.Split(escapedSystem, "\n")
		for _, line := range lines {
			wrapped := cg.wrapLine(line, 80)
			for _, wrappedLine := range wrapped {
				jsdoc.WriteString(" *   System: ")
				jsdoc.WriteString(wrappedLine)
				jsdoc.WriteString("\n")
			}
		}
	}

	if prompt.Prompt != "" {
		jsdoc.WriteString(" * @property {Function} prompt - User prompt function\n")
		// Add user prompt content
		escapedPrompt := strings.ReplaceAll(prompt.Prompt, "*/", "*\\/")
		lines := strings.Split(escapedPrompt, "\n")
		for _, line := range lines {
			wrapped := cg.wrapLine(line, 80)
			for _, wrappedLine := range wrapped {
				jsdoc.WriteString(" *   Prompt: ")
				jsdoc.WriteString(wrappedLine)
				jsdoc.WriteString("\n")
			}
		}
	}

	jsdoc.WriteString(" */\n")

	return jsdoc.String()
}
