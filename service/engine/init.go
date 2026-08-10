package engine

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/memory"
	"HackerTeam/service/engine/session"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/otiai10/copy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	stdlog "log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ag "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/log"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/team"
	"trpc.group/trpc-go/trpc-mcp-go"

	"charm.land/glamour/v2"
)

// tuiService 定义 Engine 层对 TUI 的最小依赖接口
type tuiService interface {
	AddHelpItems(items []map[string]string)
	ClearAppFuncTrigger()
	PrintToMsgView(content string, clear bool)
	ReadInputAreaPromptWithEnter()
	InputChannel() chan string
	ResetHelpItems()
	SetAppFuncTriggerWithEsc(f func())
	ShowErrorInMsgViewAndExit(errmsg string)
	ShowMsgAndExitNoTrigger(msg string)
	ShowSuccessInMsgView(sussessmsg string)
	ShowSuccessInMsgViewAndExit(sussessmsg string)
	StatusBarScrollingTip(ctx context.Context, tip string, TColor string)
	StatusBarUserTip(s string)
	NewGlamourRenderer() *glamour.TermRenderer
}

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

func (e *Engine) preCheckLoad() {

	//获取Agent可执行文件所在的目录路径
	e.getcwd()

	//检查配置文件夹
	e.checkConfigFolder()

	//检查配置文件是否存在，不存在则创建一个默认的配置文件
	e.checkConfig()

	//检查skills文件夹是否存在
	e.checkSkillsFolder()

	// 将框架日志重定向到文件，避免输出到终端干扰 TUI显示
	e.redirectFrameworkLog()

	//设置环境提示词和共享提示词片段
	e.configENVPrompt()

	//加载配置文件
	e.LoadConfig()

	//初始化内存会话服务
	e.initInMemorySessionService()

	//初始化sqlite记忆服务
	e.initSqliteMemoryService()
}

// 每次runner执行时重新加载以下Module
func (e *Engine) reload() {
	e.LoadConfig() //加载配置文件（后续agent工厂每次重建工具和技能）
}

// 配置环境提示词，替换其中的占位符
func (e *Engine) configENVPrompt() {
	envPrompt_b, _ := PromptFiles.ReadFile("prompts/common/env.md")
	(*e).EnvPrompt = string(envPrompt_b)
	//Agent名称
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{NAME}}", (*e).Agentname)

	//当前日期
	//(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{DATE}}", time.Now().Format("2006-01-02 15:04:05 (Mon)"))

	//当前时区
	zone, _ := time.Now().Zone()
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{TIMEZONE}}", fmt.Sprintf("%s (%s)", time.Now().Location().String(), zone))

	//操作系统
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{OSTYPE}}", runtime.GOOS)

	//CPU架构
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{AARCH}}", runtime.GOARCH)

	//主目录
	homeDir, _ := os.UserHomeDir()
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{HOME}}", homeDir)

	//临时目录
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{TMPDIR}}", os.TempDir())

	//当前用户
	u, _ := user.Current()
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{CURRENTUSER}}", u.Username)

	//主机名
	hostName, _ := os.Hostname()
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{HOSTNAME}}", hostName)

	//运行目录
	//(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{CWD}}", (*e).CWD)

	//配置目录
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{CONFIGPATH}}", (*e).ConfigFolderPath)

	//配置文件
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{HackerTeamConfig}}", hackerTeamConfig)
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{HackerTeamLogFile}}", hackerTeamLogFile)
	//(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{OperationRecord}}", operationRecord)

	//输出目录
	now := time.Now().Format("20060102150405")
	outDir := filepath.Join((*e).CWD, outputDir, now)
	(*e).EnvPrompt = strings.ReplaceAll((*e).EnvPrompt, "{{OUTPUTDIR}}", outDir)

	// 读取共享的 Command Execution 提示词片段（sub-agent 共用）
	cmdExecBytes, _ := PromptFiles.ReadFile("prompts/common/command_execution.md")
	(*e).CommandExecutionPrompt = string(cmdExecBytes)

	// 读取共享的 Vuln Consensus 提示词片段（漏洞定义与定级共识）
	vulnConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/vuln_consensus.md")
	(*e).VulnConsensusPrompt = string(vulnConsensusBytes)

	// 读取共享的 Output Consensus 提示词片段（结果输出规范）
	toolConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/output_consensus.md")
	(*e).OutputConsensusPrompt = string(toolConsensusBytes)
}

// 获取当前可执行文件所在的目录完整路径
func (e *Engine) getcwd() {

	exePath, err := os.Executable() // 获取当前可执行文件的路径
	if err != nil {
		(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("获取可执行文件目录错误: %v,按任意键退出", err))
	}
	(*e).CWD = filepath.Dir(exePath) // 获取当前可执行文件的目录路径（不包含程序名）

}

// 检查配置文件夹是否存在
func (e *Engine) checkConfigFolder() {
	(*e).ConfigFolderPath = filepath.Join((*e).CWD, hackerTeamConfigFolder)
	_, err := os.Stat((*e).ConfigFolderPath)
	if err != nil {
		if os.IsNotExist(err) {
			//config 文件夹不存在，创建一个默认的 config 文件夹
			err := os.MkdirAll((*e).ConfigFolderPath, os.ModePerm)
			if err != nil {
				(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("创建默认config文件夹错误：%v", err))
			}
			(*e).tui.ShowSuccessInMsgView("检查到config文件夹不存在，已创建默认config文件夹")
		} else {
			(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("检查config文件夹错误：%v", err))
		}
	}

}

// 检查配置文件是否存在，不存在则创建一个默认的配置文件
func (e *Engine) checkConfig() {
	(*e).HackerTeamConfigPath = filepath.Join((*e).ConfigFolderPath, hackerTeamConfig)
	// TODO: 读取并解析 configPath 中的 YAML 配置
	_, err := os.Stat((*e).HackerTeamConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，创建一个默认的 config.yaml
			fd, err := os.OpenFile((*e).HackerTeamConfigPath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("创建默认配置文件错误：%v", err))
			}
			defer fd.Close()
			//生成一个随机的用户ID，替换掉配置文件中的占位符
			cfg := strings.ReplaceAll(config.Template, "{USERID}", uuid.New().String())
			_, err = fd.WriteString(cfg)
			if err != nil {
				(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("写入默认配置文件错误：%v,按任意键退出", err))
			}
			(*e).tui.ShowSuccessInMsgViewAndExit("检查到配置文件不存在，已创建默认配置文件。请根据实际情况修改配置文件后重新启动程序！")
		} else {
			(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("检查配置文件错误：%v", err))
		}
	}

}

// 检查5个技能文件夹是否存在，不存在则创建并从嵌入模板复制技能
func (e *Engine) checkSkillsFolder() {

	(*e).ReconSkillsFolderPath = filepath.Join((*e).ConfigFolderPath, reconSkillsFolder)
	(*e).ExploitSkillsFolderPath = filepath.Join((*e).ConfigFolderPath, exploitSkillsFolder)
	(*e).PostExploitSkillsFolderPath = filepath.Join((*e).ConfigFolderPath, postExploitSkillsFolder)
	(*e).ScannerSkillsFolderPath = filepath.Join((*e).ConfigFolderPath, scannerSkillsFolder)
	(*e).ReproducerSkillsFolderPath = filepath.Join((*e).ConfigFolderPath, reproducerSkillsFolder)

	func(skillsFolders []string) {
		for _, folder := range skillsFolders {
			_, err := os.Stat(folder)
			if err != nil {
				if os.IsNotExist(err) {
					//skills 文件夹不存在，创建一个默认的 skills 文件夹
					err := os.MkdirAll(folder, os.ModePerm)
					if err != nil {
						(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("创建默认%s文件夹错误：%s", folder, err.Error()))
					}
					err = copy.Copy("skillsTemplates/pentest-tools", filepath.Join(folder, "pentest-tools"), copy.Options{FS: ToolSkills})
					if err != nil {
						(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("复制技能模板到%s文件夹错误：%s", folder, err.Error()))
					}
					(*e).tui.ShowSuccessInMsgView(fmt.Sprintf("检查到%s文件夹不存在，已创建", folder))
				} else {
					(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("检查%s文件夹错误：%s", folder, err.Error()))
				}
			}
		}
	}([]string{(*e).ReconSkillsFolderPath, (*e).ExploitSkillsFolderPath, (*e).PostExploitSkillsFolderPath, (*e).ScannerSkillsFolderPath, (*e).ReproducerSkillsFolderPath})

}

func (e *Engine) initInMemorySessionService() {
	(*e).SessionService_p = session.NewMemorySessionService((*(*e).Config_p).Model, (*e).tui)
}

func (e *Engine) initSqliteMemoryService() {
	service, err := memory.NewSQLiteMemoryService((*(*e).Config_p).Model, filepath.Join((*e).ConfigFolderPath, memoryDBFileName))
	if err != nil {
		(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("初始化sqlite记忆服务错误: %v", err))
	}
	(*e).SqliteMemoryService = service
}

func (e *Engine) newRunner() {
	Runner := runner.NewRunnerWithAgentFactory(
		(*e).Agentname,
		(*e).Agentname,
		func(ctx context.Context, ro ag.RunOptions) (ag.Agent, error) {
			e.reload()
			captain, err := e.initCaptain()
			if err != nil {
				return nil, err
			}
			exploit, err := e.initexploit()
			if err != nil {
				return nil, err
			}
			postexploit, err := e.initpostexploit()
			if err != nil {
				return nil, err
			}
			recon, err := e.initRecon()
			if err != nil {
				return nil, err
			}
			scanner, err := e.initScanner()
			if err != nil {
				return nil, err
			}
			reproducer, err := e.initReproducer()
			if err != nil {
				return nil, err
			}

			team.New(
				captain,
				[]ag.Agent{exploit, postexploit, recon, scanner, reproducer},
				team.WithDescription("A hacker team with one captain and three members, responsible for penetration testing tasks."),
				team.WithMemberToolStreamInner(true),                        //子agent的内部事件透传到父流程(TUI)
				team.WithMemberToolInnerTextMode(team.InnerTextModeInclude), //展示子agent完整transcript(正文+tool call+tool result)
			)
			return captain, nil
		},
		runner.WithSessionService((*e).SessionService_p),
		runner.WithMemoryService((*e).SqliteMemoryService),
	)
	(*e).AgentRunner_p = &Agentrunner{
		Runner: Runner,
		Stream: (*(*e).Config_p).Model.Stream,
	}
}

func (e *Engine) LoadConfig() {
	//加载配置文件
	c, err := config.LoadConfig((*e).HackerTeamConfigPath)
	if err != nil {
		(*e).tui.ShowErrorInMsgViewAndExit(pretty.TErrorF("加载配置文件错误: %v,按任意键退出", err))
	}
	(*e).Config_p = c
}

// redirectFrameworkLog 将框架的日志输出从 stdout 重定向到可执行文件同目录下的 HackerTeam.log 文件-created by copilot
func (e *Engine) redirectFrameworkLog() {
	logPath := filepath.Join((*e).ConfigFolderPath, hackerTeamLogFile)
	var err error
	(*e).FrameworkLogFile_p, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
		zapcore.AddSync((*e).FrameworkLogFile_p),
		zapcore.DebugLevel,
	)
	fileLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	//定向trpc-agent-go的日志输出到文件
	log.Default = fileLogger
	log.ContextDefault = fileLogger

	//定向trpc-mcp-go的日志输出到文件
	mcp.SetDefaultLogger(fileLogger)

	//重定向标准库 log 到文件（避免 gse 等第三方库的日志污染终端）
	if (*e).FrameworkLogFile_p != nil {
		stdlog.SetOutput((*e).FrameworkLogFile_p)
	}
}
