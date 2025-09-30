# Prompt ORM Migration Plan

## Overview

Migrate the proof-of-concept prompt-orm functionality into production-ready Go CLI with seamless SDK integration.

## Current State Analysis

### prompt-orm (POC)
- **Example**: `/Users/bobby/Code/prompt-orm/`
- **Language**: TypeScript/JavaScript
- **Core Feature**: YAML-to-TypeScript code generation
- **Variable System**: `{{variable}}` template substitution
- **Output**: Type-safe prompt methods with compile() functions
- **SDK Integration**: `orm.prompts.promptName.compile(variables)`

### Production Components

#### CLI (Go)
- **Example**: `/Users/bobby/Code/prompt-orm/my-ink-cli`
- **Location**: `/Users/bobby/Code/platform/cli`
- **Current Features**: Project management, agent lifecycle, bundling, deployment
- **Target Integration**: Bundle command (`agentuity bundle` / `agentuity dev`)

#### SDK (TypeScript)
- **Example**: `/Users/bobby/Code/platform/sdk-js/src/apis/prompt.ts`
- **Location**: `/Users/bobby/Code/platform/sdk-js`
- **Current State**: Placeholder PromptAPI class
- **Target**: `context.prompts` accessibility in agents

#### Sample Project (env1)
- **Example**: `/Users/bobby/Code/agents/env1/src/prompts.yaml`
- **Location**: `/Users/bobby/Code/agents/env1`
- **Current**: Basic agent with static prompts
- **Target**: Dynamic prompt compilation with variables

## POC vs Plan Analysis

### 1. YAML Schema Comparison ✅

**POC Schema (working implementation):**
```yaml
prompts:
  - name: "Hello World"
    slug: "hello-world"
    description: "A simple hello world prompt"
    prompt: "Hello, world!"
    system: "You are a helpful assistant." # optional
```

**Plan Schema (proposed):**
```yaml
prompts:
  - slug: copy-writer
    name: Copy Writer
    description: Takes a user input and turns it into a Tweet
    system: |
      You are a helpful assistant...
    prompt: |
      The user wants to write a tweet about: {{topic}}
    evals: ['professionalism'] # NEW - not in POC
```

**Key Differences:**
- ✅ Core fields identical: `name`, `slug`, `description`, `prompt`, `system`
- ⚠️ Field order: POC has `name` first, plan has `slug` first  
- ⚠️ `evals` field: Plan adds this, POC doesn't have it
- ✅ Variable system: Both use `{{variable}}` syntax
- ⚠️ Multiline: Plan uses `|` YAML syntax, POC uses quotes

### 2. Code Generation Comparison ✅

**POC Generation (TypeScript CLI):**
- **File**: `/Users/bobby/Code/prompt-orm/my-ink-cli/source/code-generator.ts`
- **Pattern**: Single `compile()` function returning `{ prompt: string, system?: string, variables: object }`
- **Variable Extraction**: Scans both `prompt` and `system` fields for `{{var}}`
- **Type Generation**: Creates unified variable interfaces

**Plan Generation (Go CLI):**
- **Pattern**: Separate `system.compile()` and `prompt.compile()` functions each returning `string`
- **Variable Extraction**: Same scanning approach needed
- **Type Generation**: Same unified variable interfaces needed

**Key Differences:**
- ⚠️ **Interface Split**: Plan splits system/prompt compilation, POC combines them
- ✅ **Variable Logic**: Same template replacement logic works for both
- ✅ **Type Safety**: Both achieve strong typing without optional chaining

### 3. SDK Integration Comparison 📋

**POC SDK Pattern:**
```typescript
// POC Usage
const orm = new PromptORM();
const result = orm.prompts.helloWorld.compile({ name: "Bobby" });
// Returns: { prompt: "Hello, Bobby!", system?: "...", variables: {...} }
```

**Plan SDK Pattern:**
```typescript  
// Plan Usage
const prompts = ctx.prompts();
const systemMsg = prompts.copyWriter.system.compile({ topic: "AI" });
const promptMsg = prompts.copyWriter.prompt.compile({ topic: "AI" });
// Each returns: string (no optional chaining needed)
```

**Key Differences:**
- ⚠️ **Return Type**: POC returns object, Plan returns separate strings
- ⚠️ **Context**: POC uses `PromptORM` class, Plan uses `AgentContext.prompts()`
- ✅ **Type Safety**: Both avoid optional chaining through different approaches

### 4. CLI Integration Comparison 🔧

**POC CLI Approach:**
- **Language**: TypeScript + Ink React CLI
- **Command**: `npm run generate` (external script)
- **Integration**: Standalone CLI that modifies SDK files in `node_modules`
- **Target**: Modifies `/sdk/src/generated/index.ts`

**Plan CLI Approach:**
- **Language**: Go (integrated into existing CLI)
- **Command**: `agentuity bundle` (integrated command)
- **Integration**: Built into existing bundler pipeline
- **Target**: Creates `src/generated/prompts.ts` in project

**Key Differences:**
- ✅ **Integration**: Plan approach is more integrated into existing workflow
- ⚠️ **Language**: POC TypeScript vs Plan Go - need to port generation logic
- ✅ **Bundler Integration**: Plan integrates with existing esbuild pipeline

### 5. Analysis Summary & Recommendations 🎯

**What Works Well in POC (Keep):**
- ✅ YAML schema structure is solid
- ✅ Variable extraction logic with `{{variable}}` syntax  
- ✅ TypeScript type generation approach
- ✅ Template replacement regex: `/\{\{([^}]+)\}\}/g`
- ✅ Unified variable interface generation

**Key Adaptations Needed for Production:**

1. **Interface Decision**: Our plan's split system/prompt interface is better than POC's combined approach:
   ```typescript
   // Better (Plan): Clean separation, clear return types
   prompts.copyWriter.system.compile({ topic }) → string
   prompts.copyWriter.prompt.compile({ topic }) → string
   
   // POC: Mixed object return, less clear usage
   prompts.copyWriter.compile({ topic }) → { prompt: string, system?: string, variables: object }
   ```

2. **Code Generation Logic to Port from POC:**
   - **File**: `/Users/bobby/Code/prompt-orm/my-ink-cli/source/code-generator.ts` (lines 10-49)
   - **Function**: `extractVariables()` - Variable regex extraction  
   - **Function**: `escapeTemplateString()` - String escaping for templates
   - **Logic**: Template replacement in compile functions

3. **YAML Parsing to Port from POC:**
   - **File**: `/Users/bobby/Code/prompt-orm/my-ink-cli/source/prompt-parser.ts` (lines 17-48)
   - **Validation**: Required field checking
   - **Structure**: Array-based prompts format

4. **Updated YAML Schema (Harmonized):**
   ```yaml
   prompts:
     - slug: copy-writer          # Keep slug-first from plan
       name: Copy Writer          # Core field from POC
       description: Takes a user input and turns it into a Tweet  # Core field from POC
       system: |                  # Support multiline from plan
         You are a helpful assistant...
       prompt: |                  # Support multiline from plan  
         The user wants to write a tweet about: {{topic}}
       evals: ['professionalism'] # Optional - new in plan
   ```

## Migration Strategy

**Test Validation**: See `/Users/bobby/Code/platform/cli/test-production-flow.sh` for end-to-end validation script

### Phase 1: Bundle Command Integration 🔄

**Goal**: Integrate prompt generation directly into the existing bundle workflow

#### 1.1 Create Prompt Processing Module
- **Example**: `/Users/bobby/Code/prompt-orm/my-ink-cli/source/prompt-parser.ts`
- **File**: `internal/bundler/prompts.go`
- **Port from POC**: Variable extraction logic from `code-generator.ts` lines 119-124
- **Responsibilities**:
  - Parse `prompts.yaml` files (similar to POC `parsePromptsYaml()`)
  - Extract `{{variable}}` templates using POC regex: `/\{\{([^}]+)\}\}/g`
  - Generate TypeScript prompt definitions with split compile functions
  - Integrate with existing bundler pipeline

#### 1.2 Extend Bundle Command
- **Integration Point**: Existing `agentuity bundle` command
- **Behavior**: 
  - Auto-detect `src/prompts.yaml` during bundling
  - Generate `src/generated/prompts.ts` before compilation
  - Include generated files in bundle output
- **Variable Extraction**: Parse both `system` and `prompt` fields for `{{variable}}` patterns
- **Type Generation**: Create unified TypeScript interface for all variables across system and prompt

#### 1.3 YAML Schema Support
```yaml
prompts:
  - slug: copy-writer
    name: Copy Writer
    description: Takes a user input and turns it into a Tweet
    system: |
      You are a helpful assistant that writes tweets. They should be simple, plain language, approachable, and engaging.
      
      Only provide the one single tweet, no other text.
    prompt: |
      The user wants to write a tweet about: {{topic}}
    evals: ['professionalism']
```

#### 1.4 Generated TypeScript Output
**Based on POC generation logic but adapted for our split interface:**
```typescript
export const prompts = {
  copyWriter: {
    slug: "copy-writer",
    name: "Copy Writer", 
    description: "Takes a user input and turns it into a Tweet",
    evals: ['professionalism'], // Optional field
    system: {
      compile(variables: { topic: string }) {
        // Using POC's escapeTemplateString() logic ported to Go
        const template = "You are a helpful assistant that writes tweets. They should be simple, plain language, approachable, and engaging.\\n\\nOnly provide the one single tweet, no other text.";
        // Using POC's variable replacement regex
        return template.replace(/\\{\\{([^}]+)\\}\\}/g, (match, varName) => {
          return variables[varName] || match;
        });
      }
    },
    prompt: {
      compile(variables: { topic: string }) {
        const template = "The user wants to write a tweet about: {{topic}}";
        // Same regex as POC: /\{\{([^}]+)\}\}/g  
        return template.replace(/\\{\\{([^}]+)\\}\\}/g, (match, varName) => {
          return variables[varName] || match;
        });
      }
    }
  }
};

// Export function that SDK will use (POC pattern adapted)
// Note: All compile functions return string (never undefined/null)
// This ensures no optional chaining is needed in agent code
export function createPromptsAPI() {
  return prompts;
}
```

### Phase 2: SDK Restructuring 🔧

**Goal**: Modify SDK to expose prompts via `context.prompts`

#### 2.1 Update AgentContext Type
- **File**: `src/types.ts`
- **Change**: Add `prompts()` function to AgentContext interface

#### 2.2 Replace PromptAPI Implementation
- **Example**: `/Users/bobby/Code/platform/sdk-js/src/apis/prompt.ts`
- **File**: `src/apis/prompt.ts`
- **Change**: 
  - Load generated prompts from `./generated/prompts.js`
  - Implement `prompts()` function that returns the loaded prompts object
  - Maintain same interface pattern as existing APIs

#### 2.3 Runtime Integration
- **Example**: `/Users/bobby/Code/platform/sdk-js/src/server/server.ts#L183`
- **File**: `src/server/server.ts`
- **Change**: Initialize context prompts with loaded prompt definitions

### Phase 3: Agent Development Experience 🚀

**Goal**: Seamless prompt usage in agent code

#### 3.1 Agent Usage Pattern
- **Example**: `/Users/bobby/Code/agents/env1/src/agents/my-agent/index.ts`
- **Type Safety**: No optional chaining or assertions required - prompts are guaranteed to exist
- **Validation**: See `test-production-flow.sh` for type safety verification
```typescript
export default async function Agent(req: AgentRequest, resp: AgentResponse, ctx: AgentContext) {
  // Get prompts object - guaranteed to exist, no optional chaining needed
  const prompts = ctx.prompts();
  
  // Compile system and prompt separately - strongly typed, no assertions needed
  const topic = await req.data.text();
  const systemMessage = prompts.copyWriter.system.compile({ topic });
  const promptMessage = prompts.copyWriter.prompt.compile({ topic });
  
  // Both return string (not string | undefined), safe to use directly
  const result = await streamText({
    model: groq('llama-3.1-8b-instant'),
    prompt: promptMessage,
    system: systemMessage
  });
  
  return resp.stream(result.textStream, 'text/markdown');
}
```

#### 3.2 Development Workflow
1. **Edit** `src/prompts.yaml`
2. **Run** `agentuity dev` (auto-regenerates prompts)
3. **Code** agents with `ctx.prompts().promptName.system.compile()` and `ctx.prompts().promptName.prompt.compile()`
4. **Test** with hot-reload support

### Phase 4: Production Features 🏗️

#### 4.1 Watch Mode Integration
- **Integration**: Existing file watcher in `agentuity dev`
- **Behavior**: Regenerate prompts on YAML changes
- **Hot Reload**: Update running development server

#### 4.2 Type Safety Enhancements
- **Generated Types**: TypeScript interfaces for prompt variables
- **IDE Support**: Full autocomplete and type checking
- **Validation**: Compile-time variable requirement verification
- **No Optional Chaining**: All prompts guaranteed to exist, compile() always returns string
- **No Type Assertions**: Strong typing eliminates need for `as string` or `!` assertions

#### 4.3 Error Handling & Validation
- **YAML Validation**: Comprehensive schema validation
- **Variable Validation**: Required vs optional variables
- **Build Integration**: Fail builds on invalid prompts

## Implementation Details

### Bundle Command Integration Points

#### Existing Bundle Flow
```
agentuity bundle
├── Parse agentuity.yaml
├── Detect bundler (bunjs/nodejs)
├── Run bundler-specific logic
├── Process imports/dependencies
├── Apply patches
└── Generate bundle output
```

#### Enhanced Bundle Flow
```
agentuity bundle
├── Parse agentuity.yaml
├── Detect bundler (bunjs/nodejs)
├── 🆕 Generate prompts (if prompts.yaml exists)
├── Run bundler-specific logic
├── Process imports/dependencies
├── Apply patches
└── Generate bundle output (including generated prompts)
```

### File Generation Strategy

#### Generated File Structure
```
src/
├── agents/
│   └── my-agent/
│       └── index.ts
├── prompts.yaml          # Source definitions
└── generated/            # Auto-generated (gitignored)
    ├── prompts.ts        # TypeScript definitions
    ├── prompts.js        # Compiled JavaScript
    └── types.ts          # TypeScript interfaces
```

#### SDK Integration Points
```
SDK Context Creation
├── Load agent code
├── 🆕 Load generated prompts
├── Initialize context.prompts
└── Execute agent function
```

## Development Steps

**Validation Script**: `/Users/bobby/Code/platform/cli/test-production-flow.sh`

### Step 1: Create Prompt Bundler Module
- [ ] `internal/bundler/prompts.go` - Core prompt processing
- [ ] YAML parsing with variable extraction from both system and prompt fields
- [ ] TypeScript/JavaScript code generation with separate compile functions
- [ ] Variable type generation (TypeScript interfaces)
- [ ] Ensure all compile functions return `string` (never undefined/null)
- [ ] Integration with existing bundler pipeline

### Step 2: Extend Bundle Command
- [ ] Auto-detect `prompts.yaml` files
- [ ] Call prompt generation during bundle process
- [ ] Handle errors and validation

### Step 3: Update SDK
- [ ] Modify AgentContext type definition to include prompts() function
- [ ] Update PromptAPI to implement prompts() function with generated prompts
- [ ] Integrate prompts into context initialization

### Step 4: Update env1 Sample
- [ ] Create comprehensive `src/prompts.yaml`
- [ ] Update agent code to use `ctx.prompts()` interface
- [ ] Verify no optional chaining or assertions needed
- [ ] Test full workflow with separate system/prompt compilation

### Step 5: Add Development Features
- [ ] Watch mode for prompt regeneration
- [ ] Error handling and validation
- [ ] Type safety improvements
- [ ] Run `test-production-flow.sh` to validate end-to-end workflow

## Success Criteria

### Core Functionality ✅
- [ ] YAML parsing with variable extraction
- [ ] TypeScript code generation from Go with separate compile functions
- [ ] Bundle command integration
- [ ] SDK context.prompts() function accessibility
- [ ] Agent usage pattern working with separate system/prompt compilation

### Developer Experience ✅
- [ ] `agentuity dev` auto-regenerates prompts
- [ ] Hot reload on prompt changes  
- [ ] Type-safe prompt compilation with separate system/prompt methods
- [ ] No optional chaining or type assertions required in agent code
- [ ] Clear error messages
- [ ] End-to-end validation with `test-production-flow.sh`

### Production Ready ✅
- [ ] Build process integration
- [ ] Deployment compatibility
- [ ] Error handling and validation
- [ ] Documentation and examples

## File Locations

### CLI Implementation
- `internal/bundler/prompts.go` - Core prompt processing
- `cmd/bundle.go` - Enhanced bundle command (if needed)

### SDK Changes
- `src/types.ts` - Add prompts() function to AgentContext
- `src/apis/prompt.ts` - Implement prompts() function with generated prompts
- `src/server/server.ts` - Context initialization

### Sample Project
- `src/prompts.yaml` - Example prompt definitions
- `src/agents/my-agent/index.ts` - Updated agent code using ctx.prompts() interface

## Notes

- **Bundle-First Approach**: Integrate directly into existing bundle workflow rather than standalone commands
- **Backward Compatibility**: Ensure existing agents continue to work
- **Type Safety**: Maintain full TypeScript support throughout
- **Performance**: Minimal impact on build and runtime performance
- **Developer Experience**: Seamless integration with existing development workflow
