## Environment
- Repo lives on WSL2-mounted NTFS (`/mnt/d/`) — run `git config --global --add safe.directory "/mnt/d/code/my/HackerTeam"` before any git commands (use the actual checkout path if it moves again)
- `git` commands via Bash tool fail with "dubious ownership" without the safe.directory fix above
- `.go` files 多为 CRLF（Windows），但较新的文件是 LF（如 `tools/functions/jsonmapper.go`）；`.md` 文件 LF。CRLF 文件 `Edit` 工具会失败→用 `python3 -c "..."` via Bash；Python 编辑须按文件探测行尾（`splitlines(keepends=True)` + 检测 `\r\n`），勿假设全 CRLF
- `sed -i` fails with "Operation not permitted" on NTFS — write to `/tmp` and `cp` back, or use Python for in-place edits
- CRLF-safe edit for `.go` files: `python3 -c "import pathlib; p=pathlib.Path('file.go'); c=p.read_text(); c=c.replace('OLD','NEW'); p.write_text(c)"`

## Build & Run
- `go build -ldflags "-s -w" -o HackerTeam ./cmd` — build for current platform（根目录无 main.go，入口在 cmd/；`go build ./cmd` 不带 -o 会因输出名与目录冲突失败）
- `go run ./cmd` — run directly (auto-loads config from `<cwd>/.HackerTeam/`)
- `go vet ./...` — static analysis (passes clean)
- `make` / `./build.sh` / `build.ps1` — release builds into `release/HackerTeam` (same `-s -w` ldflags)
- **Tests** — `go test ./...` is part of the gate alongside `go vet ./...` and `go build ./cmd`. Coverage is concentrated on agent topology, skill bootstrap, and TUI layout/collapse invariants; engine internals still rely on integration smoke runs
- `go mod tidy` — sync dependencies after adding/removing imports
- **CGO required** — `service/engine/memory/sqlite` depends on `mattn/go-sqlite3`. Cross-compilation no longer supported; build natively on each platform.
- Go module: `HackerTeam` (Go 1.26.4)

## Architecture
- Two-role AI pentesting platform: **Operator** is the only agent that talks to the user; it owns the engagement end to end and executes the work itself (shell, evidence, attack chain, report, memory, `todo_write`). **Worker** is a stateless, single-use assistant spawned through `start_task_run` for breadth or parallel work. Both are built by the same `agentSpec`/`buildAgent` path in `service/engine/members.go` — same model params, builtin tools, shared localexec toolset, skill repo, and compaction settings; they differ only in prompt, `extraTools`, status-bar flags, and memory preload.
- Delegation is pull-based, not a dispatch loop: Operator calls `start_task_run` (`async` default, `sync` when the next decision depends on it), keeps working, then collects with `wait_task_run` / `get_task_run` / `read_task_run_transcript` / `cancel_task_run`. Worker output is authoritative only in files (`TASK-<id>_result.json`, `TASK-<id>_<time>_<type>_result.md`, `TASK-<id>_<time>_raw/`) because the run `result` is head-truncated (~4000 chars).
- Workers cannot see the conversation, memory, todo list, or any file not named in the brief, and they cannot delegate. Interactive sessions (ssh, msfconsole, reverse shells) stay with Operator. Do not give Worker `todo_write`, memory tools, or taskrun tools — that would turn a disposable execution unit into a second planner with its own state.
- Worker discipline is enforced by concrete behaviors, not tool names: `worker.md` has per-task-type sections (recon never sends payloads; scanning never exploits; verification never expands scope). Forbidding a tool name alone is bypassed by doing the same manual work with the same payloads.
- Shared consensus system in `service/engine/prompts/common/` (embedded via `//go:embed prompts/*` in `service/engine/bootstrap.go`): `vuln_consensus.md` (vulnerability definition + severity rating by technical impact, no CVSS), `output_consensus.md` (output format, raw tool output preservation, vulnerability structured block format for PoC script generation), `command_execution.md` (source of `{{COMMAND_EXECUTION}}` — OS-aware command selection rules), `env.md` (source of `{{ENV}}` — resolved in `buildPrompts()` in `bootstrap.go`)
- TUI built with `charm.land/bubbletea/v2` + `charm.land/bubbles/v2` + `charm.land/lipgloss/v2` (pull-rendered: TUI polls engine state via `tea.Tick` chain, no direct engine→UI calls)
- **Startup (HyperBot v3 structure)**: `cmd/main.go` → `boot.Boot()` → `engine.GetEngineService` → `tui.NewTui(e)` (Engine implements the `EngineView` pull interface) → `go { e.Init(); e.AgentStart() }` → `tui.Run()`. Engine init MUST stay in the goroutine — first-run/config errors end in `parkWithFatal` (permanent residency), and the TUI only renders the fatal screen via the pull contract; synchronous init would block TUI startup before it can render anything.
- **UI**: full-width rounded welcome box on startup (v3 banner, brand title in border, left = welcome + logo, right = system info) + message viewport + todo bar + notice/hint bar + input row. Skills/help overlay (`bubbletea` viewport in a centered rounded box) toggled with Ctrl+K, dismissed with Esc/Ctrl+K; items from engine `SkillItemsJSON`.
- Agent framework: `trpc.group/trpc-go/trpc-agent-go`, MCP: `trpc.group/trpc-go/trpc-mcp-go`
- LLM backends: OpenAI-compatible API or Anthropic native SDK
- Config auto-generated at first run: `<binary-dir>/.HackerTeam/HackerTeam.yaml`
- MCP servers declared in config: `http_mcp`（sse/streamable_http）与 `stdin_mcp`（stdio）两个列表，`enabled` 开关控制；`name` 必须唯一（决定工具前缀 `{name}_{toolName}`，缺省时分配 `mcp_N` 默认名）；每个可配 `timeout` 字段（单位/秒，`<=0` 落默认值 `defaultMCPTimeout = 2min`）——框架把超时包在**每次 CallTool 上**（不是连接超时），长分析型调用务必调大；启用的 server 挂载给 Operator 与 Worker，同一实例跨 agent 共享
- TUI colors centralized in `utils/pretty/pretty.go` (TuiXxx constants)
- `/new`, `/exit`, `ESC` — built-in TUI commands（`/flush` 已移除；每轮 run 重建配置/技能/MCP工具集，内置工具与工具集（localexec）常驻、跨轮复用）
- Agent prompts embedded via `//go:embed prompts/*` in `service/engine/bootstrap.go` (`PromptFiles`) and `service/engine/session/summarizer.go` (`promptFiles`, `prompt/*` prefix)
- Adding a new shared consensus prompt pattern: 1) create `service/engine/prompts/common/<name>.md`, 2) add field in `Engine` struct (`service/engine/engineCore.go`) + load in `buildPrompts()` in `bootstrap.go` (follow `VulnConsensusPrompt` pattern), 3) add `{{<NAME>}}` replacement in `assemblePrompt()` in `members.go`, 4) add `{{<NAME>}}` placeholder to each agent prompt `.md` file
- `{{OUTPUTDIR}}` is replaced in `assemblePrompt()` (`members.go`), same as the other placeholders — agent prompts get the absolute output root, never a literal. `env.md` receives it through `{{ENV}}` as well.
- Adding an agent: 1) create `service/engine/prompts/agents/<name>.md` with the shared `{{ENV}}`/`{{COMMAND_EXECUTION}}`/`{{VULN_CONSENSUS}}`/`{{OUTPUT_CONSENSUS}}` placeholders you need; 2) add a constructor in `members.go` returning `buildAgent(agentSpec{...})` and set `extraTools`/status flags/memory preload there; 3) register it with `runner.WithAgentFactory(<name>, ...)` in `init.go` so `start_task_run` can resolve it. Do not fork the assembly path — every agent must stay the same implementation with different permissions.

## Directory Map
- `cmd/main.go` — entry point → `boot.Boot()`
- `boot/boot.go` — startup orchestration: `GetTuiService()` → `go { GetEngineService → AgentStart }` → `tui.Run()`
- `service/engine/` — Engine core (refactored from `global/` + `bootstrap/` + `handler/` + `config/` + `models/` + `session/` + `memory/` + `functionTools/` + `toolsets/`)
  - `engineCore.go` — `Engine` struct (all state: config pointer, runner, session, memory, single `SkillsFolderPath`, prompt strings), `PromptFiles`/`ToolSkills` are embedded in `bootstrap.go` (`//go:embed prompts/*` + `skillsTemplates/*`), `GetEngineService()`, `AgentStart()` main dialog loop, `randomStartID()`
  - `init.go` — `newRunner()` (Operator factory + `runner.WithAgentFactory("Worker", ...)` + in-process taskrun service), `reload()` (per-turn rebuild, constructs the Worker and atomically publishes it), `skillRepository()`, `loadMCPFromConfig()`/`refreshMCPFromConfig()` (MCP工具集每轮run Close+重建), `LoadConfig()`; `tuiService` interface definition
  - `bootstrap.go` — `Check()` startup environment pass (config/log/session/memory/inspect assets), `BootEnv`, `checkSkillsFolder()` (single `Skills/` dir, whole-tree copy via `makeSkillsWritable()`), `builtinAssets()` (内置工具 + single shared localexec toolset, 启动时建一次跨轮常驻), `redirectFrameworkLog()`
  - `members.go` — `agentSpec` + `buildAgent()` (the single assembly path), `initOperator()`/`initWorker()`, `setAgent()` (model selection by APIType), `assemblePrompt()` (all placeholders incl. `{{OUTPUTDIR}}`)
  - `flight.go` — `[IN-FLIGHT]` summary of background task runs appended to Operator model requests (not TUI)
  - `status.go` — per-request status bar: base time/CWD/memory for both agents; todo / memory / in-flight sections only for Operator (`agentSpec` flags)
  - `engineRun.go` — dialog loop + single-turn execution (`agentRunIteratively`/`agentRunOnce`), turn types, merged from old `handler/runIteratively.go` + `runOnce.go` + `model.go` + `bootstrap/Bootstrap.go`
  - `messagerender/` — message rendering subpackage（`MessageRender.RenderResponse` 每 choice 调一次 → stream/non-stream 事件渲染 + 工具消息 gather/render）；`messageRender.go` + `toolmessagebuffer.go`。由 `engineRun.go` 事件循环调用
  - `config/` — `config.go` (Config struct + `LoadConfig`), `config.yaml` (checked-in sample user config), `mcp.go` (HttpMCP/StdinMCP 配置结构体), `template.go` (YAML template via `//go:embed`，含 `http_mcp`/`stdin_mcp` 示例段)
  - `memory/sqlite.go` — SQLite memory service factory (no extractor; explicit memory tools only)
  - `session/` — summarizer, session service, prompt embedding (`prompt/*`)
  - `models/` — LLM provider constructors (OpenAI, Anthropic SDK wrappers)
  - `prompts/agents/` + `prompts/common/` — `operator.md` / `worker.md` + shared consensus prompts (embedded)
  - `skillsTemplates/Skills/` — embedded skill presets，与运行时 `Skills/` 同构：hacktricks-* 红队知识 + poc-scripting + pentest-tools 占位模板
  - `tools/functions/` — Custom Go function tools（`jsonmapper.go`：`Mappers` 接口 + `InjectMapper` 注册本包工具的渲染字段表，工具名用常量）
  - `tools/parammapper.go` — `ParamMapper{Name,In,Out}` + `Mappers` 具体切片 + `GetParamMapper()`（聚合各工具包的 `InjectMapper`）
  - `tools/toolsets/localexec/` — LocalExec toolset（command execution subsystem；`Close()` 先 kill 未结束命令再清注册表；`mapper.go` 自注册同形 `Mappers` 接口 + `InjectMapper`，工具名常量、带前缀名构造）
  - `tools/toolsets/mcp.go` — MCP ToolSet wrappers (`HttpMCP()`/`StdinMCP()`：`WithName` 决定工具前缀、`WithSessionReconnect(3)`、单次调用超时配置化默认 2min、`friendlyToolSet` 装饰器把超时错误改写成带时长与出路的文本——框架把 `context deadline exceeded` 原文回给模型只会引来无意义重试，改框架文本不可行（远端 pinned 依赖无 replace），装饰器是唯一拦截点)
- `service/tui/` — bubbletea TUI: `tui.go` (model/update/view 生命周期), `pull.go` (tea.Tick 拉取链), `render.go` (消息日志重放渲染 + boundaryViews 边界记录分发表), `collapse.go` (思考/工具块折叠、块行号索引与点击命中), `engine.go` (EngineView 接口), `handler.go` (按键), `banner.go` (欢迎盒), `bottom.go`/`spinner.go`/`styles.go` (状态消息宽度感知折行)
- `utils/pretty/` — Centralized TUI color constants (`TuiXxx`)

## Dependencies
- `trpc-agent-go` v1.11.1-0.20260820131707-cdaece75b478 — main agent framework (no fork/replace; pseudo-version containing #2501 fix)
- `trpc-mcp-go` v0.0.18 — MCP SDK（仅用于日志重定向，无 MCP 工具集）
- `tcell/v2` v2.13.10、`anthropic-sdk-go` v1.66.0
- `glamour` v2 (`charm.land/glamour/v2`) — Markdown → ANSI renderer (crush-style StyleConfig in `render.go`; bubbletea renders ANSI natively, no tag conversion)
- `gjson` v1.19.0 (`github.com/tidwall/gjson`) — 工具消息参数/结果字段提取（点路径取值，不反序列化；本为 indirect，现转直接）

## Skill System
- External security tools (nmap, nuclei, sqlmap, etc.) are integrated as knowledge-only skills via `trpc-agent-go`'s built-in skill system — NOT as function tools
- Skills use `llmagent.WithSkillToolProfile(llmagent.SkillToolProfileKnowledgeOnly)` — injected into system prompt, execution still via LocalExec
- Skills live in one shared directory: `.HackerTeam/Skills/` — Operator and Worker read the same index; there is no per-role split anymore
- Embedded skill presets: `service/engine/skillsTemplates/Skills/` — same layout as the runtime folder. On first start `checkSkillsFolder()` copies the whole tree into `.HackerTeam/Skills/` (installed once; an existing folder is never repopulated) and `makeSkillsWritable()` fixes the read-only embed permissions. Presets include hacktricks-* distilled knowledge (CC BY 4.0, attribution in README) + a `pentest-tools` placeholder (`SKILL.md.template`, rename to `SKILL.md` and fill in your tool inventory)
- Skill repos re-created automatically on every turn — `reload()` builds one `skill.NewFSRepository(e.SkillsFolderPath)` and `buildAgent()` hands the same repo to both Operator and Worker (no `/flush` needed)

## Terminology
- "Planner" in trpc-agent-go = `planner.Planner` interface (agent-level request/response hooks: `BuildPlanningInstruction` + `ProcessPlanningResponse`). Mounted via `llmagent.WithPlanner()`, NOT a tool. Builtin planner just sets `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` on model request — equivalent to manual config, no prompt injection. React planner injects `/*PLANNING*/`/`/*ACTION*/`/`/*FINAL_ANSWER*/` tags + prevents premature `Done=true` via response post-processing.
- "Planner" as an agent = a separate LLMAgent invoked through `start_task_run` for structured plans. Architecturally different from `WithPlanner()`. Currently NOT used — Operator's own prompt handles planning; a separate planner agent adds a round-trip without benefit.

## Agent Framework Gotchas
- Operator executes directly; it is not a dispatcher. `start_task_run` is pull-based and parallel-safe: multiple Workers can run concurrently in isolated child sessions, but they share the Operator's LocalExec job table, so concurrent shell work must not fight over the same interactive session. Interactive sessions stay with Operator.
- `LocalExec` blocks up to 20s — it returns status/exit code/output inline if the command finishes within that window, otherwise switches it to background execution and returns the job id; agents then use `CommandStatus` / `CommandOutput` / `CommandIntervene` / `CommandKill`. The old `submit_command` + mandatory-poll workflow is gone
- **One localexec Manager for the whole instance** — `builtinAssets()` builds a single `execToolSet`, and both Operator and Worker mount the same instance. Jobs started by a Worker remain visible to Operator. `Close()` kills every unfinished command and clears the registry on exit. Do not create a Manager per agent: that would split the job table and orphan Worker-launched processes.
- **Toolset 生命周期二分：常驻 vs 每轮刷新** — `builtinTools`（文件/日期工具）与 `execToolSet`（共享 localexec）启动时创建，每轮 run 挂到新建的 agent 上，不刷新；`mcpToolsets` 每轮 run 在 `reload()` 里 Close 上一轮实例→按最新配置重建（配置改动下轮即生效）。框架**不会**调用 `ToolSet.Close()`（所有权在调用方），退出回收在 `AgentStart` 的 Exit 分支显式执行
- **MCP 共享实例 = 每 server 全进程一个子进程** — stdio 传输点对点，框架不去重：每个 `NewMCPToolSet` 实例各起一个子进程且无人回收。HackerTeam 的做法是同一实例共享挂载给 Operator 与 Worker（`WithToolSets` 不转移所有权，合法），配合每轮 Close+重建与退出 Close，进程数恒定为"启用的 server 数"
- **挂载组装禁止 append 到共享列表** — 给 agent 挂工具/工具集时一律 `make`+`append` 拷入私有 slice，共享清单（`builtinTools` 等）只作 append 的来源。以共享 slice 为目的地的 `append` 在 cap 有余量时会原地写底层数组，造成跨轮/跨 agent 别名改写（无报错、race detector 不报）
- **Worker 的构造收敛在 `reload()` 单 goroutine** — `runner.WithAgentFactory("Worker", ...)` 每次 run 都会调用工厂且不做缓存，所以工厂里只读 `atomic.Pointer`；Worker 实例只在 `reload()` 里构造并原子发布，避免与字段写入并发。
- **`models.Openai()` / `models.Anthropic()` are canonical model constructors** — `service/engine/session/summarizer.go` and `setAgent()` use these two functions. They handle DeepSeek variant detection, reasoning backfill, and API auth. When creating a new model instance from config, call these instead of manually assembling options.
- **ANSI is consumed natively** — bubbletea's renderer is an ANSI consumer; no tag conversion needed. 工具行等 compact 输出用 `ansi.Strip` 压平（见 styles.go 的 compactLine）后再上色，避免 PTY 输出的转义码串色。
- **Tool response content must be skipped in content rendering** — Both stream and non-stream content paths check `Role != "tool"` to prevent tool JSON from leaking through the main content renderer.
- **Multi-tool results handled in `engineRun.go`** — Framework merges parallel tool results into a single `tool.response` event with N Choices. `agentRunOnce` detects `ObjectTypeToolResponse` and iterates ALL Choices.
- **Glamour markdown rendering** — 引擎不 import glamour：streaming choice 以 `emitChoice` 透传进消息日志（delta/message 记录），TUI 的 `render.go` 在重放时渲染（crush StyleConfig + `glamourTailPad` 剥行尾填充 + 补行尾 reset）。摘要提示走 `AppendSummary`（summarySink 小接口），不走 PrintToMsgView。
- **思考/工具块默认折叠（`collapse.go`）** — `composeView` 产出 `[]viewBlock`（可折叠块带稳定 ID：思考 `think:<记录序号>`、流式尾段 `tail:think`、工具 `tool:<消费缓冲的结果记录序号>`），`composeBlocks` join 时算出块首行号 `blockSpan` 随 `frameMsg` 回事件循环；左键点击头部行（`mouseClickHandler` 用屏幕行 + `vp.YOffset()` 反查）切换展开态并设滚动锚，下一帧把块头钉回点击行。三条不变量：① 折叠态每行必须按本轮宽度截断——折叠行不折行，终端视觉换行会让整屏行号错位，点击命中错块；② 展开状态是唯一「事件循环写、pull goroutine 读」的 model 状态（`expandState` 互斥锁 + 单调版本号，帧记录重组起点版本供锚点判新旧）；流式尾段清理只能放 `applyFrame` 的迁移检测，放进 compose 重放路径会被历史记录反复触发、抹掉用户刚点开的展开；③ `/new` 由 TUI 先 `NewConversation()`（引擎只原子换 session）再 `ClearRecords()`，`/clear`、`/resume` 成功换历史后同样 `resetExpand()`；running 拒绝时历史与展开态都保持原样。
- **`show_reasoning` config** — `config.Model.ShowReasoning` (`yaml:"show_reasoning"`) controls reasoning/thinking display. Default `false`. Affects both stream and non-stream paths.
- **`maxtokens` config must stay set** — `config.Model.MaxTokens` (`yaml:"maxtokens"`, default 12800; lowered from 32000 for Anthropic SDK limits). If unset the framework falls back to 4096 (DeepSeek API default / Anthropic adapter hardcode), large tool-call JSON (WriteFile content, heredoc commands) hits `finish_reason=length` mid-arguments, and `WithToolCallArgumentsJSONRepairEnabled(true)` then silently "repairs" the truncated JSON into incomplete params. Root-cause analysis: `docs/tool-call-args-truncation.md`
- **工具消息渲染（messagerender + parammapper）** — `MessageRender.renderToolMessages` 对每个"结果已到"的工具消息打印一次（`hasResult`/`printed` 标志去重；调用方 `engineRun.go` 每轮 `agentRunOnce` 新建 `MessageRender`，buffer 跨轮不泄漏）。字段提取：`tools.GetParamMapper()` 聚合各工具包自注册的 `InjectMapper`（`functionTools`/`localexec` 各自定义同形 `Mappers` 接口，`*tools.Mappers` 结构化满足；`tools`→两包均无环）；`renderToolMessageByMapper` 用 **gjson** 点路径取 `In`/`Out` 字段，未注册工具回退原始 JSON。格式：`pretty.TToolCompact(name,args,result)` 紧凑单行，args/result 为 `""`/`"()"`/`"{}"` 跳过、`→` 随 result 出现（args 可空）。
- **框架工具名前缀** — toolset 工具运行时名是 `{toolsetName}_{toolName}`（如 localexec `WithName("run")` + toolset 名 `"LocalExec"` → LLM 看到 `LocalExec_run`）。mapper 须用带前缀名；localexec 用 `localExecToolSetName+"_"+toolName` 构造。新增 toolset 工具的字段映射记得用前缀名。
- **加新工具的字段映射** — 在所属工具包写 `InjectMapper(m Mappers)`（或加入既有），`m.AddMapping(<实际名>, In字段路径, Out字段路径)`；若引入新工具包，在 `tools/parammapper.go` 的 `GetParamMapper()` 加一行调用其 `InjectMapper`（注意无环：工具包不得反向 import `tools`）。
- **框架内置工具（todo_write，Operator 专属）** — 来自框架（`tool/todo`），封装在 `tools/functions/todo.go`：任务清单写进 session state（`temp:todos[:branch]`，按 invocation branch 分键隔离，Worker 的 child session 不会覆盖 Operator 的计划）；**只挂给 Operator**——在 `initOperator`（`members.go`）的 `extraTools` 里单独 append，刻意不放进 `initBuiltinTools()` 的共享 `builtinTools`；使用说明由 `{{TODO_PROMPT}}` 占位符注入 `prompts/agents/operator.md`，内容取 `todo.DefaultToolPrompt`（`assemblePrompt` 替换，随框架版本走，不硬编码）。渲染字段在 `jsonmapper.go` 用常量 `todoWriteToolName` 注册，只显示 `message`（避免整份清单 JSON 刷屏）。**注意**：`await_user_reply`（`tool/awaitreply`）未启用——Worker 是一次性 child session，不能向用户追问；brief 不完整时它返回 `needs_input`，由 Operator 决定补料还是自己接手。
- **embedFS case sensitivity** — `//go:embed` + `ReadFile` paths are case-sensitive on Linux. Always match exact file name case between `go:embed` glob patterns and `ReadFile` calls.
- **Agent status bar (BeforeModel callback)** — `setBeforeModelStatusCallback()`（`service/engine/members.go`）在每次 LLM 调用时向请求**末尾** append 一条 system 状态栏（TIMENOW/CWD/MEMORY）。三条不可违反的设计约束：① 必须 append 到**尾部**而非 prepend——状态栏内容每次调用都变，放头部会破坏服务端自动前缀缓存（实测：尾部 95-99% 命中 vs 头部 0）；② 必须配套 `openai.WithOptimizeForCache(false)`（`service/engine/models/openai.go`）——否则框架的 `optimizeMessagesForCache` 会把尾部 system 重排到头部，缓存收益失效；③ `{{DATE}}`/`{{CWD}}` 占位符已从 `env.md`/`init.go` 移除（与状态栏重叠）。状态栏不进 session（仅存在于当次请求副本），不污染摘要/压缩。详见 `docs/agent-time-awareness-callbacks.md`。**Todo 注入**：同一 callback 还会追加 `functionTools.TodoStatusBar(ctx)`——经 `agent.InvocationFromContext(ctx)` 取回 invocation（BeforeModel 的 ctx 由框架 `llmflow.go` `withInvocationContextIfMissing` 兜底携带），用框架公开的 `todo.GetTodos(inv.Session, inv.Branch)` 读清单（与 todo_write 同 key `temp:todos[:branch]`），渲染成单行紧凑摘要（`[TODO] ◐ 进行中 | ☐ 待办... (N done)`，pending 最多 8 条、单条 80 rune 截断）。无清单返回空串不追加。清单变化只影响尾部消息不破坏前缀缓存；todo_write 写入后同轮下一跳请求即生效，上下文压缩只裁历史不动 session state，清单不丢。
- **Input multiplexing (InputChan)** — Tui 字段 `InputChan`（**unbuffered**），`ReadInputAreaPromptWithEnter()` 只注册捕获（非阻塞返回），引擎循环用 `select { case userPrompt = <-tui.InputChannel(): }` 读取（`service/engine/engineRun.go`）。三条约束：① 发送用 select-default——对端（引擎）未监听时不投递且**不清空输入框**（`SetText("")` 只在投递成功时执行），unbuffered send 永不阻塞 tview 事件循环、用户输入永不丢失；② **捕获常驻**——Enter 提交后不注销捕获（旧 `SetInputCapture(nil)` 已删除），agent 运行期间 Enter 被捕获消费（不换行不投递，Shift+Enter 仍可换行、Ctrl+K 帮助仍可用）；③ select 是扩展点——计划任务结果回传（schedule agent）将在 select 上加 TriggerCh 分支 + 前置 DrainPending 检查，勿改回阻塞式 `ReadInputAreaPromptWithEnter() string`。
- **`model/anthropic` 是独立子模块，须与根模块分开升级** — go.mod 里 `trpc.group/trpc-go/trpc-agent-go/model/anthropic` 单独锁版本，升级根模块不会带上子模块修复。2026-08 踩坑：根模块升到修复 commit、子模块仍锁 v1.11.2（不含 #2501 修复），无参 MCP 工具序列化为 `"properties":null`，DeepSeek anthropic 端点报 400。现锁 `v1.11.1-0.20260820131707-cdaece75b478`；官方发布新 tag 后升回正式版

## Manual Memory (SQLite)

Persistent long-term memory using SQLite. No background extractor — every write is an explicit tool call by Operator (search-before-store, on-the-spot writes, in-place maintenance). Operator is the sole memory manager — Worker does NOT get memory tools. (Mirrors HyperBot PR #12.)

### Architecture
- `service/engine/memory/sqlite.go` — factory: creates `memorysqlite.Service` with `WithSoftDelete` + `WithMemoryLimit(100000)` + `WithToolEnabled(memory.DeleteToolName, true)`. No extractor. Agentic mode: `enabledTools` is the exposure gate — framework default set is add/update/search/load; Delete needs the explicit enable; `memory_clear` is not in the set (dangerous op).
- `service/engine/engineCore.go` — `SqliteMemoryService *memorysqlite.Service` field on `Engine` struct
- `service/engine/bootstrap.go` — `Check()` calls `memory.NewSQLiteMemoryService(filepath.Join(env.ConfigFolderPath, memoryDBFileName))` and carries it on `BootEnv`; `absorb()` copies it onto `Engine`.
- `service/engine/members.go` — `initOperator()` appends `SqliteMemoryService.Tools()` (exposes `memory_search`/`memory_load`/`memory_add`/`memory_update` to Operator only) and sets `WithPreloadMemory(10)`. Worker does NOT get memory tools — only Operator manages memory.
- `service/engine/prompts/agents/operator.md` — `# Memory` section defines memory behavior: search-before-store, on-the-spot writes, in-place maintenance, subject+alias+timestamp discipline

### Operator considerations
- Operator is the sole memory manager — all memory creation, update, and deletion happens through its explicit tool calls
- Worker benefits indirectly — Operator can reference past operations and store useful patterns discovered during pentest
- No background extraction — memory exists only where Operator explicitly wrote it; stale entries are corrected/deleted on the spot when they surface in the [MEMORY] preload or a search

### Gotchas
- `SqliteMemoryService` MUST be on `Engine` before `newRunner()` — the Operator factory reads `SqliteMemoryService.Tools()`, nil service → panic
- `stdlog.SetOutput(file)` in `redirectFrameworkLog()` redirects gse dictionary-loading chatter away from TUI
- Default memory limit: 100000 (`service/engine/memory/sqlite.go:WithMemoryLimit`)
- `memory_delete` is NOT in the framework's agentic default tool set — it only works because of the explicit `WithToolEnabled(memory.DeleteToolName, true)` in the factory

## Context Management

HackerTeam uses three complementary mechanisms to prevent context overflow:

### 1. Session Summarization (`service/engine/session/summarizer.go` + `service/engine/members.go`)
- `WithAddSessionSummary(true)` on both agents (Operator + Worker) enables async summary injection
- Summarizer triggers at `CheckTokenThreshold(0.6 * contextwindow)` via `WithChecksAny` (time threshold removed — idle sessions would be compressed for no benefit)
- `WithSkipRecent` preserves the last complete interaction cycle (from last user message to tail) from being summarized — keeps current turn intact in prompt
- `WithToolResultFormatter` truncates tool results to 1000 runes (head 500 + tail 500) before entering summary model input — especially valuable for Worker tool outputs (nmap, sqlmap) that can be 50K+ tokens. Only affects summary input; original events remain intact
- `WithSyncSummaryIntraRun(true)` on both agents — enables synchronous summary refresh between LLM loop iterations. Critical for Workers running long command chains (nmap scans, exploit attempts) where async summary may arrive too late
- `WithSessionSummaryInjectionMode(SessionSummaryInjectionUser)` on Worker only (`summaryToUser: true`) — injects summary into the user message instead of the system message. Worker's system prompt carries the SOP, command rules and output contract; keeping the system area clean prevents the summary from competing with it
- Token counting uses `model/tiktoken` (BPE), configured via `summary.SetTokenCounter(counter)`
- Summary model is the same as main model; for DeepSeek reasoning models, the token counter falls back to `cl100k_base` (within ~4-7% of DeepSeek's actual count per empirical testing)
- **Worker risk**: a Worker's tool results (nmap, sqlmap, gobuster) can be 50K+ tokens each; several parallel Workers multiply that. If the summarizer's first attempt fails, the child session grows unbounded — check `HackerTeam.log` for "summary worker failed". ToolResultFormatter truncating to 1000 runes reduces but does not eliminate this; main-conversation overflow is handled by Compaction
- Post-summary hook strips `<think>...</think>` tags from summary text

### 2. Context Compaction (`service/engine/members.go`)
- `WithEnableContextCompaction(true)` on both agents enables deterministic tool result compression before each LLM call
- **Pass 1**: Historical tool results > 1024 tokens → replaced with placeholder (`event_id`/`tool_call_id` preserved for `session_load` recovery)
  - Protects current invocation + `KeepRecentRequests` (default 1) most recent completed invocations
- **Pass 2**: Any tool result > 8192 tokens → head+tail truncation with `[...N chars truncated...]` marker
  - Applies to ALL invocations including current; gated on `OversizedToolResultMaxTokens > 0`
  - Critical for Workers: a single nmap/sqlmap output truncated from 50K to ~16K before entering summarizer input
- Triggers at 70% context window (`ContextCompactionThresholdRatio`, default 0.7)
- If still over threshold after compaction → sync `CreateSessionSummary` runs as fallback → request rebuilt
- `ForceCleanToolNames`/`KeepToolNames` available for per-tool policy (not currently configured)

### 3. On-Demand Session (`service/engine/members.go`)
- `WithEnableOnDemandSession(true)` on both agents gives `session_load`/`session_search` tools
- Compacted/truncated tool results can be retrieved by `event_id` with `content_offset`/`content_limit`
- Enables sliced loading of large outputs (e.g. read nmap port list without loading full scan)

### Troubleshooting Context Overflow
- **Symptom**: API error "requested X tokens exceeds maximum 1048565" (X > 1M)
- **Check**: `HackerTeam.log` for "summary worker failed" — if present, summaries are failing
- **Verify**: `contextwindow` in config MUST be ≤ actual model limit
- **Note**: tiktoken `cl100k_base` vs DeepSeek API token count differs ~4-7% (empirically verified) — not accurate enough to explain 2x+ discrepancies
- **Root cause pattern**: first summary attempt fails → delta grows unbounded → cascade failure → permanent retry loop
- **Fix priority**: (1) enable Context Compaction for tool result size control, (2) lower `CheckTokenThresholdPercent` if needed, (3) use non-reasoning model for summarization as last resort

## TUI v3（整体移植自 HyperBot v3.0.0, commit 9f829f1 "优化TUI展示"）

- **新布局** — `service/tui/tui.go`。`MainFlex`(FlexRow) 四段：`AgentMessage`(弹性) + `TodoBar`(0~N 行动态) + `NoticeBar`(1 行常驻) + `InputRow`(1 行，左侧 2 列运行指示器 `indicator` + `InputArea`)。装饰性 StatusBar 与右侧 InputHint 已移除，键位提示由 NoticeBar 常驻 hint 承担。
- **重绘节流** — 所有 widget 写入走 `QueueUpdate`（只写不画）→ `markDirty()` → `drawLoop`（30ms）按 `dirty` 原子标志节流 `app.Draw()`。`markDirty()` 必须在写入完成**之后**调用；drawLoop 不能删（纯流式输出期间 tview 不会自行重绘）。它身兼重绘/spinner 动画/TodoBar/NoticeBar 四职，单 ticker 驱动，私有状态收在 drawLoop 局部 `drawState`，无需加锁。
- **glamour** — `glamourRenderer()` 按消息区宽度缓存 renderer；宽度必须在 `app.QueueUpdate` 内读（`GetInnerRect` 无锁，跨 goroutine 读是数据竞争）。渲染失败退回原文（`messagerender/messageRender.go`），不要吞掉整条回复。
- **TodoBar/NoticeBar** — TodoBar 纵向每个任务一行，高度 0↔N 行（`ResizeItem`，proportion 必须 0；矮终端按 `minMessageRows=3` 钳制负高度）；NoticeBar 固定 1 行永不塌陷。多行 SetText 后必须 `ScrollToBeginning()`；`SetWrap(false)` 是刻意的（行数可精确计算）。
- **todoTextSink** — `status.go` 的 `injectTodoStatus` 把 `TodoStatusBar` 输出同时注入 prompt 尾部与推给 sink。**空串也要推**（清单清空时 TodoBar 靠空串塌回 0 行）。装配约定：`agentSpec.todoSink` 只有 Operator 设为 `(*e).tui`（`withTodo: true`），Worker 不挂 todo 工具也不推 sink——否则 Worker 每跳的状态栏会清掉 Operator 的 TodoBar；prompt 注入只发生在 Operator 的请求上。
- **转义契约** — `ShowNotice` 收受信 markup（`pretty.TBarXxx`），**不转义**；`SetTodoText` 收框架/LLM 纯文本，**必须 `tview.Escape`**（否则 `[TODO]`、`[urgent]` 等会被 tview 当颜色标签整个吞掉）。转义只放 TUI 边界，绝不放进 `renderTodoStatusBar`（输出同时喂 prompt）。宽度度量用 `tview.TaggedStringWidth` 且必须先 Escape 再度量。
- **错误自动重试（errorStreak 语义 = "配置层健康检查"，非"有界重试预算"）** — `engineCore.go` 常量 `errorMaxTimes=3`/`errorSleepGap=3s` + `Engine.errorStreak`。归零时机：收到任何带 Choices 的 Response 事件（第一个 token 即归，含流式部分块——能吐 token = key/端点/鉴权/本地网络全通，故障只剩传输层抖动，随机性强、重试期望为正，自动重试到成功为止）、/new、ESC(Int)、`AgentStart` 的 else 兜底；**手动提交输入不归零**。耗尽只可能由零输出失败（401/连接拒绝/上下文超限 400）触发，"连续 N 次失败"的实际含义是"检查配置"；中途死亡类失败（流式出字后断）永不耗尽——确定性中途失败（超时、内容过滤）会无限自动重试是刻意接受的代价，配套动作是把主模型 `httptimeout` 调大。耗尽后 Code 保持 Error、streak 不归零、整个复用 `*EndTurn_p`。`agentRunIteratively` 输入循环是"Error 优先 + else 兜底"结构，不要改回列举式（新增 turnCode 会落进安全兜底）。
- **启动横幅** — `service/tui/banner.go`。挂在 `Startup` turnCode 上（该 code 永远不会被 `agentRunIteratively` 返回，横幅必然只打一次）；Engine 侧 `startupInfoLines()` 决定内容（skills 数 = `Skills/` 下含 `SKILL.md` 的子目录数），TUI 侧 `compose(width)` 决定拼版；横幅是"死文本"，resize 不重排。
- **输入复用** — `InputChan` 改为未导出 `inputChan`；`ReadInputAreaPromptWithEnter()+InputChannel()` 合并为 `ListenUserInput() chan string`。引擎循环 `select { case userPrompt = <-(*e).tui.ListenUserInput(): }`。unbuffered + select-default 投递语义不变。
- **系统消息不带装饰符号** — 语义由颜色承载（红=错误、黄=警告、绿=成功、青=信息）。样式 helpers 在 `service/tui/styles.go`（charmtone 调色板）：`errText`（ERROR 徽章 + 弱化正文）/`successText`/`warnText`/`summaryText`（状态消息）与 `noticeXxx`（通知栏）不带符号。状态消息与正文同签"**compose 阶段排完版、viewport 零改写**"契约（viewport SoftWrap=false 只横向裁切不换行）：宽度作参数（error/warn/summary 经 render.go 的 boundaryViews 分发表扣 `msgIndentW`，fatal 整行宽），经 `wrapStatusText` 清洗（剥 ANSI、\r 归一、tab 展开——tab 测宽为 0 而终端展开到制表位，含 tab 行会"测量达标、实际超宽"越界）后按 `ansi.Wrap` 词级折行、超长无空格词回退按列硬折；`errText` 徽章只出首行、续行悬挂缩进 8 列；多行文本经 `colorLines` 逐行独立 SGR（成对 reset，防跨行串色）。
- **每类事件只打一次** — 错误在 `agentRunOnce` 源头各打一次（RunError/TerminalError），中断在 `Ctx.Done` 分支打一次；`agentRunIteratively` 顶部只处理 Startup（横幅）与 New（通知），Error/Int 不再重复打印。
- **接口收敛** — `init.go` 的 tuiService 是 Engine 专属门面（新增 ListenUserInput/SetAgentRunning/ShowNotice/ShowStartupBanner/SetTodoText；移除 ReadInputAreaPromptWithEnter/InputChannel/ShowSuccessInMsgView/StatusBar*）；session 包持有单方法 `msgPrinter` 小接口。`ShowSuccessInMsgView` 已删除（职责迁去 NoticeBar），`ShowSuccessInMsgViewAndExit` 保留。
