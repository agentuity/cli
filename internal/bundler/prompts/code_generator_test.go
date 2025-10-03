package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodeGenerator(t *testing.T) {
	prompts := []Prompt{
		{
			Name:        "Test Prompt 1",
			Slug:        "test-prompt-1",
			Description: "A test prompt",
			System:      "You are a {role:assistant} specializing in {!domain}.",
			Prompt:      "Help the user with {task:their question}.",
		},
		{
			Name:        "Test Prompt 2",
			Slug:        "test-prompt-2",
			Description: "Another test prompt",
			Prompt:      "Complete this {!task} for the user.",
		},
	}

	codeGen := NewCodeGenerator(prompts)

	t.Run("GenerateJavaScript", func(t *testing.T) {
		js := codeGen.GenerateJavaScript()

		// Check that it contains the import
		assert.Contains(t, js, "import { interpolateTemplate } from '../../../index.js';")

		// Check that it contains the prompts object
		assert.Contains(t, js, "export const prompts = {")

		// Check that it contains both prompt objects
		assert.Contains(t, js, "const testPrompt1 = {")
		assert.Contains(t, js, "const testPrompt2 = {")

		// Check that it contains variables parameter (no TypeScript types)
		assert.Contains(t, js, "variables")

		// Check that it contains function signatures
		assert.Contains(t, js, "system: /**")
		assert.Contains(t, js, "prompt: /**")
		assert.Contains(t, js, "interpolateTemplate(")

		// Ensure no TypeScript syntax in JavaScript
		assert.NotContains(t, js, ": string", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "variables?: {", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "export type", "JavaScript should not contain TypeScript type definitions")
		assert.NotContains(t, js, "interface ", "JavaScript should not contain TypeScript interfaces")
	})

	t.Run("GenerateTypeScriptTypes", func(t *testing.T) {
		types := codeGen.GenerateTypeScriptTypes()

		// Check that it contains the import
		assert.Contains(t, types, "import { interpolateTemplate, Prompt } from '@agentuity/sdk';")

		// Check that it contains the prompts object
		assert.Contains(t, types, "export const prompts: PromptsCollection = {} as any;")

		// Check that it contains both prompt types
		assert.Contains(t, types, "TestPrompt1")
		assert.Contains(t, types, "TestPrompt2")

		// Check that it contains variable types with proper optional/default syntax
		assert.Contains(t, types, "role?: string | \"assistant\"")
		assert.Contains(t, types, "domain: string")
		assert.Contains(t, types, "task?: string | \"their question\"")
	})
}

func TestCodeGenerator_EmptyPrompts(t *testing.T) {
	codeGen := NewCodeGenerator([]Prompt{})

	t.Run("GenerateJavaScript", func(t *testing.T) {
		js := codeGen.GenerateJavaScript()
		assert.Contains(t, js, "export const prompts = {")
		assert.Contains(t, js, "};")

		// Ensure no TypeScript syntax in JavaScript
		assert.NotContains(t, js, ": string", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "variables?: {", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "export type", "JavaScript should not contain TypeScript type definitions")
		assert.NotContains(t, js, "interface ", "JavaScript should not contain TypeScript interfaces")
	})

	t.Run("GenerateTypeScriptTypes", func(t *testing.T) {
		types := codeGen.GenerateTypeScriptTypes()
		assert.Contains(t, types, "export const prompts: PromptsCollection = {} as any;")
	})

}

func TestCodeGenerator_SingleFieldPrompts(t *testing.T) {
	prompts := []Prompt{
		{
			Name:   "System Only",
			Slug:   "system-only",
			System: "You are a {role:assistant}.",
		},
		{
			Name:   "Prompt Only",
			Slug:   "prompt-only",
			Prompt: "Help with {task:the task}.",
		},
	}

	codeGen := NewCodeGenerator(prompts)

	t.Run("GenerateJavaScript", func(t *testing.T) {
		js := codeGen.GenerateJavaScript()

		// Check that it contains the correct function signatures
		assert.Contains(t, js, "system: /**")
		assert.Contains(t, js, "prompt: /**")
		assert.Contains(t, js, "interpolateTemplate(")
		assert.Contains(t, js, "slug:")

		// Ensure no TypeScript syntax in JavaScript
		assert.NotContains(t, js, ": string", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "variables?: {", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "export type", "JavaScript should not contain TypeScript type definitions")
		assert.NotContains(t, js, "interface ", "JavaScript should not contain TypeScript interfaces")
	})

	t.Run("GenerateTypeScriptTypes", func(t *testing.T) {
		types := codeGen.GenerateTypeScriptTypes()

		// Check that both prompts have string return types
		assert.Contains(t, types, "=> string")
	})
}

func TestCodeGenerator_ComplexPrompts(t *testing.T) {
	prompts := []Prompt{
		{
			Name:        "Complex Prompt",
			Slug:        "complex-prompt",
			Description: "A complex prompt with both system and prompt",
			System:      "You are a {role:helpful assistant} specializing in {!domain}.\nYour experience level is {experience:intermediate}.",
			Prompt:      "Help the user with: {task:their question}\nUse a {approach:detailed} approach.\nPriority: {priority:normal}",
		},
	}

	codeGen := NewCodeGenerator(prompts)

	t.Run("GenerateJavaScript", func(t *testing.T) {
		js := codeGen.GenerateJavaScript()

		// Check that it handles multiline templates correctly
		assert.Contains(t, js, "interpolateTemplate(\"You are a {role:helpful assistant} specializing in {!domain}.\\nYour experience level is {experience:intermediate}.\", { role, domain, experience })")
		assert.Contains(t, js, "interpolateTemplate(\"Help the user with: {task:their question}\\nUse a {approach:detailed} approach.\\nPriority: {priority:normal}\", { task, approach, priority })")

		// Check that it contains the correct object structure
		assert.Contains(t, js, "const complexPrompt = {")
		assert.Contains(t, js, "system: /**")
		assert.Contains(t, js, "prompt: /**")
		assert.Contains(t, js, "interpolateTemplate(")
		assert.Contains(t, js, "slug:")

		// Ensure no TypeScript syntax in JavaScript
		assert.NotContains(t, js, ": string", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "variables?: {", "JavaScript should not contain TypeScript type annotations")
		assert.NotContains(t, js, "export type", "JavaScript should not contain TypeScript type definitions")
		assert.NotContains(t, js, "interface ", "JavaScript should not contain TypeScript interfaces")
	})

	t.Run("GenerateTypeScriptTypes", func(t *testing.T) {
		types := codeGen.GenerateTypeScriptTypes()

		// Check that it has the correct object structure for complex prompts
		assert.Contains(t, types, "system: (variables:")
		assert.Contains(t, types, "prompt: (variables?:")
		assert.Contains(t, types, "slug: string;")

		// Check that it includes all variables with proper optional/default syntax
		assert.Contains(t, types, "variables?: {")
		assert.Contains(t, types, "role?: string | \"helpful assistant\"")
		assert.Contains(t, types, "domain: string")
		assert.Contains(t, types, "experience?: string | \"intermediate\"")
		assert.Contains(t, types, "task?: string | \"their question\"")
		assert.Contains(t, types, "approach?: string | \"detailed\"")
		assert.Contains(t, types, "priority?: string | \"normal\"")
	})
}

func TestCodeGenerator_VariableTypes(t *testing.T) {
	prompts := []Prompt{
		{
			Name:   "Variable Test",
			Slug:   "variable-test",
			System: "You are a {{legacy}} {new:default} {!required} assistant.",
		},
	}

	codeGen := NewCodeGenerator(prompts)

	t.Run("GenerateVariableTypes", func(t *testing.T) {
		types := codeGen.GenerateTypeScriptTypes()

		// Check that it includes all variable types with proper optional/default syntax
		assert.Contains(t, types, "legacy?: string")
		assert.Contains(t, types, "new?: string | \"default\"")
		assert.Contains(t, types, "required: string")
	})
}
