package bootstrap

import (
	"HackerTeam/config"
	"HackerTeam/handler"
	"HackerTeam/session"
	"HackerTeam/toolsets"
	"HackerTeam/toolsets/localexec"
	"HackerTeam/tui/global_object"
	"HackerTeam/utils/pretty"
	"embed"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"trpc.group/trpc-go/trpc-agent-go/log"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	"trpc.group/trpc-go/trpc-agent-go/team"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-mcp-go"
)

// 定义核心的状态变量
var (
	Config_p               *config.Config
	Agentname              string
	CWD                    string
	ConfigFolderPath       string
	HackerTeamConfigPath   string
	SkillFolderPath        string
	AgentRunner            handler.AgentRunner
	InMemorySessionService *inmemory.SessionService
	frameworkLogFile       *os.File // 保存日志文件句柄，防止被 GC 回收

	//go:embed prompt/*
	PromptFiles embed.FS

	envPrompt string
	Tools     []tool.Tool
	Toolsets  []tool.ToolSet
)

// 定义配置文件夹中的各种配置文件名称
const (
	HackerTeamConfigFolder string = ".HackerTeam"
	HackerTeamConfig       string = "HackerTeam.yaml"
	SkillsFolder           string = "skills"
	HackerTeamLogFile      string = "HackerTeam.log"
	OperationRecord        string = "OperationRecord.md"
	outputDir              string = "output"
)

func Init(an string) handler.AgentRunner {
	Agentname = an

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

	//初始化AgentRunner
	AgentRunner = NewRunner()
	return AgentRunner
}

// 配置系统提示词，替换其中的占位符
func configENVPrompt() {
	envPrompt_b, _ := PromptFiles.ReadFile("prompt/env.md")
	envPrompt = string(envPrompt_b)
	//Agent名称
	envPrompt = strings.ReplaceAll(envPrompt, "{{NAME}}", Agentname)

	//当前日期
	envPrompt = strings.ReplaceAll(envPrompt, "{{DATE}}", time.Now().Format("2006-01-02 15:04:05 (Mon)"))

	//当前时区
	zone, _ := time.Now().Zone()
	envPrompt = strings.ReplaceAll(envPrompt, "{{TIMEZONE}}", fmt.Sprintf("%s (%s)", time.Now().Location().String(), zone))

	//操作系统
	envPrompt = strings.ReplaceAll(envPrompt, "{{OSTYPE}}", runtime.GOOS)

	//CPU架构
	envPrompt = strings.ReplaceAll(envPrompt, "{{AARCH}}", runtime.GOARCH)

	//主目录
	homeDir, _ := os.UserHomeDir()
	envPrompt = strings.ReplaceAll(envPrompt, "{{HOME}}", homeDir)

	//临时目录
	envPrompt = strings.ReplaceAll(envPrompt, "{{TMPDIR}}", os.TempDir())

	//当前用户
	u, _ := user.Current()
	envPrompt = strings.ReplaceAll(envPrompt, "{{CURRENTUSER}}", u.Username)

	//主机名
	hostName, _ := os.Hostname()
	envPrompt = strings.ReplaceAll(envPrompt, "{{HOSTNAME}}", hostName)

	//运行目录
	envPrompt = strings.ReplaceAll(envPrompt, "{{CWD}}", CWD)

	//配置目录
	envPrompt = strings.ReplaceAll(envPrompt, "{{CONFIGPATH}}", ConfigFolderPath)

	//配置文件
	envPrompt = strings.ReplaceAll(envPrompt, "{{HackerTeamConfig}}", HackerTeamConfig)
	envPrompt = strings.ReplaceAll(envPrompt, "{{SkillsFolder}}", SkillsFolder)
	envPrompt = strings.ReplaceAll(envPrompt, "{{HackerTeamLogFile}}", HackerTeamLogFile)
	envPrompt = strings.ReplaceAll(envPrompt, "{{OperationRecord}}", OperationRecord)

	//输出目录
	outputDir := filepath.Join(CWD, outputDir)
	envPrompt = strings.ReplaceAll(envPrompt, "{{OUTPUTDIR}}", outputDir)
}

// 获取当前可执行文件所在的目录完整路径
func getcwd() {

	exePath, err := os.Executable() // 获取当前可执行文件的路径
	if err != nil {
		ShowErrorAndExit(pretty.TErrorF("获取可执行文件目录错误: %v,按任意键退出", err))
	}
	CWD = filepath.Dir(exePath) // 获取当前可执行文件的目录路径（不包含程序名）

}

// 检查配置文件夹是否存在
func checkConfigFolder() {
	ConfigFolderPath = filepath.Join(CWD, HackerTeamConfigFolder)
	_, err := os.Stat(ConfigFolderPath)
	if err != nil {
		if os.IsNotExist(err) {
			//config 文件夹不存在，创建一个默认的 config 文件夹
			err := os.MkdirAll(ConfigFolderPath, os.ModePerm)
			if err != nil {
				ShowErrorAndExit(pretty.TErrorF("创建默认config文件夹错误：%v", err))
			}
			ShowSuccess("检查到config文件夹不存在，已创建默认config文件夹")
		} else {
			ShowErrorAndExit(pretty.TErrorF("检查config文件夹错误：%v", err))
		}
	} else {
		ShowSuccess("检查配置文件夹通过")
	}

}

// 检查配置文件是否存在，不存在则创建一个默认的配置文件
func checkConfig() {
	HackerTeamConfigPath = filepath.Join(ConfigFolderPath, HackerTeamConfig)
	// TODO: 读取并解析 configPath 中的 YAML 配置
	_, err := os.Stat(HackerTeamConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，创建一个默认的 config.yaml
			fd, err := os.OpenFile(HackerTeamConfigPath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				ShowErrorAndExit(pretty.TErrorF("创建默认配置文件错误：%v", err))
			}
			defer fd.Close()
			//生成一个随机的用户ID，替换掉配置文件中的占位符
			cfg := strings.ReplaceAll(config.Template, "{USERID}", uuid.New().String())
			_, err = fd.WriteString(cfg)
			if err != nil {
				ShowErrorAndExit(pretty.TErrorF("写入默认配置文件错误：%v,按任意键退出", err))
			}
			ShowSuccessAndExit("检查到配置文件不存在，已创建默认配置文件。请根据实际情况修改配置文件后重新启动程序！")
		} else {
			ShowErrorAndExit(pretty.TErrorF("检查配置文件错误：%v", err))
		}
	} else {
		ShowSuccess("检查配置文件通过!")
	}

}

func checkSkillsFolder() {
	SkillFolderPath = filepath.Join(ConfigFolderPath, "skills")
	_, err := os.Stat(SkillFolderPath)
	if err != nil {
		if os.IsNotExist(err) {
			//skills 文件夹不存在，创建一个默认的 skills 文件夹
			err := os.MkdirAll(SkillFolderPath, os.ModePerm)
			if err != nil {
				ShowErrorAndExit(pretty.TErrorF("创建默认skills文件夹错误：%v", err))
			}
			ShowSuccess("检查到skills文件夹不存在，已创建默认skills文件夹")
		} else {
			ShowErrorAndExit(pretty.TErrorF("检查skills文件夹错误：%v", err))
		}
	} else {
		ShowSuccess("检查skills文件夹通过")
	}
}

func loadConfig() (*config.Config, error) {
	YamlConfig := config.Config{}
	yamlFile, err := os.ReadFile(HackerTeamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件错误：%v", err)
	}
	err = yaml.Unmarshal(yamlFile, &YamlConfig)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件错误：%v", err)
	}
	return &YamlConfig, nil
}

func parseConfig() {

	if len((*Config_p).Mcp) != 0 {
		//读取配置文件中的 MCP 配置，创建 MCP ToolSet 并添加到 Toolsets 中
		for _, mcpConfig := range (*Config_p).Mcp {
			//只有配置了 Enabled 字段为 true 的 MCP 配置才会被创建 ToolSet 并添加到 Toolsets 中
			if mcpConfig.Enabled == true {
				mcpToolSet := toolsets.MCP(string(mcpConfig.Type), mcpConfig.Endpoint, mcpConfig.Headers)
				Toolsets = append(Toolsets, mcpToolSet)
			}

		}
	}
	if len((*Config_p).StdinMcp) != 0 {
		//读取配置文件中的 StdinMCP 配置，创建 StdinMCP ToolSet 并添加到 Toolsets 中
		for _, stdinMcpConfig := range (*Config_p).StdinMcp {
			if stdinMcpConfig.Enabled == true {
				stdinMcpToolSet := toolsets.StdinMCP(stdinMcpConfig.Command, stdinMcpConfig.Args)
				Toolsets = append(Toolsets, stdinMcpToolSet)
			}
		}
	}

	Toolsets = append(Toolsets, localexec.LocalExec()) //localexec 必须启用

}

func initMemorySessionService() {
	InMemorySessionService = session.NewMemorySessionService((*Config_p).Model)
}

func initTeam() runner.Runner {
	CaptainAgent := initCaptain()
	exploitAgent := initexploit()
	postexploitAgent := initpostexploit()
	reconAgent := initRecon()
	vulnanalyzeAgent := initvulnanalyst()

	team.New(
		CaptainAgent,
		[]agent.Agent{exploitAgent, postexploitAgent, reconAgent, vulnanalyzeAgent},
		team.WithDescription("A hacker team with one captain and three members, responsible for penetration testing tasks."),
	)
	Runner := runner.NewRunner(Agentname, CaptainAgent,
		runner.WithSessionService(InMemorySessionService), // 使用内存会话服务，其中包含自动摘要功能
	)
	return Runner
}

func LoadConfig() {
	//加载配置文件
	config_p, err := loadConfig()
	if err != nil {
		ShowErrorAndExit(pretty.TErrorF("加载配置文件错误: %v,按任意键退出", err))
	}
	Config_p = config_p
}

func NewRunner() handler.AgentRunner {
	//解析配置文件
	parseConfig()
	runner := initTeam()
	ar := handler.AgentRunner{
		Runner: runner,
		Stream: (*Config_p).Model.Stream,
		UserId: (*Config_p).User.UserID,
	}
	global_object.Print2LogView(pretty.TReady(Agentname))
	return ar
}

func ShowErrorAndExit(errmsg string) {
	done := make(chan struct{})
	global_object.Print2LogView(errmsg)
	global_object.App_p.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		global_object.App_p.SetFocus(global_object.LogView_p)
		global_object.LogView_p.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				global_object.App_p.Stop()
				return nil
			})
	})
	<-done
}
func ShowSuccess(sussessmsg string) {
	global_object.Print2LogView(pretty.TSuccess(sussessmsg))
}
func ShowSuccessAndExit(sussessmsg string) {
	done := make(chan struct{})
	global_object.Print2LogView(pretty.TSuccess(sussessmsg))
	global_object.App_p.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		global_object.App_p.SetFocus(global_object.LogView_p)
		global_object.LogView_p.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				global_object.App_p.Stop()
				return nil
			})
	})
	<-done
}

// redirectFrameworkLog 将框架的日志输出从 stdout 重定向到可执行文件同目录下的 HackerTeam.log 文件-created by copilot
func redirectFrameworkLog() {
	logPath := filepath.Join(ConfigFolderPath, HackerTeamLogFile)
	var err error
	frameworkLogFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
		zapcore.AddSync(frameworkLogFile),
		zapcore.DebugLevel,
	)
	fileLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	//定向trpc-agent-go的日志输出到文件
	log.Default = fileLogger
	log.ContextDefault = fileLogger

	//定向trpc-mcp-go的日志输出到文件
	mcp.SetDefaultLogger(fileLogger)
}
