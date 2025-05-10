# Agent Creation Enhancement – Post-Creation "What should this Agent do?" Flow

## Background
The current `agent create` command already collects an **Agent name**, **optional description**, and **auth type**, then:
1. Creates the remote Agent record via the Cloud API.
2. Generates local source files from the selected runtime template (`rules.NewAgent`).
3. Saves the updated `agentuity.json` project file.

You want an **additional wizard step _after_ the template scaffolding** that lets the user describe what the Agent should actually do (its task/logic). That free-form description will be used to _modify the freshly generated source file(s)_ – e.g. by invoking an LLM-powered code-agent. You have an example implementation that should live in a new internal package `internal/codeagent`.

---

## High-Level Flow
```
agent create …
└─(existing) create remote agent & scaffold template
   └─(NEW) prompt user: "What should this agent do?"
      └─ pass description → codeagent.Generate(…)
         └─ codeagent rewrites agent source file(s)
            └─ success banner / tip
```

## Tasks

### 0. Prerequisites / Inputs
- [ ] Receive the example **code-agent** implementation from you; place under `internal/codeagent`.
- [ ] Verify it exposes an API we can call synchronously (e.g. `Generate(ctx, logger, agentDir, description string) error`). If not, wrap/adapt.

### 1. UI / Wizard Changes (`cmd/agent.go`)
1. Locate the end of the `action` func inside `agentCreateCmd` **after** `rules.NewAgent(…)` succeeds.
2. Insert a **TUI prompt** using existing helpers (`tui.Input`, or a new `tui.InputMultiline` if available) – something like:
   > "Describe what you'd like the **%s** Agent to do (press Enter twice to finish):"
3. Skip the prompt when:
   - `--non-interactive` / `!tui.HasTTY`
   - or a new CLI flag `--goal`/`--task` was supplied with the description (allows CI usage).
4. Capture the final text into `agentGoal`.

### 2. Invoke `codeagent`
1. Determine the agent's source directory:
   ```go
   agentDir := filepath.Join(theproject.Dir, theproject.Project.Bundler.AgentConfig.Dir, util.SafeFilename(name))
   ```
2. Show spinner: `tui.ShowSpinner("Crafting Agent code …", func() { … })`
3. Inside spinner action call:
   ```go
   err := codeagent.Generate(ctx, logger, agentDir, agentGoal)
   ```
4. On error → use `errsystem` with a dedicated error code `ErrAgentCodegen`.

### 3. `internal/codeagent` Package
1. **Structure**
   ```go
   package codeagent

   type Options struct {
       Logger logger.Logger
       Dir    string // agent source dir
       Goal   string // user prompt
   }

   func Generate(ctx context.Context, opts Options) error
   ```
2. Implementation will largely come from your example. At minimum it should:
   - Parse the main agent file path (rule-specific, default `index.ts` or `handler.py`, etc.).
   - Call the LLM / template logic to rewrite code.
   - Run `gofmt`, `prettier`, or language-appropriate formatter.
   - Leave backup copy in `.agentuity/backup` just like delete flow does.
3. Add **unit tests** with a fake model so CI doesn't hit the network.

### 4. CLI Flags & Docs
- [ ] Add `--goal <text>` flag to `agent create` (optional) to bypass prompt.
- [ ] Update `--help` strings and README snippet.

### 5. Error Handling & Edge Cases
- [ ] No goal provided (interactive) → skip codeagent step, show info banner.
- [ ] codeagent fails → keep original scaffold, show warning, continue.
- [ ] Network/LLM timeouts → use context with reasonable deadline.

### 6. Testing Matrix
- ✔ Create agent interactively with goal.
- ✔ Create agent with `--goal` in non-TTY.
- ✔ Failure path – model error.
- ✔ Templates for both `bun` & `python` runtimes.

### 7. Documentation
- [ ] Add `docs/agent_goal_flow.md` explaining the feature & examples.
- [ ] Release notes entry.

---

## Open Questions / Confirmations
1. Desired CLI flag name: `--goal`, `--task`, or something else?
--goal is fine

2. Multiline input UX – is single line enough or should we implement a small editor (e.g. `$EDITOR` fallback)?
We will need multi line - there should eb one in the ui package

3. Any specific formatting/linters the generated code must pass (e.g. `eslint`, `ruff`)?
None right now.

4. Location & API of the example code-agent – send when ready.
added it in the docs folder

Please review & let me know what to adjust before we start coding. 