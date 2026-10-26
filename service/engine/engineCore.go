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
	}
}

// Init 完成环境检查与资产装配（bootstrap），随后构造 runner。
func (e *Engine) Init() {
	env, err := Check(e.Agentname, e)
	if err != nil {
		e.parkWithFatal(FatalError, err.Error(), true)
		return
	}
	if env.NeedRestart {
		// 首跑创建了默认配置文件：须改完配置重启，本次启动到此为止。
		e.parkWithFatal(FatalSuccess, "Config file not found; a default one has been created. Please review it and restart.", true)
		return
	}
	e.absorb(env)
	e.newRunner()
}

// Run 启动引擎完整生命周期：先同步完成 Init，再启动主循环 AgentStart（goroutine 由其内部负责）。
// 仅当 Init 走到致命终态（parkWithFatal）时才驻留当前 goroutine，否则返回时引擎已后台运行。
func (e *Engine) Run() {
	e.Init()
	e.AgentStart()
}

// absorb 把 bootstrap 产物拷入引擎字段，并上屏启动期收集的非致命提示。
func (e *Engine) absorb(env *BootEnv) {
	e.CWD = env.CWD
	e.ExeDir = env.ExeDir
	e.ConfigFolderPath = env.ConfigFolderPath
	e.HackerTeamConfigPath = env.HackerTeamConfigPath
	e.FrameworkLogFile_p = env.LogFile
	e.Config_p = env.Config
	e.EnvPrompt = env.EnvPrompt
	e.CommandExecutionPrompt = env.CommandExecutionPrompt
	e.VulnConsensusPrompt = env.VulnConsensusPrompt
	e.OutputConsensusPrompt = env.OutputConsensusPrompt
	e.OutputDir = env.OutputDir
	e.SessionService_p = env.SessionService
	e.SqliteMemoryService = env.SqliteMemoryService
	e.builtinTools = env.BuiltinTools
	e.execToolSet = env.ExecToolSet
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
	(*e).setStartupInfo((*e).startupInfoLines())
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
