## Environment
- Repo lives on WSL2-mounted NTFS (`/mnt/d/`) — run `git config --global --add safe.directory "/mnt/d/code/my/HackerTeam"` before any git commands (use the actual checkout path if it moves again)
- `git` commands via Bash tool fail with "dubious ownership" without the safe.directory fix above
- `.go` files use CRLF line endings (Windows), `.md` files use LF — `Edit` tool fails on CRLF files, use `python3 -c "..."` via Bash instead
- `sed -i` fails with "Operation not permitted" on NTFS — write to `/tmp` and `cp` back, or use Python for in-place edits
- CRLF-safe edit for `.go` files: `python3 -c "import pathlib; p=pathlib.Path('file.go'); c=p.read_text(); c=c.replace('OLD','NEW'); p.write_text(c)"`

## Build & Run
- `go build -ldflags "-s -w" -o HackerTeam ./cmd` — build for current platform（根目录无 main.go，入口在 cmd/；`go build ./cmd` 不带 -o 会因输出名与目录冲突失败）
- `go run ./cmd` — run directly (auto-loads config from `<cwd>/.HackerTeam/`)
- `go vet ./...` — static analysis (passes clean)
- `make` / `./build.sh` / `build.ps1` — release builds into `release/HackerTeam` (same `-s -w` ldflags)
- **No test suite** — zero `*_test.go` files; `go vet ./...` + `go build ./cmd` are the verification gates. Do not look for or invent `go test` targets
- `go mod tidy` — sync dependencies after adding/removing imports
- **CGO required** — `service/engine/memory/sqlite` depends on `mattn/go-sqlite3`. Cross-compilation no longer supported; build natively on each platform.
- Go module: `HackerTeam` (Go 1.26.4)

## Architecture
- Multi-agent AI pentesting platform: Captain serially dispatches Recon → Scanner → Exploit (cross-validate) → PostExploit, plus Reproducer in two batches (Batch1: after Scanner+Exploit, Batch2: after PostExploit)
- Each agent prompt must have a "职责边界" rule as the first constraint — explicitly list what this agent MUST NOT do and WHICH agent handles that; LLMs cross role boundaries unless explicitly forbidden (Recon may try sqlmap, Scanner may try to exploit). Forbidding tool NAMES is not enough — LLMs bypass "don't use sqlmap" by doing manual injection with the same payloads. Forbid concrete BEHAVIORS with exact examples (e.g. "NEVER append ' / OR 1=1 / UNION SELECT to URL params") so the LLM cannot self-rationalize. Do NOT add cross-boundary refusal logic on sub-agents — if Recon rejects and Scanner also rejects, tasks deadlock; enforce boundaries on Captain's dispatch side only, accept the risk of Captain hallucination.
- Shared consensus system in `service/engine/prompts/common/` (embedded via `//go:embed prompts/*` in `service/engine/engineCore.go`): `vuln_consensus.md` (vulnerability definition + severity rating by technical impact, no CVSS), `output_consensus.md` (output format, raw tool output preservation, vulnerability structured block format for Reproducer consumption), `command_execution.md` (source of `{{COMMAND_EXECUTION}}` — OS-aware command selection rules), `env.md` (source of `{{ENV}}` — resolved once in `configENVPrompt()`)
- TUI built with `rivo/tview` + `gdamore/tcell/v2`, PTY execution via `creack/pty`
- **Startup (refactored to HyperBot structure)**: `cmd/main.go` → `boot.Boot()` → `GetTuiService()` + `go { GetEngineService → AgentStart }` + `tui.Run()`. Engine init MUST stay in the goroutine — `Show*InMsgViewAndExit` (in `service/tui/tui.go`) block on an never-closed channel and need the tview event loop running to exit; synchronous init deadlocks on first-run/config-error paths.
- **UI**: AgentPage layout is StatusBar + AgentMessage(no border, full flex) + InputRow(InputArea + `Ctrl+K 帮助` hint). Help page (`tview.Table`, two-column: command + description) shown via `app.SetRoot()` on Ctrl+K, dismissed with Esc/Ctrl+K, focus returns to InputArea.
- Agent framework: `trpc.group/trpc-go/trpc-agent-go`, MCP: `trpc.group/trpc-go/trpc-mcp-go`
- LLM backends: OpenAI-compatible API or Anthropic native SDK
- Config auto-generated at first run: `<binary-dir>/.HackerTeam/HackerTeam.yaml`
- MCP servers declared in config: `http_mcp`（sse/streamable_http）与 `stdin_mcp`（stdio）两个列表，`enabled` 开关控制；`name` 必须唯一（决定工具前缀 `{name}_{toolName}`，缺省时分配 `mcp_N` 默认名）；启用的 server 挂载给全部 agent（含 Captain），同一实例跨 agent 共享
- TUI colors centralized in `utils/pretty/pretty.go` (TuiXxx constants)
- `/new`, `/exit`, `ESC` — built-in TUI commands（`/flush` 已移除；每轮 run 重建配置/技能/MCP工具集，内置工具与工具集（localexec）常驻、跨轮复用）
- Agent prompts embedded via `//go:embed prompts/*` in `service/engine/engineCore.go` (`PromptFiles`) and `service/engine/session/summarizer.go` (`promptFiles`, `prompt/*` prefix)
- Adding a new shared consensus prompt pattern: 1) create `service/engine/prompts/common/<name>.md`, 2) add field in `Engine` struct (`service/engine/engineCore.go`) + load in `configENVPrompt()` in `init.go` (follow `VulnConsensusPrompt` pattern), 3) add `{{<NAME>}}` replacement in `assemblePrompt()` in `members.go`, 4) add `{{<NAME>}}` placeholder to each agent prompt `.md` file
- `{{OUTPUTDIR}}` is the exception — NOT replaced by `assemblePrompt()`. It's resolved once in `env.md` via `configENVPrompt()` then injected into all agents through `{{ENV}}`. Agents infer the path from the "Output Directory" field shown in the environment block. Use `{{OUTPUTDIR}}` directly in prompt `.md` files, do NOT add Go-level replacement for it.
- Adding a new agent: 1) create `service/engine/prompts/agents/<name>.md` (include `{{ENV}}`, `{{COMMAND_EXECUTION}}`, `{{VULN_CONSENSUS}}`, `{{OUTPUT_CONSENSUS}}` as needed), 2) add `init<Name>()` in `service/engine/members.go` **with `llmagent.WithDescription()`** (Captain dispatches members via tool calls and reads only the description to decide when/how to dispatch — include capability boundary + when-to-use + "pass task in `request` field"; follow existing agents), 3) add skill folder path field in `Engine` struct (`engineCore.go`) + const in `init.go`, 4) add folder to `checkSkillsFolder()` slice, 5) register agent in the `newRunner()` factory's `team.New` member list (`init.go`), 6) add agent definition + dispatch rules in Captain prompt (`captain.md`)

## Directory Map
- `cmd/main.go` — entry point → `boot.Boot()`
- `boot/boot.go` — startup orchestration: `GetTuiService()` → `go { GetEngineService → AgentStart }` → `tui.Run()`
- `service/engine/` — Engine core (refactored from `global/` + `bootstrap/` + `handler/` + `config/` + `models/` + `session/` + `memory/` + `functionTools/` + `toolsets/`)
  - `engineCore.go` — `Engine` struct (all state: config pointer, runner, session, memory, 5 skill folder paths, prompt strings), `PromptFiles`/`ToolSkills` embedFS (`//go:embed prompts/*` + `skillsTemplates/*`), `GetEngineService()`, `AgentStart()` main dialog loop, `randomStartID()`
  - `init.go` — `preCheckLoad()` init sequence (config/skills/log/session/memory/内置工具与工具集), `configENVPrompt()` (env.md + 3 shared prompt placeholders), `checkSkillsFolder()` (5 role skill dirs + per-role preset copy via `makeSkillsWritable()`), `newRunner()` (agent factory: 6 agent factories + `team.New`), `redirectFrameworkLog()`; `initBuiltinTools()`/`initBuiltinToolsets()` (内置工具/工具集启动时建一次，跨轮常驻), `loadMCPFromConfig()`/`refreshMCPFromConfig()` (MCP工具集每轮run Close+重建); `tuiService` interface definition
  - `members.go` — 6 agent factories (`initCaptain`/`initRecon`/`initexploit`/`initpostexploit`/`initScanner`/`initReproducer`), `setAgent()` (model selection by APIType), `assemblePrompt()`
  - `engineRun.go` — dialog loop + single-turn execution (`agentRunIteratively`/`agentRunOnce`), turn types, merged from old `handler/runIteratively.go` + `runOnce.go` + `model.go` + `bootstrap/Bootstrap.go`
  - `messageRender.go` — message rendering (`renderStreamEvent`/`renderNonStreamEvent`/`renderToolCall`/`renderToolResult`), tool call/result buffer (`toolMsgBuffer`), merged from old `handler/message.go` + `toolMsg.go`
  - `config/` — `config.go` (Config struct + `LoadConfig`), `config.yaml` (checked-in sample user config), `mcp.go` (HttpMCP/StdinMCP 配置结构体), `template.go` (YAML template via `//go:embed`，含 `http_mcp`/`stdin_mcp` 示例段)
  - `memory/sqlite.go` — SQLite memory service factory with auto-extraction
  - `session/` — summarizer, session service, prompt embedding (`prompt/*`)
  - `models/` — LLM provider constructors (OpenAI, Anthropic SDK wrappers)
  - `prompts/agents/` + `prompts/common/` — role prompts + shared consensus prompts (embedded)
  - `skillsTemplates/` — embedded per-role skill presets（`Recon/` `Scanner/` `Exploit/` `PostExploit/`，各含 pentest-tools 模板 + hacktricks 红队知识；`Reproducer/` 预置 poc-scripting 复现脚本模式）
  - `tools/functions/` — Custom Go function tools for agents
  - `tools/toolsets/localexec/` — LocalExec toolset (command execution subsystem for all agents；`Close()` 先 kill 未结束命令再清注册表)
  - `tools/toolsets/mcp.go` — MCP ToolSet wrappers (`HttpMCP()`/`StdinMCP()`：`WithName` 决定工具前缀、`WithSessionReconnect(3)`、10s timeout)
- `service/tui/` — `tui.go` (TUI widgets, `GetTuiService`, `Run`, `Show*InMsgViewAndExit`), `Internal.go` (help table toggle, `PrintToMsgView` wrappers)
- `utils/pretty/` — Centralized TUI color constants (`TuiXxx`)

## Dependencies
- `trpc-agent-go` v1.11.1-0.20260820131707-cdaece75b478 — main agent framework (no fork/replace; pseudo-version containing #2501 fix)
- `trpc-mcp-go` v0.0.18 — MCP SDK（仅用于日志重定向，无 MCP 工具集）
- `tcell/v2` v2.13.10、`anthropic-sdk-go` v1.66.0
- `glamour` v2.0.1 (`charm.land/glamour/v2`) — Markdown → ANSI renderer (Dracula theme, non-stream mode uses `glamour.Render` + `tview.TranslateANSI` for formatted markdown display)

## Skill System
- External security tools (nmap, nuclei, sqlmap, etc.) are integrated as knowledge-only skills via `trpc-agent-go`'s built-in skill system — NOT as function tools
- Skills use `llmagent.WithSkillToolProfile(llmagent.SkillToolProfileKnowledgeOnly)` — injected into system prompt, execution still via LocalExec
- Each agent gets its own skill subdirectory: `.HackerTeam/<Role>Skills/` (ReconSkills, ScannerSkills, ExploitSkills, PostExploitSkills, ReproducerSkills — Reproducer 预置 poc-scripting（复现脚本编写模式），无 pentest-tools/hacktricks 知识)
- Embedded skill presets: `service/engine/skillsTemplates/<Role>/` 按角色分发（`checkSkillsFolder()` 在角色目录不存在时创建目录并整目录复制对应预设，embedFS 复制后经 `makeSkillsWritable()` 修正只读权限；预设含 hacktricks-* 蒸馏知识 + pentest-tools 占位模板（`SKILL.md.template`，复制后由用户自行改名 SKILL.md 并填写工具清单）；Reproducer 预置 poc-scripting（SQLi/RCE/SSTI/SSRF/XSS/协议客户端等复现脚本模式））
- Skill repos re-created automatically on every run — each agent's `init*()` function calls `skill.NewFSRepository(...)` locally, and the `newRunner()` factory re-runs all 6 factories each run (no `/flush` needed). Unlike HyperBot's global SkillRepo singleton, this per-agent pattern has no cache staleness risk.

## Terminology
- "Planner" in trpc-agent-go = `planner.Planner` interface (agent-level request/response hooks: `BuildPlanningInstruction` + `ProcessPlanningResponse`). Mounted via `llmagent.WithPlanner()`, NOT a tool. Builtin planner just sets `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` on model request — equivalent to manual config, no prompt injection. React planner injects `/*PLANNING*/`/`/*ACTION*/`/`/*FINAL_ANSWER*/` tags + prevents premature `Done=true` via response post-processing.
- "Planner" as a team member = a separate LLMAgent that Captain dispatches to for structured attack plans (like Recon/Scanner). Architecturally different from `WithPlanner()`. Currently NOT used — Captain's own prompt handles planning adequately; adding a separate PlannerAgent adds a round-trip without benefit.

## Agent Framework Gotchas
- Captain dispatches agents **serially** (Recon → Scanner → Exploit → PostExploit → Reproducer in two batches), not in parallel — `WithEnableParallelTools` is disabled; parallel dispatch causes framework-level issues when skill + localexec toolsets coexist
- `HistoryScope` is **NOT** set to Isolated — the framework default is `HistoryScopeParentBranch`, meaning sub-agents inherit Captain's conversation branch history. Code does not override this default.
- `LocalExec.submit_command` executes immediately (submit+start merged into one async call) — agents MUST poll `get_status` before `get_output`; `start_command` tool no longer exists
- `localexec.Manager` is per-agent, not a global singleton — `LocalExec()` creates a new Manager for each `LocalExecToolSet` instance; global `cache.go` removed。**实例启动时建一次（`initBuiltinToolsets()`，每个执行角色一个），跨轮复用不重建**——上一轮 run 提交的长任务下一轮仍可 `get_status`/`get_output` 续查；`Close()` 会先 kill 所有未结束命令再清空注册表
- **Toolset 生命周期二分：常驻 vs 每轮刷新** — `builtinTools`（15 件文件/日期工具）与 `builtinToolsets`（每角色 localexec）启动时创建，每轮 run 挂到新建的 agent 上，不刷新；`mcpToolsets` 每轮 run 在 `reload()` 里 Close 上一轮实例→按最新配置重建（配置改动下轮即生效）。框架**不会**调用 `ToolSet.Close()`（所有权在调用方），退出回收在 `AgentStart` 的 Exit 分支显式执行
- **MCP 共享实例 = 每 server 全进程一个子进程** — stdio 传输点对点，框架不去重：每个 `NewMCPToolSet` 实例各起一个子进程且无人回收。HackerTeam 的做法是同一实例共享挂载给全部 6 个 agent（`WithToolSets` 不转移所有权，合法），配合每轮 Close+重建与退出 Close，进程数恒定为"启用的 server 数"
- **挂载组装禁止 append 到共享列表** — 给 agent 挂工具/工具集时一律 `make`+`append` 拷入私有 slice，共享清单（`builtinTools` 等）只作 append 的来源。以共享 slice 为目的地的 `append` 在 cap 有余量时会原地写底层数组，造成跨轮/跨 agent 别名改写（无报错、race detector 不报）
- `team.WithMemberToolStreamInner(true)` + `team.WithMemberToolInnerTextMode(team.InnerTextModeInclude)` — TUI shows sub-agent full transcript (text+tool calls+results); use `InnerTextModeExclude` to show only progress signals, hiding assistant text
- **`models.Openai()` / `models.Anthropic()` are canonical model constructors** — `service/engine/session/summarizer.go` and `setAgent()` use these two functions. They handle DeepSeek variant detection, reasoning backfill, and API auth. When creating a new model instance from config, call these instead of manually assembling options.
- **ANSI → tview tag conversion required** — tview's `SetDynamicColors(true)` only supports its own color tag format (`[red]text[-]`). Standard ANSI escape sequences must go through `tview.TranslateANSI()` before writing to a TextView. Without this, ANSI codes appear as visible garbage.
- **Tool response content must be skipped in content rendering** — Both stream and non-stream content paths check `Role != "tool"` to prevent tool JSON from leaking through the main content renderer.
- **Multi-tool results handled in `engineRun.go`** — Framework merges parallel tool results into a single `tool.response` event with N Choices. `agentRunOnce` detects `ObjectTypeToolResponse` and iterates ALL Choices.
- **Glamour markdown rendering** — Non-stream body text is rendered via `glamour` (dracula theme). `document.margin = 0` removes dracula theme's left margin; `strings.TrimRight` strips trailing whitespace to prevent alignment artifacts before tool calls. **Must append `[-:-:-]` after `TranslateANSI(out)`** — glamour's ANSI output may not end with a full reset sequence, leaving unclosed tview tags that leak into the next line (tool calls appear brighter/miscolored).
- **`show_reasoning` config** — `config.Model.ShowReasoning` (`yaml:"show_reasoning"`) controls reasoning/thinking display. Default `false`. Affects both stream and non-stream paths.
- **`maxtokens` config must stay set** — `config.Model.MaxTokens` (`yaml:"maxtokens"`, default 12800; lowered from 32000 for Anthropic SDK limits). If unset the framework falls back to 4096 (DeepSeek API default / Anthropic adapter hardcode), large tool-call JSON (WriteFile content, heredoc commands) hits `finish_reason=length` mid-arguments, and `WithToolCallArgumentsJSONRepairEnabled(true)` then silently "repairs" the truncated JSON into incomplete params. Root-cause analysis: `docs/tool-call-args-truncation.md`
- **`messageRender.go` refactored** — `printMessage` split into `renderStreamEvent`, `renderNonStreamEvent`, `renderToolCall`, `renderToolResult` (`service/engine/messageRender.go`). Tool call/result rendering uses shared `addToolCallMsg`/`addToolResultMsg` helpers with `toolMsgBuffer` mutex. Compact single-line format via `pretty.TToolCompact` — green `●` + orange tool name + dim gray `args → result_summary`. No trailing `\n` (double-newline with next tool's leading `\n` causes alignment shift).
- **embedFS case sensitivity** — `//go:embed` + `ReadFile` paths are case-sensitive on Linux. Always match exact file name case between `go:embed` glob patterns and `ReadFile` calls.
- **Agent status bar (BeforeModel callback)** — `setBeforeModelStatusCallback()`（`service/engine/members.go`）在每次 LLM 调用时向请求**末尾** append 一条 system 状态栏（TIMENOW/CWD/MEMORY）。三条不可违反的设计约束：① 必须 append 到**尾部**而非 prepend——状态栏内容每次调用都变，放头部会破坏服务端自动前缀缓存（实测：尾部 95-99% 命中 vs 头部 0）；② 必须配套 `openai.WithOptimizeForCache(false)`（`service/engine/models/openai.go`）——否则框架的 `optimizeMessagesForCache` 会把尾部 system 重排到头部，缓存收益失效；③ `{{DATE}}`/`{{CWD}}` 占位符已从 `env.md`/`init.go` 移除（与状态栏重叠）。状态栏不进 session（仅存在于当次请求副本），不污染摘要/压缩。详见 `docs/agent-time-awareness-callbacks.md`
- **Input multiplexing (InputChan)** — Tui 字段 `InputChan`（**unbuffered**），`ReadInputAreaPromptWithEnter()` 只注册捕获（非阻塞返回），引擎循环用 `select { case userPrompt = <-tui.InputChannel(): }` 读取（`service/engine/engineRun.go`）。三条约束：① 发送用 select-default——对端（引擎）未监听时不投递且**不清空输入框**（`SetText("")` 只在投递成功时执行），unbuffered send 永不阻塞 tview 事件循环、用户输入永不丢失；② **捕获常驻**——Enter 提交后不注销捕获（旧 `SetInputCapture(nil)` 已删除），agent 运行期间 Enter 被捕获消费（不换行不投递，Shift+Enter 仍可换行、Ctrl+K 帮助仍可用）；③ select 是扩展点——计划任务结果回传（schedule agent）将在 select 上加 TriggerCh 分支 + 前置 DrainPending 检查，勿改回阻塞式 `ReadInputAreaPromptWithEnter() string`。
- **`model/anthropic` 是独立子模块，须与根模块分开升级** — go.mod 里 `trpc.group/trpc-go/trpc-agent-go/model/anthropic` 单独锁版本，升级根模块不会带上子模块修复。2026-08 踩坑：根模块升到修复 commit、子模块仍锁 v1.11.2（不含 #2501 修复），无参 MCP 工具序列化为 `"properties":null`，DeepSeek anthropic 端点报 400。现锁 `v1.11.1-0.20260820131707-cdaece75b478`；官方发布新 tag 后升回正式版

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
