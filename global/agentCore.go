package global

import (
	"HackerTeam/config"
	"embed"
	"os"

	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

type Agentrunner struct {
	Runner runner.Runner
	Stream bool
}

// 定义核心的状态变量
var (
	Config_p             *config.Config           //yaml配置
	Agentname            string                   //Agent名称
	CWD                  string                   //当前工作目录
	ConfigFolderPath     string                   //配置文件夹路径
	HackerTeamConfigPath string                   //配置文件路径
	AgentRunner_p        *Agentrunner             //Runner，全局唯一
	SessionService       *inmemory.SessionService //会话服务，包含自动摘要功能
	SqliteMemoryService  *memorysqlite.Service    // sqlite记忆服务
	FrameworkLogFile     *os.File                 // 保存日志文件句柄，防止被 GC 回收

	//go:embed prompts/*
	PromptFiles embed.FS //提示词嵌入FS

	//go:embed skillsTemplates/*
	ToolSkills embed.FS

	EnvPrompt              string
	CommandExecutionPrompt string
	VulnConsensusPrompt    string
	OutputConsensusPrompt  string

	SessionId string
	RequestId string
)

// 技能目录相关配置
func AgentEngineRun(initFn, startFn func()) {
	go func() {
		initFn()
		startFn()
	}()
}

var (
	ReconSkillsFolderPath       string
	ExploitSkillsFolderPath     string
	PostExploitSkillsFolderPath string
	ScannerSkillsFolderPath     string
	ReproducerSkillsFolderPath  string
)
