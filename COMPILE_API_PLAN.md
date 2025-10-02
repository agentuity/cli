# Compile API Logic Plan

## Current Issue
The `compile()` method currently requires a `variables` parameter even for prompts with no variables, but it should follow the same logic as individual prompt functions:

## Required Logic for `compile()` Method

### 1. No Variables
- **Current**: `ctx.prompts.compile('simple-helper', {})` (requires empty object)
- **Should be**: `ctx.prompts.compile('simple-helper')` (no parameters needed)

### 2. All Required Variables  
- **Current**: `ctx.prompts.compile('required-only', { system: {...}, prompt: {...} })` (optional)
- **Should be**: `ctx.prompts.compile('required-only', { system: {...}, prompt: {...} })` (required)

### 3. All Optional Variables
- **Current**: `ctx.prompts.compile('optional-only', { system: {...}, prompt: {...} })` (optional)
- **Should be**: `ctx.prompts.compile('optional-only', { system: {...}, prompt: {...} })` (optional)

### 4. Mixed Variables
- **Current**: `ctx.prompts.compile('mixed', { system: {...}, prompt: {...} })` (optional)
- **Should be**: `ctx.prompts.compile('mixed', { system: {...}, prompt: {...} })` (optional)

## Implementation Plan

### Step 1: Update Code Generator
- Modify `generatePromptTypeExports()` in `code_generator.go`
- Add logic to determine if `compile()` should require variables
- Generate different TypeScript signatures based on prompt variable requirements

### Step 2: Update SDK Implementation
- Modify `compile()` method in `sdk-js/src/apis/prompt/index.ts`
- Make `variables` parameter conditional based on prompt requirements
- Handle cases where no variables are needed

### Step 3: Update Type Definitions
- Update TypeScript types to reflect conditional parameter requirements
- Ensure IntelliSense works correctly for all scenarios

### Step 4: Update Test Agent
- Modify test cases to use the new API patterns
- Test all four scenarios (no vars, all required, all optional, mixed)

## Logic Rules
- **No variables in system AND prompt**: `compile(name)` - no parameters
- **Any required variables in system OR prompt**: `compile(name, variables)` - required parameters  
- **Only optional variables**: `compile(name, variables?)` - optional parameters

## Files to Modify
1. `/Users/bobby/Code/platform/cli/internal/bundler/prompts/code_generator.go`
2. `/Users/bobby/Code/platform/sdk-js/src/apis/prompt/index.ts`
3. `/Users/bobby/Code/agents/test-prompt-compile/src/agents/my-agent/index.ts`
