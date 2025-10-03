# Coding Agent Prompt: Type-Safe Prompt Compilation

## Context

You are working on a code generation system where a CLI (Go) generates TypeScript/JavaScript code that gets consumed by an SDK (TypeScript). The system generates prompt templates with variable interpolation, and we need to make the `compile()` function type-safe.

## Current Problem

The `ctx.prompts.compile()` function currently accepts `any` parameters, but it should provide proper TypeScript intellisense based on the actual prompt template requirements.

## What's Already Working

1. **CLI generates code correctly** - `internal/bundler/prompts/code_generator.go`
2. **SDK loads generated content** - `src/apis/prompt/index.ts`
3. **Basic compilation works** - `ctx.prompts.compile('slug', params)`
4. **Slug-based naming** - Uses `'simple-helper'` instead of camelCase

## What Needs to be Fixed

The `compile()` function should have conditional TypeScript signatures based on prompt template analysis:

### Example Requirements

Given these prompt templates from `test-prompt-compile/src/prompts.yaml`:

```yaml
# Required variables only
- slug: required-variables-only
  system: |
    You are a {!role} assistant.
    Your specialization is {!specialization}.
  prompt: |
    Complete the {!task} with {!quality} quality.

# Optional variables with defaults  
- slug: optional-with-defaults
  system: |
    You are a {role:helpful assistant} specializing in {domain:general topics}.
    Your experience level is {experience:intermediate}
  prompt: |
    Help the user with: {task:their question}
    Use a {tone:friendly} approach.

# No variables
- slug: simple-helper
  system: |
    You are a helpful assistant that provides clear and concise answers.
  prompt: |
    Please help the user with their question.
```

### Expected TypeScript Behavior

```typescript
// For 'required-variables-only' - should require system and prompt with specific fields
ctx.prompts.compile('required-variables-only', {
  system: { role: string, specialization: string },
  prompt: { task: string, quality: string }
});

// For 'optional-with-defaults' - should make system and prompt optional with optional fields
ctx.prompts.compile('optional-with-defaults', {
  system?: { role?: string, domain?: string, experience?: string },
  prompt?: { task?: string, tone?: string }
});

// For 'simple-helper' - should require no parameters
ctx.prompts.compile('simple-helper');
```

## Implementation Approach

### 1. Template Analysis (CLI)

Extend `internal/bundler/prompts/code_generator.go` to:

1. **Parse variable syntax**:
   - `{!variable}` = required variable
   - `{variable:default}` = optional variable with default
   - `{variable}` = optional variable without default

2. **Analyze each prompt**:
   - Extract system variables and their requirements
   - Extract prompt variables and their requirements
   - Determine if system/prompt parameters should be included

3. **Generate TypeScript interfaces**:
   - Create parameter interfaces for each prompt
   - Generate conditional compile signature types

### 2. Type Generation (CLI)

Generate TypeScript code that creates:

```typescript
// Parameter interfaces for each prompt
export interface RequiredVariablesOnlyParams {
  system: {
    role: string;
    specialization: string;
  };
  prompt: {
    task: string;
    quality: string;
  };
}

export interface OptionalWithDefaultsParams {
  system?: {
    role?: string;
    domain?: string;
    experience?: string;
  };
  prompt?: {
    task?: string;
    tone?: string;
  };
}

// Conditional compile signature
export type CompileParams<T extends keyof GeneratedPromptsCollection> = 
  T extends 'required-variables-only' ? RequiredVariablesOnlyParams :
  T extends 'optional-with-defaults' ? OptionalWithDefaultsParams :
  T extends 'simple-helper' ? never :
  any;

// Updated compile function signature
compile<T extends keyof GeneratedPromptsCollection>(
  slug: T,
  params: CompileParams<T>
): string;
```

### 3. SDK Integration (SDK)

Update `src/types.ts` and `src/server/server.ts` to:

1. **Use generated types** for compile function signature
2. **Implement runtime validation** for required parameters
3. **Maintain backward compatibility**

## Key Files to Focus On

### CLI Files:
- `internal/bundler/prompts/code_generator.go` - Main logic for template analysis and type generation
- `internal/bundler/prompts/prompts.go` - Orchestration

### SDK Files:
- `src/types.ts` - Update AgentContext compile signature
- `src/server/server.ts` - Implement type-safe compile function

### Test Files:
- `test-prompt-compile/src/agents/my-agent/index.ts` - Current test cases

## Reference Implementation

Look at how `getPrompt()` currently works for inspiration on conditional types. The `compile()` function should follow a similar pattern but with more complex parameter analysis.

## Success Criteria

1. **TypeScript intellisense** shows correct parameters for each prompt slug
2. **Required parameters** are enforced at compile time
3. **Optional parameters** are properly marked as optional
4. **No parameters** prompts work with empty object
5. **Runtime validation** throws clear errors for missing required parameters
6. **Backward compatibility** maintained for existing code

## Testing

Use the existing test project at `test-prompt-compile/` to verify your changes work correctly. The test cases in `src/agents/my-agent/index.ts` should provide good examples of expected behavior.

## Important Notes

- **Don't break existing functionality** - maintain backward compatibility
- **Use the existing patterns** - follow the shell-based approach already implemented
- **Test incrementally** - verify each change works before moving to the next
- **Keep it simple** - avoid overly complex generics that cause compilation issues

## Getting Started

1. Start by analyzing the template parsing logic in `code_generator.go`
2. Add functions to extract variable requirements from system/prompt templates
3. Generate the appropriate TypeScript interfaces
4. Update the SDK to use the generated types
5. Test with the existing test project

The goal is to make `ctx.prompts.compile()` as type-safe and user-friendly as possible while maintaining the existing architecture and patterns.
