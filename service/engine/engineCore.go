package engine

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"HackerTeam/service/engine/config"
	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/taskrun/inprocess"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/taskrun"
)

type Agentrunner struct {
	Runner runner.Runner
	Stream bool
}

// Engine 封装核心状态变量（原 global 包级变量）。
// 对上层 UI 的可观察状态与 pull 契约见 uistate.go；错误预算见 errorBudget.go。
type Engine struct {
	Config_p             *config.Config  //yaml配置
	Agentname            string          //Agent名称
	CWD                  string          //当前工作目录（os.Getwd()）
	ExeDir               string          //可执行文件所在目录，资产锚点
	ConfigFolderPath     string          //配置文件夹路径
	HackerTeamConfigPath string          //配置文件路径
	AgentRunner_p        *Agentrunner    //Runner，全局唯一
	SessionService_p     session.Service //会话服务，包含自动摘要功能
	SqliteMemoryService  memory.Service  // sqlite记忆服务
	FrameworkLogFile_p   *os.File        // 保存日志文件句柄，防止被 GC 回收

	EnvPrompt              string //环境上下文提示词（prompts/common/env.md，已替换占位符）
	CommandExecutionPrompt string //共享的命令执行提示词片段
	VulnConsensusPrompt    string //共享的漏洞定义与定级共识提示词
	OutputConsensusPrompt  string //共享的结果输出规范提示词
	OutputDir              string //本次运行的输出根目录，提示词里的 {{OUTPUTDIR}} 由它替换

	SessionId string
	RequestId string

	builtinTools []tool.Tool // 内置function工具清单（文件系统/文件操作/日期），启动时确定，跨轮常驻不刷新
	// execToolSet 是 Operator 与 Worker 共享的常驻 localexec 作业表，
	// 启动时确定、跨轮常驻，jobs 可跨轮续查。
	execToolSet tool.ToolSet
	mcpToolsets []tool.ToolSet // 配置文件声明的MCP工具集，增量对齐（同名复用），共享挂载给全部agent

	// skillsRepo 是合并后 Skills 目录的唯一索引：Operator 与 Worker 用同一份技能库，
	// 按任务自选技能，而不是靠预先把角色切碎。每次 reload 重建（技能文件可编辑）。
	skillsRepo *skill.FSRepository

	// worker 由 reload 在主 goroutine 上构造后原子发布；taskrun 的 worker 工厂只读它。
	// 这样 Worker 的构造不会与 reload 的字段写入并发。
	worker atomic.Pointer[llmagent.LLMAgent]

	// taskService/taskTools：agent 级后台任务的调度面。Operator 通过 taskrun 工具
	// 起 Worker；Worker 本身是一次性 child session，不持有跨轮记忆。
	taskService *inprocess.Service
	taskTools   *taskrun.Tools

	// SkillsFolderPath 合并后的技能目录：Operator 与 Worker 共用同一份索引。
	SkillsFolderPath string

	// errBudget：连续错误自动重试预算，状态流转见 errorBudget.go
	errBudget errorBudget

	// 启动状态（BootXxx 常量，经 FirstStartupJSON 暴露给 TUI）：
	// booting → （首次启动）config → ready，或 booting → ready。
	// firstStartupFields 是 config 态下配置页的预填值快照。
	bootState          string
	firstStartupFields map[string]string

	// ── 对上层 UI 暴露的可观察状态（pull 契约，方法见 uistate.go）──
	mu        sync.Mutex // 串行化引擎各 goroutine 的写入与 UI goroutine 的读取
	records   []string
	version   uint64
	runState  RunState
	todoText  string
	notice    notice
	startup   []string
	startupOK bool
	inputCh   chan string

	interruptCh chan struct{}
}

// GetEngineService 创建引擎实例（轻量：只装配字段，不触盘）。生命周期统一由 Run 驱动。
func GetEngineService(name string) *Engine {
	return &Engine{
		Agentname:   name,
		inputCh:     make(chan string),
		interruptCh: make(chan struct{}),
		notice:      notice{Kind: NoticeNone},
		errBudget:   newErrorBudget(defaultErrorMaxTimes, defaultErrorSleepGap),
		bootState:   BootBooting,
	}
}

// Init 完成环境检查（bootstrap），把检查产物拷入引擎后由引擎装配运行时资产，
// 最后构造 runner。检查与装配的分工见 bootstrap.go / assemble.go。
// 首次启动（Check 创建了默认配置）不再驻留退出：引擎照常装配模板配置，
// 置 bootState=config，由 TUI 弹配置页收集模型配置后 ApplyFirstStartupConfig 收尾。
func (e *Engine) Init() {
	env, err := Check()
	if err != nil {
		e.parkWithFatal(FatalError, err.Error(), true)
		return
	}
	e.absorb(env)
	if err := e.flush(); err != nil {
		e.parkWithFatal(FatalError, err.Error(), true)
		return
	}
	if env.ConfigCreated {
		e.enterFirstStartup()
		return
	}
	e.setBootState(BootReady)
}

// flush 释放上一轮装配的运行时资产后重跑 ⑥–⑩ + newRunner。
// 幂等：不改变语义状态，重复调用只是把资产按当前配置再重建一遍，结果收敛。
// 首次启动配置提交后整条装配路径重跑：引擎无需知道哪些组件快照了 config
// （session service 的 summarizer 拷贝了 config.Model 值——整体重建一次性覆盖）。
// 仅可在无回合进行中时调用（首次启动的配置页是模态的，天然满足；
// 装配错误由调用方决定呈现方式）。
func (e *Engine) flush() error {
	e.releaseRuntime()
	if err := e.assembleConfig(); err != nil {
		return err
	}
	e.buildPrompts(e.Agentname)
	e.SessionService_p = e.buildSessionService()
	if err := e.buildMemoryService(); err != nil {
		return err
	}
	e.buildBuiltinAssets()
	e.newRunner()
	return nil
}

// releaseRuntime 关闭当前运行时资产（nil 安全：首次调用无事可放）。
// 顺序同 Shutdown：先停 taskService（取消并等待在跑 Worker），再放 runner；
// MCP/localexec 随后。session service 接口自带 Close（内存/SQLite 实现均满足）。
func (e *Engine) releaseRuntime() {
	if e.taskService != nil {
		e.taskService.Close()
		e.taskService = nil
	}
	e.taskTools = nil
	if e.AgentRunner_p != nil {
		(*e.AgentRunner_p).Runner.Close()
		e.AgentRunner_p = nil
	}
	for _, ts := range e.mcpToolsets {
		ts.Close()
	}
	e.mcpToolsets = nil
	if e.execToolSet != nil {
		e.execToolSet.Close() //localexec：kill 残留运行命令并清空注册表（首启 flush 时无作业）
		e.execToolSet = nil
	}
	if e.SessionService_p != nil {
		_ = e.SessionService_p.Close()
		e.SessionService_p = nil
	}
}

// ApplyFirstStartupConfig 接收 TUI 配置页提交的模型配置（yaml 字段名 → 值）：
// 校验 → 行级写回 HackerTeam.yaml → flush 整体重跑装配。仅 config 态可调用；
// 成功后引擎进入 ready，TUI 侧横幅与本页关闭由返回值驱动。
func (e *Engine) ApplyFirstStartupConfig(vals map[string]string) error {
	e.mu.Lock()
	state := e.bootState
	e.mu.Unlock()
	if state != BootConfigNeeded {
		return fmt.Errorf("engine is not waiting for first startup config")
	}
	if err := validateModelForm(vals); err != nil {
		return err
	}
	if err := config.PatchModelFields(e.HackerTeamConfigPath, normalizeModelForm(vals)); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := e.flush(); err != nil {
		// 写回成功但装配失败：引擎已不可用，置终态交给 TUI 任意键退出。
		// 不能 parkWithFatal：本方法在 TUI 事件循环 goroutine 上调用，驻留会冻死渲染。
		e.setFatal(FatalError, err.Error(), true)
		return err
	}
	e.firstStartupFields = nil
	e.setBootState(BootReady)
	e.setStartupInfo(e.startupInfoLines())
	return nil
}

// modelFormKeys 配置页可提交的字段白名单（yaml 字段名）。
var modelFormKeys = []string{"model", "baseurl", "apikey", "apitype"}

// validateModelForm 必填与枚举校验；白名单外的键忽略（前向兼容）。
func validateModelForm(vals map[string]string) error {
	for _, k := range modelFormKeys {
		if strings.TrimSpace(vals[k]) == "" {
			return fmt.Errorf("model config field %q is required", k)
		}
	}
	switch strings.ToLower(strings.TrimSpace(vals["apitype"])) {
	case "openai", "anthropic":
	default:
		return fmt.Errorf("apitype must be openai or anthropic, got %q", vals["apitype"])
	}
	return nil
}

// normalizeModelForm 裁剪空白并统一 apitype 小写。
func normalizeModelForm(vals map[string]string) map[string]string {
	out := make(map[string]string, len(vals))
	for _, k := range modelFormKeys {
		out[k] = strings.TrimSpace(vals[k])
	}
	out["apitype"] = strings.ToLower(out["apitype"])
	return out
}

// Run 启动引擎完整生命周期：先同步完成 Init，再启动主循环 AgentStart（goroutine 由其内部负责）。
// 仅当 Init 走到致命终态（parkWithFatal）时才驻留当前 goroutine，否则返回时引擎已后台运行。
func (e *Engine) Run() {
	e.Init()
	e.AgentStart()
}

// absorb 把 bootstrap 检查产物（路径、日志句柄）拷入引擎字段，
// 并上屏启动期收集的非致命提示。运行时对象的装配在 Init 后续步骤完成。
func (e *Engine) absorb(env *BootEnv) {
	e.CWD = env.CWD
	e.ExeDir = env.ExeDir
	e.ConfigFolderPath = env.ConfigFolderPath
	e.HackerTeamConfigPath = env.HackerTeamConfigPath
	e.FrameworkLogFile_p = env.LogFile
	e.SkillsFolderPath = env.SkillsFolderPath
	for _, n := range env.Notices {
		e.setNotice(n.Kind, n.Text)
	}
}

// AgentStart 启动引擎主循环（goroutine 内读用户输入、分类分发，直到退出），
// 调用方无需自行起 goroutine。错误预算的状态流转见 errorBudget。
func (e *Engine) AgentStart() {
	go e.agentLoop()
}

// agentLoop 主循环本体：读对话输入、逐轮执行，直到退出。
func (e *Engine) agentLoop() {
	e.randomStartID()
	// 启动横幅只在此组装一次（bootstrap/newRunner 均已完成）。
	// 首次启动例外：此时还是模板配置，等 ApplyFirstStartupConfig 提交后
	// 以真实值组装（本函数此刻可能还没跑到，gate 保证不抢在配置页前面）。
	if e.bootStateSnapshot() == BootReady {
		(*e).setStartupInfo((*e).startupInfoLines())
	}
	for {
		line := <-(*e).inputCh
		(*e).errBudget.recharge() //任何用户输入都充值（空输入也是，无害）

		if strings.TrimSpace(line) == "" { //空输入，重新等待（前端已兜底，双保险）
			continue
		}
		//斜杠命令不经过这里：语义与注册表在前端（service/tui/commands.go），后端只暴露能力接口
		(*e).appendTyped(RecUser, line)
		e.turn(line)
	}
}

// ── 斜杠命令的后端能力接口（前端注册表见 service/tui/commands.go）──────

// Shutdown /exit 的引擎侧收尾：先停后台任务，再释放 runner、MCP 连接与 localexec 作业表。
// 置终态与退出由前端接管（tea.Quit）。
//
// 顺序有意义：taskService.Close 会取消并等待在跑的 Worker 退出，之后才能安全关闭
// 它们正在使用的 MCP 工具集与 localexec 作业表。
//
// 刻意不加 isRunning 守卫：长任务运行中 /exit 是常态，加了守卫会整段跳过清理，
// MCP stdio 子进程与 localexec 的 PTY 作业就会活过进程。localexec.Close() 非阻塞
// （拷作业 ID → Kill → 清注册表），不会挂住退出；关闭运行中的 runner 是紧随
// tea.Quit 的竞态，与上游同型、刻意接受。
func (e *Engine) Shutdown() {
	if (*e).taskService != nil {
		(*e).taskService.Close()
	}
	if (*e).AgentRunner_p != nil {
		(*(*e).AgentRunner_p).Runner.Close()
	}
	for _, ts := range (*e).mcpToolsets {
		ts.Close() //关闭MCP连接，释放stdio子进程
	}
	if (*e).execToolSet != nil {
		(*e).execToolSet.Close() //localexec：kill残留运行命令并清空注册表
	}
}

// NewConversation /new：重置 SessionId 与 RequestId 开新对话。running 时拒绝：
// SessionId 读写无锁，turn 进行中重置是 data race，这里显式兜底。
func (e *Engine) NewConversation() bool {
	if e.isRunning() {
		e.setNotice(NoticeWarning, "agent is running, try /new later")
		return false
	}
	(*e).appendTyped(RecSlash, "/new")
	e.randomStartID()
	(*e).setNotice(NoticeNewConversation, "")
	return true
}

// ClearRecords /clear：丢弃全部消息记录（连命令本身也不留痕）。running 拒绝同 NewConversation。
func (e *Engine) ClearRecords() bool {
	if e.isRunning() {
		e.setNotice(NoticeWarning, "agent is running, try /clear later")
		return false
	}
	e.clearRecords()
	e.setNotice(NoticeClear, "")
	return true
}

func (e *Engine) randomStartID() {
	(*e).SessionId = uuid.New().String()
	(*e).RequestId = uuid.New().String()
}

// startupInfoLines 拼出启动横幅的信息行（label 补齐到 13 列），宽度截断交给 TUI。
// 调用时机在 bootstrap 完成后的第一轮，此时 newRunner/randomStartID 均已完成。
func (e *Engine) startupInfoLines() []string {
	cfg := (*e).Config_p.Model
	cwd := (*e).CWD
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	sid := (*e).SessionId
	if len(sid) > 8 {
		sid = sid[:8]
	}
	// 统计 Skills 目录下的 skill 数量：一个 skill = 一个含 SKILL.md 的子目录。
	// 只数子目录会把 pentest-tools（仅 .template 占位）也算进去，那是虚数。
	skills := 0
	if entries, err := os.ReadDir((*e).SkillsFolderPath); err == nil {
		for _, en := range entries {
			if !en.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join((*e).SkillsFolderPath, en.Name(), "SKILL.md")); err == nil {
				skills++
			}
		}
	}
	host, err := url.Parse(cfg.BaseURL)
	if err != nil {
		host = &url.URL{Host: ""}
	}
	toolCount := len((*e).builtinTools)
	if (*e).execToolSet != nil {
		toolCount += len((*e).execToolSet.Tools(context.Background()))
	}
	return []string{
		fmt.Sprintf("model        %s %d", cfg.Model, cfg.ContextWindow),
		fmt.Sprintf("endpoint     %s", host.Host),
		fmt.Sprintf("cwd          %s", cwd),
		fmt.Sprintf("tools        %d · skills %d · mcp %d",
			toolCount,
			skills,
			len((*e).mcpToolsets)),
		fmt.Sprintf("session      %s", sid),
	}
}
