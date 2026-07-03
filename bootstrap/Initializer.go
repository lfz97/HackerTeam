package bootstrap

import (
	"HackerTeam/config"
	"HackerTeam/global"
	"HackerTeam/memory"
	"HackerTeam/session"
	"HackerTeam/utils/pretty"
	"fmt"
	"github.com/google/uuid"
	"github.com/otiai10/copy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	stdlog "log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/log"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/team"
	"trpc.group/trpc-go/trpc-mcp-go"
)

// 定义配置文件夹中的各种配置文件名称
const (
	hackerTeamConfigFolder string = ".HackerTeam"
	hackerTeamConfig       string = "HackerTeam.yaml"
	hackerTeamLogFile      string = "HackerTeam.log"
	memoryDBFileName       string = "memory.db"
	operationRecord        string = "OperationRecord.md"
	outputDir              string = "output"
)

// 技能目录名称
const (
	reconSkillsFolder       string = "ReconSkills"
	exploitSkillsFolder     string = "ExploitSkills"
	postExploitSkillsFolder string = "PostExploitSkills"
	scannerSkillsFolder     string = "ScannerSkills"
	reproducerSkillsFolder  string = "ReproducerSkills"
)

func Init(an string) {
	global.Agentname = an

	//获取Agent可执行文件所在的目录路径
	getcwd()

	//检查配置文件夹
	checkConfigFolder()

	//检查配置文件是否存在，不存在则创建一个默认的配置文件
	checkConfig()

	//检查skills文件夹是否存在
	checkSkillsFolder()

	// 将框架日志重定向到文件，避免输出到终端干扰 TUI显示
	redirectFrameworkLog()

	//设置系统提示词
	configENVPrompt()

	//加载配置文件
	LoadConfig()

	//初始化内存会话服务
	initMemorySessionService()

	//初始化sqlite记忆服务
	initSqliteMemoryService()

	//初始化AgentRunner
	NewRunner()
}

// 配置系统提示词，替换其中的占位符
func configENVPrompt() {
	envPrompt_b, _ := global.PromptFiles.ReadFile("prompts/common/env.md")
	global.EnvPrompt = string(envPrompt_b)
	//Agent名称
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{NAME}}", global.Agentname)

	//当前日期
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{DATE}}", time.Now().Format("2006-01-02 15:04:05 (Mon)"))

	//当前时区
	zone, _ := time.Now().Zone()
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{TIMEZONE}}", fmt.Sprintf("%s (%s)", time.Now().Location().String(), zone))

	//操作系统
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{OSTYPE}}", runtime.GOOS)

	//CPU架构
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{AARCH}}", runtime.GOARCH)

	//主目录
	homeDir, _ := os.UserHomeDir()
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{HOME}}", homeDir)

	//临时目录
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{TMPDIR}}", os.TempDir())

	//当前用户
	u, _ := user.Current()
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{CURRENTUSER}}", u.Username)

	//主机名
	hostName, _ := os.Hostname()
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{HOSTNAME}}", hostName)

	//运行目录
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{CWD}}", global.CWD)

	//配置目录
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{CONFIGPATH}}", global.ConfigFolderPath)

	//配置文件
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{HackerTeamConfig}}", hackerTeamConfig)
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{HackerTeamLogFile}}", hackerTeamLogFile)
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{OperationRecord}}", operationRecord)

	//输出目录
	time := time.Now().Format("20060102150405")
	outputDir := filepath.Join(global.CWD, outputDir, time)
	global.EnvPrompt = strings.ReplaceAll(global.EnvPrompt, "{{OUTPUTDIR}}", outputDir)

	// 读取共享的 Command Execution 提示词片段（sub-agent 共用）
	cmdExecBytes, _ := global.PromptFiles.ReadFile("prompts/common/command_execution.md")
	global.CommandExecutionPrompt = string(cmdExecBytes)

	// 读取共享的 Vuln Consensus 提示词片段（漏洞定义与定级共识）
	vulnConsensusBytes, _ := global.PromptFiles.ReadFile("prompts/common/vuln_consensus.md")
	global.VulnConsensusPrompt = string(vulnConsensusBytes)

	// 读取共享的 Output Consensus 提示词片段（结果输出规范）
	toolConsensusBytes, _ := global.PromptFiles.ReadFile("prompts/common/output_consensus.md")
	global.OutputConsensusPrompt = string(toolConsensusBytes)
}

// 获取当前可执行文件所在的目录完整路径
func getcwd() {

	exePath, err := os.Executable() // 获取当前可执行文件的路径
	if err != nil {
		global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("获取可执行文件目录错误: %v,按任意键退出", err))
	}
	global.CWD = filepath.Dir(exePath) // 获取当前可执行文件的目录路径（不包含程序名）

}

// 检查配置文件夹是否存在
func checkConfigFolder() {
	global.ConfigFolderPath = filepath.Join(global.CWD, hackerTeamConfigFolder)
	_, err := os.Stat(global.ConfigFolderPath)
	if err != nil {
		if os.IsNotExist(err) {
			//config 文件夹不存在，创建一个默认的 config 文件夹
			err := os.MkdirAll(global.ConfigFolderPath, os.ModePerm)
			if err != nil {
				global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("创建默认config文件夹错误：%v", err))
			}
			global.ShowSuccess(global.AgentMessage, "检查到config文件夹不存在，已创建默认config文件夹")
		} else {
			global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("检查config文件夹错误：%v", err))
		}
	} else {
		global.ShowSuccess(global.AgentMessage, "检查配置文件夹通过")
	}

}

// 检查配置文件是否存在，不存在则创建一个默认的配置文件
func checkConfig() {
	global.HackerTeamConfigPath = filepath.Join(global.ConfigFolderPath, hackerTeamConfig)
	// TODO: 读取并解析 configPath 中的 YAML 配置
	_, err := os.Stat(global.HackerTeamConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，创建一个默认的 config.yaml
			fd, err := os.OpenFile(global.HackerTeamConfigPath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("创建默认配置文件错误：%v", err))
			}
			defer fd.Close()
			//生成一个随机的用户ID，替换掉配置文件中的占位符
			cfg := strings.ReplaceAll(config.Template, "{USERID}", uuid.New().String())
			_, err = fd.WriteString(cfg)
			if err != nil {
				global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("写入默认配置文件错误：%v,按任意键退出", err))
			}
			global.ShowSuccessAndExit(global.AgentMessage, "检查到配置文件不存在，已创建默认配置文件。请根据实际情况修改配置文件后重新启动程序！")
		} else {
			global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("检查配置文件错误：%v", err))
		}
	} else {
		global.ShowSuccess(global.AgentMessage, "检查配置文件通过!")
	}

}

func checkSkillsFolder() {

	global.ReconSkillsFolderPath = filepath.Join(global.ConfigFolderPath, reconSkillsFolder)
	global.ExploitSkillsFolderPath = filepath.Join(global.ConfigFolderPath, exploitSkillsFolder)
	global.PostExploitSkillsFolderPath = filepath.Join(global.ConfigFolderPath, postExploitSkillsFolder)
	global.ScannerSkillsFolderPath = filepath.Join(global.ConfigFolderPath, scannerSkillsFolder)
	global.ReproducerSkillsFolderPath = filepath.Join(global.ConfigFolderPath, reproducerSkillsFolder)

	func(skillsFolders []string) {
		for _, folder := range skillsFolders {
			_, err := os.Stat(folder)
			if err != nil {
				if os.IsNotExist(err) {
					//skills 文件夹不存在，创建一个默认的 skills 文件夹
					err := os.MkdirAll(folder, os.ModePerm)
					if err != nil {
						global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("创建默认%s文件夹错误：%s", folder, err.Error()))
					}
					err = copy.Copy("skillsTemplates/pentest-tools", filepath.Join(folder, "pentest-tools"), copy.Options{FS: global.ToolSkills})
					if err != nil {
						global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("复制技能模板到%s文件夹错误：%s", folder, err.Error()))
					}
					global.ShowSuccess(global.AgentMessage, fmt.Sprintf("检查到%s文件夹不存在，已创建", folder))
				} else {
					global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("检查%s文件夹错误：%s", folder, err.Error()))
				}
			} else {
				global.ShowSuccess(global.AgentMessage, fmt.Sprintf("检查%s文件夹通过", folder))
			}
		}
	}([]string{global.ReconSkillsFolderPath, global.ExploitSkillsFolderPath, global.PostExploitSkillsFolderPath, global.ScannerSkillsFolderPath, global.ReproducerSkillsFolderPath})

}

func loadConfig() (*config.Config, error) {
	YamlConfig := config.Config{}
	yamlFile, err := os.ReadFile(global.HackerTeamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件错误：%v", err)
	}
	err = yaml.Unmarshal(yamlFile, &YamlConfig)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件错误：%v", err)
	}
	return &YamlConfig, nil
}

func initMemorySessionService() {
	global.SessionService = session.NewMemorySessionService((*global.Config_p).Model)
}

func initSqliteMemoryService() {
	service, err := memory.NewSQLiteMemoryService((*global.Config_p).Model, filepath.Join(global.ConfigFolderPath, memoryDBFileName))
	if err != nil {
		global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("初始化sqlite记忆服务错误: %v", err))
	}
	global.SqliteMemoryService = service
}

func initTeam() runner.Runner {
	CaptainAgent := initCaptain()
	exploitAgent := initexploit()
	postexploitAgent := initpostexploit()
	reconAgent := initRecon()
	scannerAgent := initScanner()
	reproducerAgent := initReproducer()

	team.New(
		CaptainAgent,
		[]agent.Agent{exploitAgent, postexploitAgent, reconAgent, scannerAgent, reproducerAgent},
		team.WithDescription("A hacker team with one captain and three members, responsible for penetration testing tasks."),
		team.WithMemberToolStreamInner(true),                        //子agent的内部事件透传到父流程(TUI)
		team.WithMemberToolInnerTextMode(team.InnerTextModeInclude), //展示子agent完整transcript(正文+tool call+tool result)
	)
	Runner := runner.NewRunner(global.Agentname, CaptainAgent,
		runner.WithSessionService(global.SessionService),     // 使用内存会话服务，其中包含自动摘要功能
		runner.WithMemoryService(global.SqliteMemoryService), // 使用sqlite记忆服务
	)
	return Runner
}

func LoadConfig() {
	//加载配置文件
	config_p, err := loadConfig()
	if err != nil {
		global.ShowErrorAndExit(global.AgentMessage, pretty.TErrorF("加载配置文件错误: %v,按任意键退出", err))
	}
	global.Config_p = config_p
}

// 解析加载完成的配置文件，内部创建Team agent，并生成一个runner
func NewRunner() {
	runner := initTeam()
	global.AgentRunner_p = &global.Agentrunner{
		Runner: runner,
		Stream: (*global.Config_p).Model.Stream,
	}
	global.PrintToTui(global.AgentMessage, pretty.TReady(global.Agentname), true)
}

// redirectFrameworkLog 将框架的日志输出从 stdout 重定向到可执行文件同目录下的 HackerTeam.log 文件-created by copilot
func redirectFrameworkLog() {
	logPath := filepath.Join(global.ConfigFolderPath, hackerTeamLogFile)
	var err error
	global.FrameworkLogFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "lvl",
		NameKey:        "name",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.AddSync(global.FrameworkLogFile),
		zapcore.DebugLevel,
	)
	fileLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	//定向trpc-agent-go的日志输出到文件
	log.Default = fileLogger
	log.ContextDefault = fileLogger

	//定向trpc-mcp-go的日志输出到文件
	mcp.SetDefaultLogger(fileLogger)

	//重定向标准库 log 到文件（避免 gse 等第三方库的日志污染终端）
	if global.FrameworkLogFile != nil {
		stdlog.SetOutput(global.FrameworkLogFile)
	}
}
