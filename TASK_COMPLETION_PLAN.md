# Task Completion Plan: Type-Safe Prompt Compilation

## Current State Analysis

### What's Working ✅
- CLI generates JavaScript and TypeScript files correctly
- SDK loads generated content dynamically
- Basic prompt compilation works with `ctx.prompts.compile()`
- Slug-based naming is implemented (`'simple-helper'` instead of camelCase)

### What Needs Fixing ❌
- **Type Safety Issue**: The `compile()` function doesn't provide proper TypeScript feedback
- **Parameter Structure**: Should conditionally include `system` and `prompt` parameters based on template content
- **Variable Requirements**: Should distinguish between required, optional, and no variables

## Required Changes

### 1. Type-Safe Compile Function Signature

The `compile()` function should have conditional parameters based on prompt template analysis:

```typescript
// Current (incorrect):
compile(slug: string, params: any): string

// Should be (conditional based on template):
compile<T extends keyof GeneratedPromptsCollection>(
  slug: T, 
  params: CompileParams<T>
): string

// Where CompileParams<T> is conditionally typed:
type CompileParams<T> = 
  T extends 'required-variables-only' 
    ? { system: { role: string; specialization: string }; prompt: { task: string; quality: string } }
  : T extends 'optional-with-defaults'
    ? { system?: { role?: string; domain?: string; experience?: string }; prompt?: { task?: string; tone?: string } }
  : T extends 'simple-helper'
    ? never // No parameters needed
  : any; // Fallback
```

### 2. Template Analysis Requirements

Need to analyze each prompt template to determine:
- **System variables**: Required (`{!var}`), Optional (`{var:default}`), or None
- **Prompt variables**: Required (`{!var}`), Optional (`{var:default}`), or None
- **Parameter structure**: Include `system`/`prompt` only if they have variables

### 3. Generated Type Structure

The CLI should generate TypeScript types that reflect the actual template requirements:

```typescript
// For 'required-variables-only':
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

// For 'optional-with-defaults':
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

// For 'simple-helper':
// No parameters interface needed (no parameters at all)
```

## Implementation Plan

### Phase 1: Template Analysis (CLI)
1. **Extend `code_generator.go`** to analyze prompt templates
2. **Parse variable syntax**:
   - `{!variable}` = required
   - `{variable:default}` = optional with default
   - `{variable}` = optional without default
3. **Generate parameter interfaces** for each prompt
4. **Create conditional compile signature** types

### Phase 2: Type Generation (CLI)
1. **Generate TypeScript interfaces** for each prompt's parameters
2. **Create union types** for compile function parameters
3. **Update generated `index.d.ts`** with proper type definitions
4. **Ensure backward compatibility** with existing code

### Phase 3: SDK Integration (SDK)
1. **Update `AgentContext`** to use generated types
2. **Implement type-safe compile function** in server context
3. **Add proper error handling** for missing required parameters
4. **Test with existing agent code**

### Phase 4: Testing & Validation
1. **Test all prompt scenarios** from `prompts.yaml`
2. **Verify TypeScript intellisense** works correctly
3. **Ensure runtime behavior** matches type expectations
4. **Update documentation** with new patterns

## Files to Modify

### CLI Files:
- `internal/bundler/prompts/code_generator.go` - Template analysis & type generation
- `internal/bundler/prompts/prompts.go` - Orchestration updates

### SDK Files:
- `src/types.ts` - Update AgentContext compile signature
- `src/server/server.ts` - Implement type-safe compile function
- `src/apis/prompt/index.ts` - Export generated types

### Test Files:
- `test-prompt-compile/src/agents/my-agent/index.ts` - Update test cases

## Success Criteria

1. **Type Safety**: `ctx.prompts.compile()` provides accurate TypeScript intellisense
2. **Conditional Parameters**: Only required parameters are included in function signature
3. **Runtime Validation**: Missing required parameters throw clear errors
4. **Backward Compatibility**: Existing code continues to work
5. **Performance**: No significant impact on compilation or runtime performance

## Reference Implementation

Look at the current working `getPrompt()` function for inspiration on how conditional types should work. The `compile()` function should follow a similar pattern but with more complex parameter analysis.

## Next Steps

1. Start with Phase 1 (Template Analysis)
2. Test each phase incrementally
3. Maintain backward compatibility throughout
4. Update documentation as you go
5. Test with the existing `test-prompt-compile` project
