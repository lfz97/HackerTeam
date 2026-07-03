# Align HackerTeam File Structure with HyperBot

## Motivation

HackerTeam was forked from HyperBot with the goal of making minimal multi-agent modifications. Over time, file names and variable names drifted from HyperBot's conventions. This spec aligns the non-architectural naming back to HyperBot conventions while preserving all multi-agent functionality.

## Scope

**In scope:** 4 file renames, 2 variable renames, 1 function rename — all naming-only, zero logic changes.

**Out of scope:** Adding/removing features, changing multi-agent architecture, altering `models/` directory name, MCP toolset files, `prompts/` embed path.

## Changes

### 1. File Renames (git mv only, no content edits)

| From | To |
|---|---|
| `global/agentCore.go` | `global/AgentEngine.go` |
| `global/tui.go` | `global/TUI.go` |
| `config/yaml_template.go` | `config/configTemplate.go` |
| `handler/toolMsgBuffer.go` | `handler/toolMsg.go` |

### 2. Global Variable Renames

| From | To | Affected Files |
|---|---|---|
| `SessionService` | `SessionService_p` | `global/AgentEngine.go` (declaration), `bootstrap/Initializer.go` (4 uses), `session/memSessionService.go` (comment only) |
| `FrameworkLogFile` | `FrameworkLogFile_p` | `global/AgentEngine.go` (declaration), `bootstrap/Initializer.go` (4 uses) |

### 3. Function Rename

| From | To | Affected Files |
|---|---|---|
| `initMemorySessionService()` | `initInMemorySessionService()` | `bootstrap/Initializer.go` (definition + 1 call site) |

### 4. Intentional Retentions (not changing)

- `models/` — functionally different from HyperBot's `agent/` (model constructors vs agent wrappers)
- `prompts/` — plural form is reasonable, large blast radius
- `configENVPrompt()` — semantically accurate for what it does
- `initTeam()` — reflects multi-agent architecture
- All project-name constants (`HackerTeamConfigPath`, `.HackerTeam`, etc.)

## Implementation Order

1. `git mv` the 4 files
2. Global variable renames (find-replace in affected files)
3. Function rename (find-replace in `bootstrap/Initializer.go`)
4. `go vet ./...` to verify
5. `go build` to verify
