package engine

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"HackerTeam/service/engine/config"
	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/tool"
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
	CWD                  string          //当前工作目录
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

	SessionId string
	RequestId string

	builtinTools    []tool.Tool             // 内置function工具清单（文件系统/文件操作/日期），启动时确定，跨轮常驻不刷新
	builtinToolsets map[string]tool.ToolSet // 内置工具集（每个执行角色一个常驻localexec实例），启动时确定，跨轮常驻，jobs可跨轮续查
	mcpToolsets     []tool.ToolSet          // 配置文件声明的MCP工具集，每轮run自动Close重建，共享挂载给全部agent

	ReconSkillsFolderPath       string
	ExploitSkillsFolderPath     string
	PostExploitSkillsFolderPath string
	ScannerSkillsFolderPath     string
	ReproducerSkillsFolderPath  string

	// errBudget 连续错误自动重试预算：策略（max/gap）与状态（streak）同体，
	// 状态流转见 errorBudget.go 的 errorBudget 类型。
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
	skills    []SkillItem
	inputCh   chan string

	interruptCh chan struct{}
}

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
// 检查器只返回错误与产物；致命呈现（parkWithFatal）是 Engine 的职责。
func (e *Engine) Init() {
	env, err := Check(e.Agentname, e)
	if err != nil {
		e.parkWithFatal(FatalError, err.Error(), true)
		return
	}
	if env.NeedRestart {
		// 首跑创建了默认配置文件：须改完配置重启，本次启动到此为止。
		e.parkWithFatal(FatalSuccess, "检查到配置文件不存在，已创建默认配置文件。请根据实际情况修改配置文件后重新启动程序！", true)
		return
	}
	e.absorb(env)
	e.newRunner()
}

// absorb 把 bootstrap 产物拷入引擎字段，并上屏启动期收集的非致命提示。
func (e *Engine) absorb(env *BootEnv) {
	e.CWD = env.CWD
	e.ConfigFolderPath = env.ConfigFolderPath
	e.HackerTeamConfigPath = env.HackerTeamConfigPath
	e.FrameworkLogFile_p = env.LogFile
	e.Config_p = env.Config
	e.EnvPrompt = env.EnvPrompt
	e.CommandExecutionPrompt = env.CommandExecutionPrompt
	e.VulnConsensusPrompt = env.VulnConsensusPrompt
	e.OutputConsensusPrompt = env.OutputConsensusPrompt
	e.SessionService_p = env.SessionService
	e.SqliteMemoryService = env.SqliteMemoryService
	e.builtinTools = env.BuiltinTools
	e.builtinToolsets = env.BuiltinToolsets
	for role, folder := range env.RoleSkillFolders {
		switch role {
		case reconSkillsFolder:
			e.ReconSkillsFolderPath = folder
		case exploitSkillsFolder:
			e.ExploitSkillsFolderPath = folder
		case postExploitSkillsFolder:
			e.PostExploitSkillsFolderPath = folder
		case scannerSkillsFolder:
			e.ScannerSkillsFolderPath = folder
		case reproducerSkillsFolder:
			e.ReproducerSkillsFolderPath = folder
		}
	}
	for _, n := range env.Notices {
		e.setNotice(n.Kind, n.Text)
	}
}

// AgentStart 引擎主循环：读用户输入、分类分发，直到退出。
// 一轮对话在 turn 里跑（含自动重试）；错误预算的状态流转见 errorBudget。
func (e *Engine) AgentStart() {
	e.randomStartID()
	// 启动横幅只在此组装一次（bootstrap/newRunner 均已完成）。
	(*e).setStartupInfo((*e).startupInfoLines())
	for {
		cmd := parseInput(<-(*e).inputCh)
		(*e).errBudget.recharge() //任何用户输入都充值（斜杠/空输入也是，无害）

		switch cmd.Kind {
		case cmdExit: //用户主动结束对话：释放资源，置终态并永久驻留
			(*e).appendTyped("slash", cmd.Prompt)
			//关闭AgentRunner，释放资源
			(*(*e).AgentRunner_p).Runner.Close()
			for _, ts := range (*e).mcpToolsets {
				ts.Close() //关闭MCP连接，释放stdio子进程
			}
			for _, ts := range (*e).builtinToolsets {
				ts.Close() //localexec：kill残留运行命令并清空注册表
			}
			(*e).parkWithFatal(FatalExit, "", false)

		case cmdNew: //用户开始新对话：重置SessionId, RequestId
			(*e).appendTyped("slash", cmd.Prompt)
			e.randomStartID()
			(*e).setNotice(NoticeNewConversation, "")

		case cmdPrompt: //普通对话输入
			(*e).appendTyped("user", cmd.Prompt)
			e.turn(cmd.Prompt)

		case cmdEmpty: //空输入，重新等待
		}
	}
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
	// 统计 5 个角色 skills 文件夹下的 skill 数量（每个 skill 是一个子目录）
	skills := 0
	for _, dir := range []string{(*e).ReconSkillsFolderPath, (*e).ExploitSkillsFolderPath,
		(*e).PostExploitSkillsFolderPath, (*e).ScannerSkillsFolderPath, (*e).ReproducerSkillsFolderPath} {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, en := range entries {
				if en.IsDir() {
					skills++
				}
			}
		}
	}
	host, err := url.Parse(cfg.BaseURL)
	if err != nil {
		host = &url.URL{Host: ""}
	}
	return []string{
		fmt.Sprintf("model        %s %d", cfg.Model, cfg.ContextWindow),
		fmt.Sprintf("endpoint     %s", host.Host),
		fmt.Sprintf("cwd          %s", cwd),
		fmt.Sprintf("tools        %d · skills %d · mcp %d",
			len((*e).builtinTools)+len((*e).builtinToolsets),
			skills,
			len((*e).mcpToolsets)),
		fmt.Sprintf("session      %s", sid),
	}
}
