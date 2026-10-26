package engine

// bootstrap 环境检查器：只做检查与供给，返回错误和产物；不驻留、不碰 UI。
// 致命呈现与通知上屏是 Engine（Init）的职责；运行时回调由调用方注入，检查器不认识 *Engine。

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
	sessionDBFileName      string = "session.db"
	operationRecord        string = "OperationRecord.md"
	outputDir              string = "output"
)

// 技能目录名称。重构后 Operator 与 Worker 是同一份实现、同一套技能，
// 不再按角色分目录——所有技能合并进 Skills 一个目录。
const skillsFolder string = "Skills"

// skillsTemplateDir 内置技能模板目录：与运行时 Skills/ 同构，整目录一次安装。
const skillsTemplateDir = "skillsTemplates/Skills"

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
	CWD                  string //进程工作目录
	ExeDir               string //可执行文件目录，资产锚点（config/log/output 跟随二进制）
	ConfigFolderPath     string
	HackerTeamConfigPath string
	// SkillsFolderPath 合并后的唯一技能目录（Operator 与 Worker 共用）。
	SkillsFolderPath string

	LogFile     *os.File
	NeedRestart bool //首跑创建了默认配置文件：须改完配置重启

	Config                 *config.Config
	EnvPrompt              string //环境上下文提示词（prompts/common/env.md，已替换占位符）
	CommandExecutionPrompt string //共享的命令执行提示词片段
	VulnConsensusPrompt    string //共享的漏洞定义与定级共识提示词
	OutputConsensusPrompt  string //共享的结果输出规范提示词
	OutputDir              string //本次运行的输出根目录（每次启动按时间戳独立）

	SessionService      framsession.Service
	SqliteMemoryService framememory.Service
	BuiltinTools        []tool.Tool
	// ExecToolSet 是 Operator 与 Worker 共享的常驻 localexec 作业表。
	// 两者同源、同一命名空间：Worker 起的命令 Operator 能直接 status/output 续查，
	// 反之亦然。
	ExecToolSet tool.ToolSet

	Notices []bootNotice
}

// Check 跑完整的启动检查与资产装配。任何一步失败都返回带步骤语义的
// error，由调用方决定如何呈现（parkWithFatal）。
func Check(agentName string, sink summarySink) (*BootEnv, error) {
	env := &BootEnv{}

	// ① 可执行文件所在目录：一切资产的锚点
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable directory: %w", err)
	}
	env.ExeDir = filepath.Dir(exePath)

	// Getwd 失败（如启动目录被删）时回退 ExeDir，保证 CWD 非空。
	env.CWD, err = os.Getwd()
	if err != nil {
		env.CWD = env.ExeDir
	}

	// ② 配置文件夹：不存在则创建默认的
	env.ConfigFolderPath = filepath.Join(env.ExeDir, hackerTeamConfigFolder)
	if _, err := os.Stat(env.ConfigFolderPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to check config directory: %w", err)
		}
		if err := os.MkdirAll(env.ConfigFolderPath, os.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create default config directory: %w", err)
		}
		env.Notices = append(env.Notices, bootNotice{NoticeSuccess, "config folder not found, created default"})
	}

	// ③ 框架日志重定向到文件：Check 与 TUI 并发启动，越早静音 stderr，日志污染 TUI 的窗口越小
	env.redirectFrameworkLog()

	// ④ 配置文件：不存在则写入默认模板，本次启动到此为止（须改完配置重启）
	env.HackerTeamConfigPath = filepath.Join(env.ConfigFolderPath, hackerTeamConfig)
	if _, err := os.Stat(env.HackerTeamConfigPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to check config file: %w", err)
		}
		fd, err := os.OpenFile(env.HackerTeamConfigPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to create default config file: %w", err)
		}
		defer fd.Close()
		cfg := strings.ReplaceAll(config.Template, "{USERID}", uuid.New().String())
		if _, err := fd.WriteString(cfg); err != nil {
			return nil, fmt.Errorf("failed to write default config file: %w", err)
		}
		env.NeedRestart = true
		return env, nil
	}

	// ⑤ 技能目录：合并后的单一 Skills 目录，不存在则从嵌入模板安装并迁移旧角色目录
	if err := env.checkSkillsFolder(); err != nil {
		return nil, err
	}

	// ⑥ 加载配置文件
	cfg, err := config.LoadConfig(env.HackerTeamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}
	env.Config = cfg

	// ⑦ 环境提示词与共享共识提示词（模板占位符替换）
	env.buildPrompts(agentName)

	// ⑧ 会话服务：默认 SQLite（<CWD>/.HackerTeam/session.db），会话跨进程持久化；
	// 创建失败按原因降级内存服务（降级不阻断启动，原因经 Notices 上屏）
	env.SessionService = env.buildSessionService(sink)

	// ⑨ sqlite 记忆服务
	memoryService, err := memory.NewSQLiteMemoryService(filepath.Join(env.ConfigFolderPath, memoryDBFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sqlite memory service: %w", err)
	}
	env.SqliteMemoryService = memoryService

	// ⑩ 内置 function 工具与共享 localexec 工具集
	tools, execToolSet, err := builtinAssets()
	if err != nil {
		return nil, err
	}
	env.BuiltinTools = tools
	env.ExecToolSet = execToolSet

	return env, nil
}

// buildSessionService 构建会话服务：默认 SQLite 持久化（<CWD>/.HackerTeam/session.db）。
// 会话按工作目录归属：同 CWD 的多实例共享同一 db（WAL 等参数支撑并发读写），不同 CWD 各自独立。
// 创建失败降级为内存服务（不阻断启动）：目录不存在则建目录重试一次，其余原因直接降级。
// "路径不存在"用 os.Stat 判定而非解析驱动错误：go-sqlite3 的 CantOpen 不保证 errors.Is 到 fs.ErrNotExist。
func (env *BootEnv) buildSessionService(sink summarySink) framsession.Service {
	dbPath := filepath.Join(env.CWD, hackerTeamConfigFolder, sessionDBFileName)

	svc, err := session.NewSQLiteSessionService(dbPath, env.Config.Model, sink)
	if err == nil {
		return svc
	}

	if _, statErr := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(statErr) {
		if mkErr := os.MkdirAll(filepath.Dir(dbPath), os.ModePerm); mkErr != nil {
			env.Notices = append(env.Notices, bootNotice{NoticeWarning, fmt.Sprintf(
				"sqlite session db directory create failed (%v), fallback to in-memory session service", mkErr)})
			return session.NewMemorySessionService(env.Config.Model, sink)
		}
		svc, err = session.NewSQLiteSessionService(dbPath, env.Config.Model, sink)
		if err == nil {
			return svc
		}
	}

	env.Notices = append(env.Notices, bootNotice{NoticeWarning, fmt.Sprintf(
		"sqlite session service failed (%v), fallback to in-memory session service", err)})
	return session.NewMemorySessionService(env.Config.Model, sink)
}

// checkSkillsFolder 检查 Skills 目录：不存在则从嵌入模板整目录安装。
// 目录一旦存在就不再补给——删掉的技能文件不该在下次启动时复活。
func (env *BootEnv) checkSkillsFolder() error {
	skillsDir := filepath.Join(env.ConfigFolderPath, skillsFolder)
	env.SkillsFolderPath = skillsDir
	if _, err := os.Stat(skillsDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check %s folder: %w", skillsFolder, err)
	}
	if err := copy.Copy(skillsTemplateDir, skillsDir, copy.Options{FS: ToolSkills}); err != nil {
		return fmt.Errorf("failed to copy skill templates into %s folder: %w", skillsFolder, err)
	}
	// embedFS 源文件只读(0444)，复制后修正权限保证 skill 可编辑
	if err := makeSkillsWritable(skillsDir); err != nil {
		return fmt.Errorf("failed to fix %s folder permissions: %w", skillsFolder, err)
	}
	env.Notices = append(env.Notices, bootNotice{NoticeSuccess, fmt.Sprintf("%s folder not found, created default", skillsFolder)})
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
	prompt = strings.ReplaceAll(prompt, "{{NAME}}", agentName)
	// {{DATE}}/{{CWD}} 不注入：已由 BeforeModel 状态栏每次调用刷新提供
	zone, _ := time.Now().Zone()
	prompt = strings.ReplaceAll(prompt, "{{TIMEZONE}}", fmt.Sprintf("%s (%s)", time.Now().Location().String(), zone))

	prompt = strings.ReplaceAll(prompt, "{{OSTYPE}}", runtime.GOOS)

	prompt = strings.ReplaceAll(prompt, "{{AARCH}}", runtime.GOARCH)

	homeDir, _ := os.UserHomeDir()
	prompt = strings.ReplaceAll(prompt, "{{HOME}}", homeDir)

	prompt = strings.ReplaceAll(prompt, "{{TMPDIR}}", os.TempDir())

	u, _ := user.Current()
	prompt = strings.ReplaceAll(prompt, "{{CURRENTUSER}}", u.Username)

	hostName, _ := os.Hostname()
	prompt = strings.ReplaceAll(prompt, "{{HOSTNAME}}", hostName)

	prompt = strings.ReplaceAll(prompt, "{{CONFIGPATH}}", env.ConfigFolderPath)

	prompt = strings.ReplaceAll(prompt, "{{HackerTeamConfig}}", hackerTeamConfig)
	prompt = strings.ReplaceAll(prompt, "{{HackerTeamLogFile}}", hackerTeamLogFile)

	//输出目录（每次运行按时间戳独立，便于事后审计；锚定 ExeDir，与 localexec/tooloutput 保持一致）
	now := time.Now().Format("20060102150405")
	outDir := filepath.Join(env.ExeDir, outputDir, now)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUTDIR}}", outDir)

	env.EnvPrompt = prompt
	env.OutputDir = outDir

	// 读取共享的 Command Execution 提示词片段（Operator/Worker 共用）
	cmdExecBytes, _ := PromptFiles.ReadFile("prompts/common/command_execution.md")
	env.CommandExecutionPrompt = string(cmdExecBytes)

	// 读取共享的 Vuln Consensus 提示词片段（漏洞定义与定级共识）
	vulnConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/vuln_consensus.md")
	env.VulnConsensusPrompt = string(vulnConsensusBytes)

	// 读取共享的 Output Consensus 提示词片段（结果输出规范）
	toolConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/output_consensus.md")
	env.OutputConsensusPrompt = string(toolConsensusBytes)
}

// builtinAssets 内置 function 工具（文件系统/文件操作/日期）与一个共享的
// localexec 工具集。作业表全进程一份：Manager 跨轮复用（上一轮提交的长任务
// 下一轮仍可 status/output 续查），Operator 与 Worker 看到的是同一个命名空间。
func builtinAssets() ([]tool.Tool, tool.ToolSet, error) {
	tools := []tool.Tool{}
	tools = append(tools, functionTools.GetFileSystemTools()...)
	tools = append(tools, functionTools.GetFileOperationsTools()...)
	tools = append(tools, functionTools.GetDateTools()...)

	return tools, localexec.LocalExec(), nil
}
