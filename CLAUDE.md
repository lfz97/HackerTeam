## Environment
- Repo lives on WSL2-mounted NTFS (OneDrive path at `/mnt/d/`) — run `git config --global --add safe.directory "/mnt/d/OneDrive - 上海达美乐比萨有限公司/documents/docs/code/go-code/MyProjects/git space/HackerTeam"` before any git commands
- `git` commands via Bash tool fail with "dubious ownership" without the safe.directory fix above
- `.go` files use CRLF line endings (Windows), `.md` files use LF — `Edit` tool fails on CRLF files, use `python3 -c "..."` via Bash instead
- `sed -i` fails with "Operation not permitted" on NTFS — write to `/tmp` and `cp` back, or use Python for in-place edits
- CRLF-safe edit for `.go` files: `python3 -c "import pathlib; p=pathlib.Path('file.go'); c=p.read_text(); c=c.replace('OLD','NEW'); p.write_text(c)"`

## Build & Run
- `go build -ldflags "-s -w" -o HackerTeam .` — build for current platform
- `.\build.ps1` — cross-compile all platforms (PowerShell)
- `make` or `make linux-x64` etc. — cross-compile (Makefile)
- `go run .` — run directly (auto-loads config from `<cwd>/.HackerTeam/`)
- `go vet ./...` — static analysis (passes clean)
- Go module: `HackerTeam` (Go 1.26.1)

## Architecture
- Multi-agent AI pentesting platform: Captain serially dispatches Recon → Scanner → Exploit (cross-validate) → PostExploit, plus Reproducer in two batches (Batch1: after Scanner+Exploit, Batch2: after PostExploit)
- Each agent prompt must have a "职责边界" rule as the first constraint — explicitly list what this agent MUST NOT do and WHICH agent handles that; LLMs cross role boundaries unless explicitly forbidden (Recon may try sqlmap, Scanner may try to exploit). Forbidding tool NAMES is not enough — LLMs bypass "don't use sqlmap" by doing manual injection with the same payloads. Forbid concrete BEHAVIORS with exact examples (e.g. "NEVER append ' / OR 1=1 / UNION SELECT to URL params") so the LLM cannot self-rationalize. Do NOT add cross-boundary refusal logic on sub-agents — if Recon rejects and Scanner also rejects, tasks deadlock; enforce boundaries on Captain's dispatch side only, accept the risk of Captain hallucination.
- Shared consensus system in `global/prompts/common/` (embedded via `//go:embed` in `global/agentCore.go`): `vuln_consensus.md` (vulnerability definition + severity rating by technical impact, no CVSS), `output_consensus.md` (output format, raw tool output preservation, vulnerability structured block format for Reproducer consumption)
- TUI built with `rivo/tview` + `gdamore/tcell/v2`, PTY execution via `creack/pty`
- Agent framework: `trpc.group/trpc-go/trpc-agent-go`, MCP: `trpc.group/trpc-go/trpc-mcp-go`
- LLM backends: OpenAI-compatible API or Anthropic native SDK
- Config auto-generated at first run: `<binary-dir>/.HackerTeam/HackerTeam.yaml`
- TUI colors centralized in `utils/pretty/pretty.go` (TuiXxx constants)
- `/new`, `/flush`, `/exit`, `ESC` — built-in TUI commands
- Agent prompts embedded via `//go:embed` in `global/agentCore.go` (`PromptFiles`, `prompts/*` prefix in ReadFile) and `session/summarizer.go` (`promptFiles`, `prompt/*` prefix)
- Adding a new shared consensus prompt pattern: 1) create `global/prompts/common/<name>.md`, 2) add variable in `global/agentCore.go` + load in `Initializer.go` (follow `VulnConsensusPrompt` pattern), 3) add `{{<NAME>}}` replacement in `assemblePrompt()` in `members.go`, 4) add `{{<NAME>}}` placeholder to each agent prompt `.md` file
- `{{OUTPUTDIR}}` is the exception — NOT replaced by `assemblePrompt()`. It's resolved once in `env.md` via `configENVPrompt()` then injected into all agents through `{{ENV}}`. Agents infer the path from the "Output Directory" field shown in the environment block. Use `{{OUTPUTDIR}}` directly in prompt `.md` files, do NOT add Go-level replacement for it.
- Adding a new agent: 1) create `global/prompts/agents/<name>.md` (include `{{ENV}}`, `{{COMMAND_EXECUTION}}`, `{{VULN_CONSENSUS}}`, `{{OUTPUT_CONSENSUS}}` as needed), 2) add `init<Name>()` in `members.go` (follow existing agent pattern), 3) add skill folder path var in `global/agentCore.go` + const in `Initializer.go`, 4) add folder to `checkSkillsFolder()` slice, 5) register agent in `initTeam()` team.New member list, 6) add agent definition + dispatch rules in Captain prompt (`captain.md`)

## Directory Map
- `global/` — Shared state: `Agentrunner` struct, config pointer, session service, embedFS (`PromptFiles`/`ToolSkills`), prompt strings, TUI widget references
- `bootstrap/` — Initializer (config, logging, session), member assembly (6 agent factories), main dialog loop
- `session/` — Agent runtime: summarizer, session service, prompt embedding (`prompt/*`)
- `handler/` — TUI dialog loop (`runIteratively.go`), single-turn execution (`runOnce.go`), message rendering (`message.go`), types (`model.go`)
- `config/` — Config struct and YAML loading (`HackerTeam.yaml`)
- `models/` — LLM provider constructors (OpenAI, Anthropic SDK wrappers)
- `tui/` — tview-based terminal UI (ConfigPage + AgentPage), PTY management
- `toolsets/localexec/` — LocalExec toolset (command execution subsystem for all agents)
- `functionTools/` — Custom Go function tools for agents
- `utils/pretty/` — Centralized TUI color constants (`TuiXxx`)

## Dependencies
- `trpc-agent-go` — main agent framework, currently from upstream `trpc.group/trpc-go/trpc-agent-go` (no fork/replace)

## Skill System
- External security tools (nmap, nuclei, sqlmap, etc.) are integrated as knowledge-only skills via `trpc-agent-go`'s built-in skill system — NOT as function tools
- Skills use `llmagent.WithSkillToolProfile(llmagent.SkillToolProfileKnowledgeOnly)` — injected into system prompt, execution still via LocalExec
- Each agent gets its own skill subdirectory: `.HackerTeam/<Role>Skills/` (ReconSkills, ScannerSkills, ExploitSkills, PostExploitSkills, ReproducerSkills — Reproducer's folder is intentionally left empty, no pentest-tool skills)
- Embedded skill template: `global/skillsTemplates/pentest-tools/SKILL.md.template` (via `//go:embed` in `global/agentCore.go`)
- `/flush` must re-create skill repos and re-attach to agents

## Terminology
- "Planner" in trpc-agent-go = `planner.Planner` interface (agent-level request/response hooks: `BuildPlanningInstruction` + `ProcessPlanningResponse`). Mounted via `llmagent.WithPlanner()`, NOT a tool. Builtin planner just sets `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` on model request — equivalent to manual config, no prompt injection. React planner injects `/*PLANNING*/`/`/*ACTION*/`/`/*FINAL_ANSWER*/` tags + prevents premature `Done=true` via response post-processing.
- "Planner" as a team member = a separate LLMAgent that Captain dispatches to for structured attack plans (like Recon/Scanner). Architecturally different from `WithPlanner()`. Currently NOT used — Captain's own prompt handles planning adequately; adding a separate PlannerAgent adds a round-trip without benefit.

## Agent Framework Gotchas
- Captain dispatches agents **serially** (Recon → Scanner → Exploit → PostExploit → Reproducer in two batches), not in parallel — `WithEnableParallelTools` is disabled; parallel dispatch causes framework-level issues when skill + localexec toolsets coexist
- `HistoryScope` is **NOT** set to Isolated — the framework default is `HistoryScopeParentBranch`, meaning sub-agents inherit Captain's conversation branch history. Code does not override this default.
- `LocalExec.submit_command` executes immediately (submit+start merged into one async call) — agents MUST poll `get_status` before `get_output`; `start_command` tool no longer exists
- `localexec.Manager` is per-agent, not a global singleton — `LocalExec()` creates a new Manager for each `LocalExecToolSet` instance; global `cache.go` removed
- `team.WithMemberToolStreamInner(true)` + `team.WithMemberToolInnerTextMode(team.InnerTextModeInclude)` — TUI shows sub-agent full transcript (text+tool calls+results); use `InnerTextModeExclude` to show only progress signals, hiding assistant text

## Context Management

HackerTeam uses three complementary mechanisms to prevent context overflow:

### 1. Session Summarization (`session/summarizer.go` + `bootstrap/members.go`)
- `WithAddSessionSummary(true)` on ALL 6 agents (Captain + 5 sub-agents) enables async summary injection
- Summarizer triggers at `CheckTokenThreshold(0.4 * contextwindow)` OR `CheckTimeThreshold(10min)` via `WithChecksAny`
- Token counting uses `model/tiktoken` (BPE), configured via `summary.SetTokenCounter(counter)`
- Summary model is the same as main model; for DeepSeek reasoning models, the token counter falls back to `cl100k_base` (within ~4-7% of DeepSeek's actual count per empirical testing)
- **Team-specific risk**: sub-agent tool results (nmap, sqlmap, gobuster output) can be 50K+ tokens each. Team serial dispatch (5 sub-agents) can produce 1M+ tokens in a single `runner.Run()`. If the summarizer's first attempt fails (e.g. summary model itself exceeds context), session grows unbounded — check `HackerTeam.log` for "summary worker failed"
- Post-summary hook strips `<think>...</think>` tags from summary text

### 2. Context Compaction (`bootstrap/members.go`)
- `WithEnableContextCompaction(true)` on ALL 6 agents enables deterministic tool result compression before each LLM call
- **Pass 1**: Historical tool results > 1024 tokens → replaced with placeholder (`event_id`/`tool_call_id` preserved for `session_load` recovery)
  - Protects current invocation + `KeepRecentRequests` (default 1) most recent completed invocations
- **Pass 2**: Any tool result > 8192 tokens → head+tail truncation with `[...N chars truncated...]` marker
  - Applies to ALL invocations including current; gated on `OversizedToolResultMaxTokens > 0`
  - Critical for sub-agents: single nmap/sqlmap output truncated from 50K to ~16K before entering summarizer input
- Triggers at 70% context window (`ContextCompactionThresholdRatio`, default 0.7)
- If still over threshold after compaction → sync `CreateSessionSummary` runs as fallback → request rebuilt
- `ForceCleanToolNames`/`KeepToolNames` available for per-tool policy (not currently configured)

### 3. On-Demand Session (`bootstrap/members.go`)
- `WithEnableOnDemandSession(true)` on ALL 6 agents gives `session_load`/`session_search` tools
- Compacted/truncated tool results can be retrieved by `event_id` with `content_offset`/`content_limit`
- Enables sliced loading of large outputs (e.g. read nmap port list without loading full scan)

### Troubleshooting Context Overflow
- **Symptom**: API error "requested X tokens exceeds maximum 1048565" (X > 1M)
- **Check**: `HackerTeam.log` for "summary worker failed" — if present, summaries are failing
- **Verify**: `contextwindow` in config MUST be ≤ actual model limit
- **Note**: tiktoken `cl100k_base` vs DeepSeek API token count differs ~4-7% (empirically verified) — not accurate enough to explain 2x+ discrepancies
- **Root cause pattern**: first summary attempt fails → delta grows unbounded → cascade failure → permanent retry loop
- **Fix priority**: (1) enable Context Compaction for tool result size control, (2) lower `CheckTokenThresholdPercent` if needed, (3) use non-reasoning model for summarization as last resort
