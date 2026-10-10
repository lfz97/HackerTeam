package engine

// bootstrap 环境检查器：只做检查与供给，返回错误和产物；不驻留、不碰 UI。
// 致命呈现（parkWithFatal）与通知上屏（setNotice）是 Engine 的职责，在 Init
// 里完成。运行时回调（agent factory / summarySink / 状态栏）由调用方以窄接口
// 或方法值注入，检查器不认识 *Engine。

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/memory"
	"HackerTeam/service/engine/session"
	functionTools "HackerTeam/service/engine/tools/functions"
	"HackerTeam/service/engine/tools/toolsets/localexec"
	"embed"
	"fmt"
	"io/fs"
	stdlog "log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/otiai10/copy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"trpc.group/trpc-go/trpc-agent-go/log"
	framememory "trpc.group/trpc-go/trpc-agent-go/memory"
	framsession "trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	mcp "trpc.group/trpc-go/trpc-mcp-go"
)

//go:embed prompts/*
var PromptFiles embed.FS

//go:embed skillsTemplates/*
var ToolSkills embed.FS

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

// summarySink 本文件对摘要展示端的全部需求（Engine 以 AppendSummary 实现）。
type summarySink interface {
	AppendSummary(text string)
}

// bootNotice 启动期间收集的非致命提示（absorb 时经 setNotice 上屏）。
type bootNotice struct {
	Kind string
	Text string
}

// BootEnv 环境检查与资产装配的产物，Engine 吸收后即可构造 runner。
type BootEnv struct {
	CWD                  string
	ConfigFolderPath     string
	HackerTeamConfigPath string
	// 5 个角色的 skills 文件夹路径
	RoleSkillFolders map[string]string

	LogFile     *os.File
	NeedRestart bool //首跑创建了默认配置文件：须改完配置重启

	Config                 *config.Config
	EnvPrompt              string //环境上下文提示词（prompts/common/env.md，已替换占位符）
	CommandExecutionPrompt string //共享的命令执行提示词片段
	VulnConsensusPrompt    string //共享的漏洞定义与定级共识提示词
	OutputConsensusPrompt  string //共享的结果输出规范提示词

	SessionService      framsession.Service
	SqliteMemoryService framememory.Service
	BuiltinTools        []tool.Tool
	BuiltinToolsets     map[string]tool.ToolSet //每个执行角色一个常驻 localexec 实例

	Notices []bootNotice
}

// Check 跑完整的启动检查与资产装配。任何一步失败都返回带步骤语义的
// error，由调用方决定如何呈现（parkWithFatal）。
func Check(agentName string, sink summarySink) (*BootEnv, error) {
	env := &BootEnv{RoleSkillFolders: map[string]string{}}

	// ① 可执行文件所在目录：一切路径的锚点
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取可执行文件目录错误: %w", err)
	}
	env.CWD = filepath.Dir(exePath)

	// ② 配置文件夹：不存在则创建默认的
	env.ConfigFolderPath = filepath.Join(env.CWD, hackerTeamConfigFolder)
	if _, err := os.Stat(env.ConfigFolderPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("检查config文件夹错误: %w", err)
		}
		if err := os.MkdirAll(env.ConfigFolderPath, os.ModePerm); err != nil {
			return nil, fmt.Errorf("创建默认config文件夹错误: %w", err)
		}
		env.Notices = append(env.Notices, bootNotice{NoticeSuccess, "config folder not found, created default"})
	}

	// ③ 配置文件：不存在则写入默认模板，本次启动到此为止（须改完配置重启）
	env.HackerTeamConfigPath = filepath.Join(env.ConfigFolderPath, hackerTeamConfig)
	if _, err := os.Stat(env.HackerTeamConfigPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("检查配置文件错误: %w", err)
		}
		fd, err := os.OpenFile(env.HackerTeamConfigPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return nil, fmt.Errorf("创建默认配置文件错误: %w", err)
		}
		defer fd.Close()
		cfg := strings.ReplaceAll(config.Template, "{USERID}", uuid.New().String())
		if _, err := fd.WriteString(cfg); err != nil {
			return nil, fmt.Errorf("写入默认配置文件错误: %w", err)
		}
		env.NeedRestart = true
		return env, nil
	}

	// ④ 5 个角色的 skills 文件夹：不存在则创建并从嵌入模板复制技能
	if err := env.checkRoleSkillFolders(); err != nil {
		return nil, err
	}

	// ⑤ 框架日志重定向到文件，避免输出干扰 TUI（失败则静默跳过，保持默认输出）
	env.redirectFrameworkLog()

	// ⑥ 加载配置文件
	cfg, err := config.LoadConfig(env.HackerTeamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件错误: %w", err)
	}
	env.Config = cfg

	// ⑦ 环境提示词与共享共识提示词（模板占位符替换）
	env.buildPrompts(agentName)

	// ⑧ 会话服务（摘要展示经 sink 注入）
	env.SessionService = session.NewMemorySessionService(env.Config.Model, sink)

	// ⑨ sqlite 记忆服务
	memoryService, err := memory.NewSQLiteMemoryService(filepath.Join(env.ConfigFolderPath, memoryDBFileName))
	if err != nil {
		return nil, fmt.Errorf("初始化sqlite记忆服务错误: %w", err)
	}
	env.SqliteMemoryService = memoryService

	// ⑩ 内置 function 工具与每角色 localexec 工具集
	tools, toolsets, err := builtinAssets()
	if err != nil {
		return nil, err
	}
	env.BuiltinTools = tools
	env.BuiltinToolsets = toolsets

	return env, nil
}

// checkRoleSkillFolders 检查 5 个角色的 skills 文件夹，不存在则创建并从
// skillsTemplates 嵌入模板复制该角色的预设技能（Reproducer 无预设，仅建空目录）。
func (env *BootEnv) checkRoleSkillFolders() error {
	// 角色目录 → skillsTemplates 下的预设目录（按角色分发各自的红队知识 skill）
	presetFolders := []struct {
		roleFolder string
		presetName string
	}{
		{reconSkillsFolder, "Recon"},
		{scannerSkillsFolder, "Scanner"},
		{exploitSkillsFolder, "Exploit"},
		{postExploitSkillsFolder, "PostExploit"},
		{reproducerSkillsFolder, ""},
	}
	for _, pf := range presetFolders {
		roleFolder := filepath.Join(env.ConfigFolderPath, pf.roleFolder)
		env.RoleSkillFolders[pf.roleFolder] = roleFolder
		if _, err := os.Stat(roleFolder); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查%s文件夹错误: %w", pf.roleFolder, err)
		}
		if err := os.MkdirAll(roleFolder, os.ModePerm); err != nil {
			return fmt.Errorf("创建默认%s文件夹错误: %w", pf.roleFolder, err)
		}
		if pf.presetName != "" {
			if err := copy.Copy("skillsTemplates/"+pf.presetName, roleFolder, copy.Options{FS: ToolSkills}); err != nil {
				return fmt.Errorf("复制技能模板到%s文件夹错误: %w", pf.roleFolder, err)
			}
			// embedFS 源文件只读(0444)，复制后修正权限保证 skill 可编辑
			if err := makeSkillsWritable(roleFolder); err != nil {
				return fmt.Errorf("修正%s文件夹权限错误: %w", pf.roleFolder, err)
			}
		}
		env.Notices = append(env.Notices, bootNotice{NoticeSuccess, fmt.Sprintf("%s folder not found, created default", pf.roleFolder)})
	}
	return nil
}

// makeSkillsWritable 修正 embedFS 复制出的只读权限(0444/0555)，保证 skill 文件与目录可编辑。
// otiai10/copy 对目录的 chmod 是异步 defer 执行的，复制返回后显式遍历修正更可靠。
func makeSkillsWritable(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}

// redirectFrameworkLog 将框架/mcp/标准库的日志输出重定向到 HackerTeam.log。
func (env *BootEnv) redirectFrameworkLog() {
	logPath := filepath.Join(env.ConfigFolderPath, hackerTeamLogFile)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	env.LogFile = logFile
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
		zapcore.AddSync(logFile),
		zapcore.DebugLevel,
	)
	fileLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	//定向trpc-agent-go的日志输出到文件
	log.Default = fileLogger
	log.ContextDefault = fileLogger

	//定向trpc-mcp-go的日志输出到文件
	mcp.SetDefaultLogger(fileLogger)

	//重定向标准库 log 到文件（避免 gse 等第三方库的日志污染终端）
	stdlog.SetOutput(logFile)
}

// buildPrompts 构造环境上下文提示词与三个共享共识提示词片段（模板占位符替换）。
func (env *BootEnv) buildPrompts(agentName string) {
	envPrompt_b, _ := PromptFiles.ReadFile("prompts/common/env.md")
	prompt := string(envPrompt_b)
	//Agent名称
	prompt = strings.ReplaceAll(prompt, "{{NAME}}", agentName)

	//当前日期
	//prompt = strings.ReplaceAll(prompt, "{{DATE}}", time.Now().Format("2006-01-02 15:04:05 (Mon)"))

	//当前时区
	zone, _ := time.Now().Zone()
	prompt = strings.ReplaceAll(prompt, "{{TIMEZONE}}", fmt.Sprintf("%s (%s)", time.Now().Location().String(), zone))

	//操作系统
	prompt = strings.ReplaceAll(prompt, "{{OSTYPE}}", runtime.GOOS)

	//CPU架构
	prompt = strings.ReplaceAll(prompt, "{{AARCH}}", runtime.GOARCH)

	//主目录
	homeDir, _ := os.UserHomeDir()
	prompt = strings.ReplaceAll(prompt, "{{HOME}}", homeDir)

	//临时目录
	prompt = strings.ReplaceAll(prompt, "{{TMPDIR}}", os.TempDir())

	//当前用户
	u, _ := user.Current()
	prompt = strings.ReplaceAll(prompt, "{{CURRENTUSER}}", u.Username)

	//主机名
	hostName, _ := os.Hostname()
	prompt = strings.ReplaceAll(prompt, "{{HOSTNAME}}", hostName)

	//运行目录
	//prompt = strings.ReplaceAll(prompt, "{{CWD}}", env.CWD)

	//配置目录
	prompt = strings.ReplaceAll(prompt, "{{CONFIGPATH}}", env.ConfigFolderPath)

	//配置文件
	prompt = strings.ReplaceAll(prompt, "{{HackerTeamConfig}}", hackerTeamConfig)
	prompt = strings.ReplaceAll(prompt, "{{HackerTeamLogFile}}", hackerTeamLogFile)
	//prompt = strings.ReplaceAll(prompt, "{{OperationRecord}}", operationRecord)

	//输出目录（每次运行按时间戳独立，便于事后审计）
	now := time.Now().Format("20060102150405")
	outDir := filepath.Join(env.CWD, outputDir, now)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUTDIR}}", outDir)

	env.EnvPrompt = prompt

	// 读取共享的 Command Execution 提示词片段（sub-agent 共用）
	cmdExecBytes, _ := PromptFiles.ReadFile("prompts/common/command_execution.md")
	env.CommandExecutionPrompt = string(cmdExecBytes)

	// 读取共享的 Vuln Consensus 提示词片段（漏洞定义与定级共识）
	vulnConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/vuln_consensus.md")
	env.VulnConsensusPrompt = string(vulnConsensusBytes)

	// 读取共享的 Output Consensus 提示词片段（结果输出规范）
	toolConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/output_consensus.md")
	env.OutputConsensusPrompt = string(toolConsensusBytes)
}

// builtinAssets 内置 function 工具（文件系统/文件操作/日期）与每个执行角色
// 一个常驻 localexec 工具集（Manager 保持 per-agent，作业命名空间隔离，
// 实例跨轮复用——上一轮提交的长任务在下一轮仍可通过 status/output 续查）。
func builtinAssets() ([]tool.Tool, map[string]tool.ToolSet, error) {
	tools := []tool.Tool{}
	tools = append(tools, functionTools.GetFileSystemTools()...)
	tools = append(tools, functionTools.GetFileOperationsTools()...)
	tools = append(tools, functionTools.GetDateTools()...)

	toolsets := map[string]tool.ToolSet{}
	for _, role := range []string{"Recon", "Scanner", "Exploit", "PostExploit", "Reproducer"} {
		toolsets[role] = localexec.LocalExec()
	}

	return tools, toolsets, nil
}
