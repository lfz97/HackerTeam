package engine

import (
	"context"
	"embed"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"HackerTeam/service/engine/config"
	"HackerTeam/utils/pretty"
	"github.com/google/uuid"

	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// 连续错误自动重试策略。达到上限后不再自动重试，把控制权交还用户。
// 用常量而非 yaml 配置：目前没有按实例调整的需求，将来要配再提升为 Engine 字段。
const (
	errorMaxTimes = 3
	errorSleepGap = 3 * time.Second
)

type Agentrunner struct {
	Runner runner.Runner
	Stream bool
}

// Engine 封装核心状态变量（原 global 包级变量）
type Engine struct {
	// errorStreak 当前连续错误次数，配合 errorMaxTimes / errorSleepGap
	// 实现自动重试的上限与退避。归零时机有四个，缺一不可：一轮成功(Continue)、/new、
	// 用户 ESC 中断(Int)、用户在 agentRunIteratively 提交一条非空输入。
	// 只有"自动重试链"内部不归零——那正是要计数的时候。
	errorStreak int

	tui tuiService

	Config_p             *config.Config           //yaml配置
	Agentname            string                   //Agent名称
	CWD                  string                   //当前工作目录
	ConfigFolderPath     string                   //配置文件夹路径
	HackerTeamConfigPath string                   //配置文件路径
	AgentRunner_p        *Agentrunner             //Runner，全局唯一
	SessionService_p     *inmemory.SessionService //会话服务，包含自动摘要功能
	SqliteMemoryService  *memorysqlite.Service    // sqlite记忆服务
	FrameworkLogFile_p   *os.File                 // 保存日志文件句柄，防止被 GC 回收

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
}

//go:embed prompts/*
var PromptFiles embed.FS //提示词嵌入FS

//go:embed skillsTemplates/*
var ToolSkills embed.FS //技能模板嵌入FS

func GetEngineService(name string, tui tuiService) *Engine {
	e := &Engine{
		tui: tui,
	}
	(*e).Agentname = name
	(*e).preCheckLoad()
	(*e).newRunner()
	return e
}

func (e *Engine) AgentStart() {
	// 初始用 Startup 而不是 New：程序刚启动时并不存在"上一轮对话"，
	// 推一条"新对话已开始"到 NoticeBar 是噪音。New 只留给用户真的敲 /new 的场合。
	MsgContext := turnInfo{
		Code:          Startup,
		Reason:        "程序启动",
		PartialOutput: "",
	}
	e.randomStartID()
	for {
		EndTurn_p := e.agentRunIteratively(context.Background(), MsgContext)
		if (*EndTurn_p).Code == Exit { //用户主动结束对话，退出程序
			//关闭AgentRunner，释放资源
			(*(*e).AgentRunner_p).Runner.Close()
			for _, ts := range (*e).mcpToolsets {
				ts.Close() //关闭MCP连接，释放stdio子进程
			}
			for _, ts := range (*e).builtinToolsets {
				ts.Close() //localexec：kill残留运行命令并清空注册表
			}
			(*e).tui.ShowMsgAndExitNoTrigger(pretty.TExit("对话已结束，感谢使用！后会有期！"))

		} else if (*EndTurn_p).Code == New { //用户开始新对话，重置SessionId, RequestId，更新MsgContext为新对话的初始状态
			// /new 在 agentRunIteratively 的输入分支里是提前 return 的，走不到"用户提交
			// 非空输入"那处归零，所以必须在这里单独归
			(*e).errorStreak = 0
			e.randomStartID()
			MsgContext = turnInfo{
				Code:          New,
				Reason:        "新对话",
				PartialOutput: "",
			}

		} else if (*EndTurn_p).Code == Error { //出错：累加连续错误计数，未达上限则退避后自动重试
			(*e).errorStreak++
			if (*e).errorStreak >= errorMaxTimes {
				// 放弃自动重试，把控制权交还用户。三个要点：
				// ① Code 保持 Error —— Int 的语义是"用户按了 ESC 中断"，与事实不符，
				//    不能为了蹭"回到输入循环"这个副作用而填一个假状态码。真正让下一轮
				//    等用户输入的是 agentRunIteratively 里的 errorStreak < errorMaxTimes 判定。
				// ② errorStreak 不归零 —— 归零会让下一轮重新满足自动重试条件，无限循环照旧。
				// ③ 整个复用 *EndTurn_p，不新造 literal —— 新建会静默丢掉 Reason 与 PartialOutput。
				(*e).tui.PrintToMsgView(pretty.TErrorF("连续 %d 次失败，已停止自动重试。请检查网络/配置后重新输入。", errorMaxTimes), false)
				MsgContext = *EndTurn_p
				continue
			}
			// 必须在 Sleep 之前打：sleep 期间引擎 goroutine 阻塞、不监听输入通道，
			// 用户打字没有反应，需要知道程序在等什么。
			(*e).tui.PrintToMsgView(pretty.TWarningF("%d 秒后重试（第 %d/%d 次）...", errorSleepGap/time.Second, (*e).errorStreak, errorMaxTimes), false)
			time.Sleep(errorSleepGap)
			MsgContext = *EndTurn_p

		} else { //其他情况（Continue 正常结束 / Int 用户中断），错误链断开、计数归零
			// 归零覆盖两个时机：Continue（自动重试链里第 2 次尝试成功时收口）与 Int（人已介入）
			(*e).errorStreak = 0
			MsgContext = *EndTurn_p
			continue
		}

	}
}

func (e *Engine) randomStartID() {
	(*e).SessionId = uuid.New().String()
	(*e).RequestId = uuid.New().String()
}

// startupInfoLines 拼出启动横幅的信息行（label 补齐到 13 列），宽度截断交给 TUI。
// 调用时机在 AgentStart 第一轮，此时 preCheckLoad/newRunner/randomStartID 均已完成。
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
