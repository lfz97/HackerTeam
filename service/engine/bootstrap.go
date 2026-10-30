package engine

// bootstrap 环境检查器：只做检查与供给，返回错误和产物；不驻留、不碰 UI。
// 运行时对象（Config/服务/工具）由 Engine 在 absorb 后装配（见 assemble.go）。

import (
	"HackerTeam/service/engine/config"
	"embed"
	"fmt"
	"io"
	"io/fs"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/otiai10/copy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"trpc.group/trpc-go/trpc-agent-go/log"
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

// bootNotice 启动期间收集的非致命提示（absorb 时经 setNotice 上屏）。
type bootNotice struct {
	Kind string
	Text string
}

// BootEnv 环境检查的产物：只含路径、日志句柄与非致命提示；
// 运行时对象由 Engine 在 absorb 后装配（见 assemble.go）。
type BootEnv struct {
	CWD                  string //进程工作目录
	ExeDir               string //可执行文件目录，资产锚点（config/log/output 跟随二进制）
	ConfigFolderPath     string
	HackerTeamConfigPath string
	// SkillsFolderPath 合并后的唯一技能目录（Operator 与 Worker 共用）。
	SkillsFolderPath string

	LogFile       *os.File
	ConfigCreated bool //首跑创建了默认配置文件：进入首次启动配置流程（TUI 配置页收集模型配置）

	Notices []bootNotice
}

// Check 跑完整的启动检查：路径存在性、默认配置/技能目录落盘、日志重定向。
// 不实例化任何运行时对象。任何一步失败都返回带步骤语义的
// error，由调用方决定如何呈现（parkWithFatal）。
func Check() (*BootEnv, error) {
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

	// ④ 配置文件：不存在则写入默认模板，本次启动进入首次启动配置流程
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
		env.ConfigCreated = true
	}

	// ⑤ 技能目录：合并后的单一 Skills 目录，不存在则从嵌入模板安装并迁移旧角色目录。
	// 首启同样执行：不再有"改完配置重启"，技能目录必须本次就绪（reload 依赖它）。
	if err := env.checkSkillsFolder(); err != nil {
		return nil, err
	}

	return env, nil
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
// 日志文件打不开（如 exe 放在 Program Files 等只读目录）时降级为 io.Discard 黑洞：
// 重定向的第一目标是防止日志污染 TUI 终端，宁可丢弃日志也不回退到 stdout/stderr。
func (env *BootEnv) redirectFrameworkLog() {
	logPath := filepath.Join(env.ConfigFolderPath, hackerTeamLogFile)
	sink := zapcore.AddSync(io.Discard)
	if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		env.LogFile = logFile
		sink = zapcore.AddSync(logFile)
	} else {
		// 日志去哪了必须告知：走 bootNotices，absorb 时经 setNotice 上屏
		// （状态槽位 + 每帧拉取，TTL 内不惧与 TUI 启动的并发时序）
		env.Notices = append(env.Notices, bootNotice{NoticeWarning, "log file not writable, framework logs discarded"})
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
		sink,
		zapcore.DebugLevel,
	)
	fileLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
	//定向trpc-agent-go的日志输出到文件
	log.Default = fileLogger
	log.ContextDefault = fileLogger

	//定向trpc-mcp-go的日志输出到文件
	mcp.SetDefaultLogger(fileLogger)

	//重定向标准库 log 到文件（避免 gse 等第三方库的日志污染终端）
	stdlog.SetOutput(sink)
}
