# Console-Error Debugging Agent – Project Plan

## 1. Problem Statement
Developers run `agentuity dev` to iterate on agents locally. When the process encounters a runtime error (panic, stack-trace, unhandled promise rejection, etc.) the developer must manually read logs, locate offending code and work out a fix. We want a companion "Debug Agent" that wakes up automatically on such errors, inspects the failure context, reads relevant source files and surfaces concise diagnostics & remediation hints.

## 2. High-Level Goals
1. Detect meaningful errors emitted by the dev server in real-time.
2. Trigger an LLM-powered assistant (Debug Agent) that:
   - Summarises the error (what, where, why).
   - Reads affected source files to provide context.
   - Suggests possible root causes & concrete next steps.
3. Present the advice to the developer via:
   - CLI stdout (initial target).
   - Live-dev websocket → web UI (future enhancement).
4. **Read-only** interaction for the first iteration (no automatic file edits).

## 3. Architectural Overview
```text
┌──────────────┐      stdout/stderr       ┌───────────────┐
│ agentuity dev│ ───────────────────────▶ │ Error Monitor │
└──────────────┘                          └──────┬────────┘
                                                │  triggers
                                                ▼
                                      ┌────────────────────┐
                                      │   Debug Agent      │
                                      │ (LLM tool-caller)  │
                                      └────────┬───────────┘
                                               │ suggestions
                                               ▼
                                   CLI / Web UI / Log file
```

### Key Components
1. **Error Monitor** (`internal/dev/debugmon`):
   - Wraps/dev taps into the `agentuity dev` process pipes.
   - Regex/classifier to recognise actionable errors vs. regular output.
   - Debounces duplicate messages.
   - Sends `ErrorEvent` {message, stackTrace, timestamp} to Debug Agent.
2. **Debug Agent** (`internal/debugagent`):
   - Reuses `codeagent` machinery (conversation loop, tool schema) with a trimmed tool-set: `read_file`, `list_files` only.
   - System prompt specialised for debugging ("You are a code-diagnosis assistant…").
   - Iteration budget small (e.g., 3).
3. **Presentation Layer**
   - CLI: coloured box with summary + numbered suggestions.
   - Hook existing websocket to forward advice to the app (phase-2).

## 4. Detailed Task Breakdown
| # | Task | Owner | Status | Notes |
|---|------|-------|--------|-------|
| 1 | Create `internal/dev/debugmon` package that wraps `exec.Cmd` and streams output lines with callbacks. |  | In Progress | Initial scaffold committed (`Monitor`, `ErrorEvent`). |
| 2 | Implement error pattern detection (basic regex for `panic:`, `ERROR`, stack trace). |  | In Progress | Basic regex patterns implemented in `debugmon`. |
| 3 | Add prompt-size safeguards (truncate error, file contents, list size). |  | Done | Guard rails added in `debugagent`. |
| 4 | Define `ErrorEvent` struct and channel between monitor and debug agent. |  | Done | Struct defined. Channel usage placeholder. |
| 5 | Fork existing `codeagent` → `debugagent` (read-only tools). |  | In Progress | Core scaffold (`Analyze`, tools, prompt) committed. |
| 6 | Craft debugging system prompt template (can embed with `go:embed`). |  | Not Started |  |
| 7 | Wire monitor ↔ debug agent in `cmd/dev.go` behind flag `--debug-assist`. |  | In Progress | Dev command patched with monitor, flag, and output tee. |
| 8 | Pretty-print suggestions to terminal (use `glamour` for markdown). |  | Done | Glamour renderer integrated. |
| 9 | Unit tests: error detection & secure-join read protection. |  | Not Started |  |
| 10 | Documentation & README update. |  | Not Started |  |
| 11 | Handle non-convergence by returning last assistant text. |  | Done | Fallback implemented + default iterations 8. |

## 5. MVP Acceptance Criteria
- Running `agentuity dev --debug-assist` prints additional advice after an error appears.
- Advice includes: summary sentence + ≥1 actionable suggestion.
- No source files are modified automatically.

## 6. Nice-to-Haves / Future Iterations
1. Configurable error patterns.
2. Automatic link to open file/line in IDE.
3. Optional automatic patch proposal (via `edit_file`).
4. Web UI surfacing (reuse live-dev websocket).
5. Remember past errors & resolutions (cache).

## 7. Risks & Mitigations
- **False positives**: fine-tune regex, add heuristics, allow disable.
- **Noise/Over-verbosity**: cap token budget, summarise.
- **Latency**: run LLM call asynchronously; spinner & timeout.
- **Security**: ensure Debug Agent can only read inside project dir.

## 8. Timeline (indicative)
- Week 1: Error monitor + pattern detection.
- Week 2: Debug Agent scaffolding & integration.
- Week 3: CLI presentation, polish, docs.

## 9. Progress Log

- **{{TODAY}}** – Scaffolded `internal/dev/debugmon`, added `internal/debugagent`, and integrated `--debug-assist` flag & monitor wiring in `cmd/dev.go`. 
- **{{TODAY}}** – Improved Analyze loop: returns last assistant message even if tool loop exceeds iterations; default iterations now 8. 

---
Owner: TBD
Last updated: {{TODAY}} 