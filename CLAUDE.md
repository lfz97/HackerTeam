## Environment
- Repo lives on WSL2-mounted NTFS (OneDrive path at `/mnt/d/`) — run `git config --global --add safe.directory "/mnt/d/OneDrive - 上海达美乐比萨有限公司/documents/docs/code/go-code/MyProjects/git space/HackerTeam"` before any git commands
- `git` commands via Bash tool fail with "dubious ownership" without the safe.directory fix above
- `.go` files use CRLF line endings (Windows), `.md` files use LF — `Edit` tool fails on CRLF files, use `python3 -c "..."` via Bash instead
- `sed -i` fails with "Operation not permitted" on NTFS — write to `/tmp` and `cp` back, or use Python for in-place edits
- CRLF-safe edit for `.go` files: `python3 -c "import pathlib; p=pathlib.Path('file.go'); c=p.read_text(); c=c.replace('OLD','NEW'); p.write_text(c)"`

## Build & Run
- `go build -ldflags "-s -w" -o HackerTeam ./cmd` — build for current platform（根目录无 main.go，入口在 cmd/；`go build ./cmd` 不带 -o 会因输出名与目录冲突失败）
- `go run ./cmd` — run directly (auto-loads config from `<cwd>/.HackerTeam/`)
- `go vet ./...` — static analysis (passes clean)
- `go mod tidy` — sync dependencies after adding/removing imports
- **CGO required** — `service/engine/memory/sqlite` depends on `mattn/go-sqlite3`. Cross-compilation no longer supported; build natively on each platform.
- Go module: `HackerTeam` (Go 1.26.4)

## Architecture
- Multi-agent AI pentesting platform: Captain serially dispatches Recon → Scanner → Exploit (cross-validate) → PostExploit, plus Reproducer in two batches (Batch1: after Scanner+Exploit, Batch2: after PostExploit)
- Each agent prompt must have a "职责边界" rule as the first constraint — explicitly list what this agent MUST NOT do and WHICH agent handles that; LLMs cross role boundaries unless explicitly forbidden (Recon may try sqlmap, Scanner may try to exploit). Forbidding tool NAMES is not enough — LLMs bypass "don't use sqlmap" by doing manual injection with the same payloads. Forbid concrete BEHAVIORS with exact examples (e.g. "NEVER append ' / OR 1=1 / UNION SELECT to URL params") so the LLM cannot self-rationalize. Do NOT add cross-boundary refusal logic on sub-agents — if Recon rejects and Scanner also rejects, tasks deadlock; enforce boundaries on Captain's dispatch side only, accept the risk of Captain hallucination.
- Shared consensus system in `service/engine/prompts/common/` (embedded via `//go:embed prompts/*` in `service/engine/engineCore.go`): `vuln_consensus.md` (vulnerability definition + severity rating by technical impact, no CVSS), `output_consensus.md` (output format, raw tool output preservation, vulnerability structured block format for Reproducer consumption)
- TUI built with `rivo/tview` + `gdamore/tcell/v2`, PTY execution via `creack/pty`
- **Startup (refactored to HyperBot structure)**: `cmd/main.go` → `boot.Boot()` → `GetTuiService()` + `go { GetEngineService → AgentStart }` + `tui.Run()`. Engine init MUST stay in the goroutine — `Show*InMsgViewAndExit` (in `service/tui/tui.go`) block on an never-closed channel and need the tview event loop running to exit; synchronous init deadlocks on first-run/config-error paths.
- **UI**: AgentPage layout is StatusBar + AgentMessage(no border, full flex) + InputRow(InputArea + `Ctrl+K 帮助` hint). Help page (`tview.Table`, two-column: command + description) shown via `app.SetRoot()` on Ctrl+K, dismissed with Esc/Ctrl+K, focus returns to InputArea.
- Agent framework: `trpc.group/trpc-go/trpc-agent-go`, MCP: `trpc.group/trpc-go/trpc-mcp-go`
- LLM backends: OpenAI-compatible API or Anthropic native SDK
- Config auto-generated at first run: `<binary-dir>/.HackerTeam/HackerTeam.yaml`
- TUI colors centralized in `utils/pretty/pretty.go` (TuiXxx constants)
- `/new`, `/exit`, `ESC` — built-in TUI commands（`/flush` 已移除，每次 run 自动重建配置/技能/工具）
- Agent prompts embedded via `//go:embed prompts/*` in `service/engine/engineCore.go` (`PromptFiles`) and `service/engine/session/summarizer.go` (`promptFiles`, `prompt/*` prefix)
- Adding a new shared consensus prompt pattern: 1) create `service/engine/prompts/common/<name>.md`, 2) add field in `Engine` struct (`service/engine/engineCore.go`) + load in `configENVPrompt()` in `init.go` (follow `VulnConsensusPrompt` pattern), 3) add `{{<NAME>}}` replacement in `assemblePrompt()` in `members.go`, 4) add `{{<NAME>}}` placeholder to each agent prompt `.md` file
- `{{OUTPUTDIR}}` is the exception — NOT replaced by `assemblePrompt()`. It's resolved once in `env.md` via `configENVPrompt()` then injected into all agents through `{{ENV}}`. Agents infer the path from the "Output Directory" field shown in the environment block. Use `{{OUTPUTDIR}}` directly in prompt `.md` files, do NOT add Go-level replacement for it.
- Adding a new agent: 1) create `service/engine/prompts/agents/<name>.md` (include `{{ENV}}`, `{{COMMAND_EXECUTION}}`, `{{VULN_CONSENSUS}}`, `{{OUTPUT_CONSENSUS}}` as needed), 2) add `init<Name>()` in `service/engine/members.go` (follow existing agent pattern), 3) add skill folder path field in `Engine` struct (`engineCore.go`) + const in `init.go`, 4) add folder to `checkSkillsFolder()` slice, 5) register agent in the `newRunner()` factory's `team.New` member list (`init.go`), 6) add agent definition + dispatch rules in Captain prompt (`captain.md`)

## Directory Map
- `cmd/main.go` — entry point → `boot.Boot()`
- `boot/boot.go` — startup orchestration: `GetTuiService()` → `go { GetEngineService → AgentStart }` → `tui.Run()`
- `service/engine/` — Engine core (refactored from `global/` + `bootstrap/` + `handler/` + `config/` + `models/` + `session/` + `memory/` + `functionTools/` + `toolsets/`)
  - `engineCore.go` — `Engine` struct (all state: config pointer, runner, session, memory, 5 skill folder paths, prompt strings), `PromptFiles`/`ToolSkills` embedFS (`//go:embed prompts/*` + `skillsTemplates/*`), `GetEngineService()`, `AgentStart()` main dialog loop, `randomStartID()`
  - `init.go` — `preCheckLoad()` init sequence (config/skills/log/session/memory), `configENVPrompt()` (env.md + 3 shared prompt placeholders), `checkSkillsFolder()` (5 role skill dirs + template copy), `newRunner()` (agent factory: 6 agent factories + `team.New`), `redirectFrameworkLog()`; `tuiService` interface definition
  - `members.go` — 6 agent factories (`initCaptain`/`initRecon`/`initexploit`/`initpostexploit`/`initScanner`/`initReproducer`), `setAgent()` (model selection by APIType), `assemblePrompt()`
  - `engineRun.go` — dialog loop + single-turn execution (`agentRunIteratively`/`agentRunOnce`), turn types, merged from old `handler/runIteratively.go` + `runOnce.go` + `model.go` + `bootstrap/Bootstrap.go`
  - `messageRender.go` — message rendering (`renderStreamEvent`/`renderNonStreamEvent`/`renderToolCall`/`renderToolResult`), tool call/result buffer (`toolMsgBuffer`), merged from old `handler/message.go` + `toolMsg.go`
  - `config/` — `config.go` (Config struct + `LoadConfig`), `template.go` (YAML template via `//go:embed`)
  - `memory/sqlite.go` — SQLite memory service factory with auto-extraction
  - `session/` — summarizer, session service, prompt embedding (`prompt/*`)
  - `models/` — LLM provider constructors (OpenAI, Anthropic SDK wrappers)
  - `prompts/agents/` + `prompts/common/` — role prompts + shared consensus prompts (embedded)
  - `skillsTemplates/` — embedded pentest-tools skill template
  - `tools/functions/` — Custom Go function tools for agents
  - `tools/toolsets/localexec/` — LocalExec toolset (command execution subsystem for all agents)
- `service/tui/` — `tui.go` (TUI widgets, `GetTuiService`, `Run`, `Show*InMsgViewAndExit`), `Internal.go` (help table toggle, `PrintToMsgView` wrappers)
- `utils/pretty/` — Centralized TUI color constants (`TuiXxx`)

## Dependencies
- `trpc-agent-go` v1.11.0 — main agent framework, currently from upstream `trpc.group/trpc-go/trpc-agent-go` (no fork/replace)
- `trpc-mcp-go` v0.0.17 — MCP SDK（仅用于日志重定向，无 MCP 工具集）
- `tcell/v2` v2.13.10、`anthropic-sdk-go` v1.61.0
- `glamour` v2.0.1 (`charm.land/glamour/v2`) — Markdown → ANSI renderer (Dracula theme, non-stream mode uses `glamour.Render` + `tview.TranslateANSI` for formatted markdown display)

## Skill System
- External security tools (nmap, nuclei, sqlmap, etc.) are integrated as knowledge-only skills via `trpc-agent-go`'s built-in skill system — NOT as function tools
- Skills use `llmagent.WithSkillToolProfile(llmagent.SkillToolProfileKnowledgeOnly)` — injected into system prompt, execution still via LocalExec
- Each agent gets its own skill subdirectory: `.HackerTeam/<Role>Skills/` (ReconSkills, ScannerSkills, ExploitSkills, PostExploitSkills, ReproducerSkills — Reproducer's folder is intentionally left empty, no pentest-tool skills)
- Embedded skill template: `service/engine/skillsTemplates/pentest-tools/SKILL.md.template` (via `//go:embed skillsTemplates/*` in `service/engine/engineCore.go`)
- Skill repos re-created automatically on every run — each agent's `init*()` function calls `skill.NewFSRepository(...)` locally, and the `newRunner()` factory re-runs all 6 factories each run (no `/flush` needed). Unlike HyperBot's global SkillRepo singleton, this per-agent pattern has no cache staleness risk.

## Terminology
- "Planner" in trpc-agent-go = `planner.Planner` interface (agent-level request/response hooks: `BuildPlanningInstruction` + `ProcessPlanningResponse`). Mounted via `llmagent.WithPlanner()`, NOT a tool. Builtin planner just sets `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` on model request — equivalent to manual config, no prompt injection. React planner injects `/*PLANNING*/`/`/*ACTION*/`/`/*FINAL_ANSWER*/` tags + prevents premature `Done=true` via response post-processing.
- "Planner" as a team member = a separate LLMAgent that Captain dispatches to for structured attack plans (like Recon/Scanner). Architecturally different from `WithPlanner()`. Currently NOT used — Captain's own prompt handles planning adequately; adding a separate PlannerAgent adds a round-trip without benefit.

## Agent Framework Gotchas
- Captain dispatches agents **serially** (Recon → Scanner → Exploit → PostExploit → Reproducer in two batches), not in parallel — `WithEnableParallelTools` is disabled; parallel dispatch causes framework-level issues when skill + localexec toolsets coexist
- `HistoryScope` is **NOT** set to Isolated — the framework default is `HistoryScopeParentBranch`, meaning sub-agents inherit Captain's conversation branch history. Code does not override this default.
- `LocalExec.submit_command` executes immediately (submit+start merged into one async call) — agents MUST poll `get_status` before `get_output`; `start_command` tool no longer exists
- `localexec.Manager` is per-agent, not a global singleton — `LocalExec()` creates a new Manager for each `LocalExecToolSet` instance; global `cache.go` removed
- `team.WithMemberToolStreamInner(true)` + `team.WithMemberToolInnerTextMode(team.InnerTextModeInclude)` — TUI shows sub-agent full transcript (text+tool calls+results); use `InnerTextModeExclude` to show only progress signals, hiding assistant text
- **`models.Openai()` / `models.Anthropic()` are canonical model constructors** — `service/engine/session/summarizer.go` and `setAgent()` use these two functions. They handle DeepSeek variant detection, reasoning backfill, and API auth. When creating a new model instance from config, call these instead of manually assembling options.
- **ANSI → tview tag conversion required** — tview's `SetDynamicColors(true)` only supports its own color tag format (`[red]text[-]`). Standard ANSI escape sequences must go through `tview.TranslateANSI()` before writing to a TextView. Without this, ANSI codes appear as visible garbage.
- **Tool response content must be skipped in content rendering** — Both stream and non-stream content paths check `Role != "tool"` to prevent tool JSON from leaking through the main content renderer.
- **Multi-tool results handled in `engineRun.go`** — Framework merges parallel tool results into a single `tool.response` event with N Choices. `agentRunOnce` detects `ObjectTypeToolResponse` and iterates ALL Choices.
- **Glamour markdown rendering** — Non-stream body text is rendered via `glamour` (dracula theme). `document.margin = 0` removes dracula theme's left margin; `strings.TrimRight` strips trailing whitespace to prevent alignment artifacts before tool calls. **Must append `[-:-:-]` after `TranslateANSI(out)`** — glamour's ANSI output may not end with a full reset sequence, leaving unclosed tview tags that leak into the next line (tool calls appear brighter/miscolored).
- **`show_reasoning` config** — `config.Model.ShowReasoning` (`yaml:"show_reasoning"`) controls reasoning/thinking display. Default `false`. Affects both stream and non-stream paths.
- **`messageRender.go` refactored** — `printMessage` split into `renderStreamEvent`, `renderNonStreamEvent`, `renderToolCall`, `renderToolResult` (`service/engine/messageRender.go`). Tool call/result rendering uses shared `addToolCallMsg`/`addToolResultMsg` helpers with `toolMsgBuffer` mutex. Compact single-line format via `pretty.TToolCompact` — green `●` + orange tool name + dim gray `args → result_summary`. No trailing `\n` (double-newline with next tool's leading `\n` causes alignment shift).
- **embedFS case sensitivity** — `//go:embed` + `ReadFile` paths are case-sensitive on Linux. Always match exact file name case between `go:embed` glob patterns and `ReadFile` calls.

## Auto-Extraction Memory (SQLite)

Persistent long-term memory using SQLite, with background LLM-based extraction after each turn. Captain is the sole memory manager — sub-agents do NOT get memory tools.

### Architecture
- `service/engine/memory/sqlite.go` — factory: creates `memorysqlite.Service` with `extractor.NewExtractor(model)` + `WithExtractor(ext)`. Exposes `memory_search`, `memory_load`, `memory_add`, `memory_update` via `WithAutoMemoryExposedTools`. `memory_delete` and `memory_clear` are not exposed to agents.
- `service/engine/engineCore.go` — `SqliteMemoryService *memorysqlite.Service` field on `Engine` struct
- `service/engine/init.go` — `initSqliteMemoryService()` creates extractor model from config (via `models.Openai()` / `models.Anthropic()`), passes to `NewSQLiteMemoryService(m, dbPath)`. Called after `LoadConfig()`, before `newRunner()`.
- `service/engine/members.go` — `initCaptain()` appends `SqliteMemoryService.Tools()` (exposes `memory_search`/`memory_load`/`memory_add`/`memory_update` to Captain only) and sets `WithPreloadMemory(10)`. Sub-agents do NOT get memory tools — only Captain manages memory.
- `service/engine/prompts/agents/captain.md` — `# Memory` section defines Captain's memory behavior: search-before-store, proactive storage, outdated correction, atomic/specific writing standards

### Team considerations
- Captain is the sole memory manager — all memory creation, update, and deletion happens through Captain's explicit tool calls
- Sub-agents benefit indirectly — Captain can reference past operations and store useful patterns discovered during pentest
- Auto-extraction runs after each turn via the framework's `EnqueueAutoMemoryJob`, but Captain can also manually manage memories

### Gotchas
- `initSqliteMemoryService()` MUST be called before `newRunner()` — the agent factory reads `SqliteMemoryService.Tools()`, nil service → panic
- `stdlog.SetOutput(file)` in `redirectFrameworkLog()` redirects gse dictionary-loading chatter away from TUI
- Default memory limit: 100000 (`service/engine/memory/sqlite.go:WithMemoryLimit`)
- Extractor model is the same as main model (same API endpoint/credentials)

## Context Management

HackerTeam uses three complementary mechanisms to prevent context overflow:

### 1. Session Summarization (`service/engine/session/summarizer.go` + `service/engine/members.go`)
- `WithAddSessionSummary(true)` on ALL 6 agents (Captain + 5 sub-agents) enables async summary injection
- Summarizer triggers at `CheckTokenThreshold(0.6 * contextwindow)` OR `CheckTimeThreshold(10min)` via `WithChecksAny`
- `WithSkipRecent` preserves the last complete interaction cycle (from last user message to tail) from being summarized — keeps current turn intact in prompt
- `WithToolResultFormatter` truncates tool results to 1000 runes (head 500 + tail 500) before entering summary model input — especially valuable for sub-agents whose tool outputs (nmap, sqlmap) are 50K+ tokens. Only affects summary input; original events remain intact
- `WithSyncSummaryIntraRun(true)` on ALL 6 agents — enables synchronous summary refresh between LLM loop iterations. Critical for sub-agents running long command chains (nmap scans, exploit attempts) where async summary may arrive too late
- `WithSessionSummaryInjectionMode(SessionSummaryInjectionUser)` on ALL 6 agents — injects summary into user message instead of system message. Each agent has a long SOP-focused system prompt (职责边界, command execution rules, output format); keeping system area clean prevents summary from competing with SOP priority
- Token counting uses `model/tiktoken` (BPE), configured via `summary.SetTokenCounter(counter)`
- Summary model is the same as main model; for DeepSeek reasoning models, the token counter falls back to `cl100k_base` (within ~4-7% of DeepSeek's actual count per empirical testing)
- **Team-specific risk**: sub-agent tool results (nmap, sqlmap, gobuster output) can be 50K+ tokens each. Team serial dispatch (5 sub-agents) can produce 1M+ tokens in a single `runner.Run()`. If the summarizer's first attempt fails, session grows unbounded — check `HackerTeam.log` for "summary worker failed". With ToolResultFormatter truncating to 1000 runes, this risk is significantly reduced but not eliminated for the main conversation (Compaction handles that)
- Post-summary hook strips `<think>...</think>` tags from summary text

### 2. Context Compaction (`service/engine/members.go`)
- `WithEnableContextCompaction(true)` on ALL 6 agents enables deterministic tool result compression before each LLM call
- **Pass 1**: Historical tool results > 1024 tokens → replaced with placeholder (`event_id`/`tool_call_id` preserved for `session_load` recovery)
  - Protects current invocation + `KeepRecentRequests` (default 1) most recent completed invocations
- **Pass 2**: Any tool result > 8192 tokens → head+tail truncation with `[...N chars truncated...]` marker
  - Applies to ALL invocations including current; gated on `OversizedToolResultMaxTokens > 0`
  - Critical for sub-agents: single nmap/sqlmap output truncated from 50K to ~16K before entering summarizer input
- Triggers at 70% context window (`ContextCompactionThresholdRatio`, default 0.7)
- If still over threshold after compaction → sync `CreateSessionSummary` runs as fallback → request rebuilt
- `ForceCleanToolNames`/`KeepToolNames` available for per-tool policy (not currently configured)

### 3. On-Demand Session (`service/engine/members.go`)
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
