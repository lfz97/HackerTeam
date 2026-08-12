package engine

import (
	"context"
	"embed"
	"os"

	"HackerTeam/service/engine/config"
	"HackerTeam/utils/pretty"
	"github.com/google/uuid"

	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type Agentrunner struct {
	Runner runner.Runner
	Stream bool
}

// Engine 封装核心状态变量（原 global 包级变量）
type Engine struct {
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
	MsgContext := turnResult{
		Code:       New,
		Reason:     "新对话",
		OutputPart: "",
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
			e.randomStartID()
			MsgContext = turnResult{
				Code:       New,
				Reason:     "新对话",
				OutputPart: "",
			}

		} else { //其他情况，继续使用当前的SessionId, RequestId，更新MsgContext为当前对话的结束状态，供下一轮对话使用
			MsgContext = *EndTurn_p
			continue
		}

	}
}

func (e *Engine) randomStartID() {
	(*e).SessionId = uuid.New().String()
	(*e).RequestId = uuid.New().String()
}
