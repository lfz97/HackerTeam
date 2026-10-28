package engine

// assemble 引擎资产装配：Config、提示词、会话/记忆服务、内置工具。
// 与 bootstrap 的检查分工：Check 只做路径存在性检查与默认文件落盘；
// 这里在 absorb（路径已拷入引擎字段）之后由引擎创建真正的运行时对象。

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/memory"
	"HackerTeam/service/engine/session"
	functionTools "HackerTeam/service/engine/tools/functions"
	"HackerTeam/service/engine/tools/toolsets/localexec"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	framesession "trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// assembleConfig 加载配置文件到引擎（⑥）。
func (e *Engine) assembleConfig() error {
	cfg, err := config.LoadConfig(e.HackerTeamConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}
	e.Config_p = cfg
	return nil
}

// buildSessionService 构建会话服务：默认 SQLite 持久化（<CWD>/.HackerTeam/session.db）。
// 会话按工作目录归属：同 CWD 的多实例共享同一 db（WAL 等参数支撑并发读写），不同 CWD 各自独立。
// 创建失败降级为内存服务（不阻断启动）：目录不存在则建目录重试一次，其余原因直接降级。
// "路径不存在"用 os.Stat 判定而非解析驱动错误：go-sqlite3 的 CantOpen 不保证 errors.Is 到 fs.ErrNotExist。
func (e *Engine) buildSessionService() framesession.Service {
	dbPath := filepath.Join(e.CWD, hackerTeamConfigFolder, sessionDBFileName)

	svc, err := session.NewSQLiteSessionService(dbPath, e.Config_p.Model, e)
	if err == nil {
		return svc
	}

	if _, statErr := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(statErr) {
		if mkErr := os.MkdirAll(filepath.Dir(dbPath), os.ModePerm); mkErr != nil {
			e.setNotice(NoticeWarning, fmt.Sprintf(
				"sqlite session db directory create failed (%v), fallback to in-memory session service", mkErr))
			return session.NewMemorySessionService(e.Config_p.Model, e)
		}
		svc, err = session.NewSQLiteSessionService(dbPath, e.Config_p.Model, e)
		if err == nil {
			return svc
		}
	}

	e.setNotice(NoticeWarning, fmt.Sprintf(
		"sqlite session service failed (%v), fallback to in-memory session service", err))
	return session.NewMemorySessionService(e.Config_p.Model, e)
}

// buildMemoryService 构建 sqlite 记忆服务（⑨）。
func (e *Engine) buildMemoryService() error {
	memoryService, err := memory.NewSQLiteMemoryService(filepath.Join(e.ConfigFolderPath, memoryDBFileName))
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite memory service: %w", err)
	}
	e.SqliteMemoryService = memoryService
	return nil
}

// buildBuiltinAssets 内置 function 工具与共享 localexec 工具集（⑩）。
// 作业表全进程一份：Manager 跨轮复用（上一轮提交的长任务下一轮仍可
// status/output 续查），Operator 与 Worker 看到的是同一个命名空间。
func (e *Engine) buildBuiltinAssets() {
	tools := []tool.Tool{}
	tools = append(tools, functionTools.GetFileSystemTools()...)
	tools = append(tools, functionTools.GetFileOperationsTools()...)
	tools = append(tools, functionTools.GetDateTools()...)

	e.builtinTools = tools
	e.execToolSet = localexec.LocalExec()
}

// buildPrompts 构造环境上下文提示词与三个共享共识提示词片段（⑦，模板占位符替换）。
func (e *Engine) buildPrompts(agentName string) {
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

	prompt = strings.ReplaceAll(prompt, "{{CONFIGPATH}}", e.ConfigFolderPath)

	prompt = strings.ReplaceAll(prompt, "{{HackerTeamConfig}}", hackerTeamConfig)
	prompt = strings.ReplaceAll(prompt, "{{HackerTeamLogFile}}", hackerTeamLogFile)

	//输出目录（每次运行按时间戳独立，便于事后审计；锚定 ExeDir，与 localexec/tooloutput 保持一致）
	now := time.Now().Format("20060102150405")
	outDir := filepath.Join(e.ExeDir, outputDir, now)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUTDIR}}", outDir)

	e.EnvPrompt = prompt
	e.OutputDir = outDir

	// 读取共享的 Command Execution 提示词片段（Operator/Worker 共用）
	cmdExecBytes, _ := PromptFiles.ReadFile("prompts/common/command_execution.md")
	e.CommandExecutionPrompt = string(cmdExecBytes)

	// 读取共享的 Vuln Consensus 提示词片段（漏洞定义与定级共识）
	vulnConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/vuln_consensus.md")
	e.VulnConsensusPrompt = string(vulnConsensusBytes)

	// 读取共享的 Output Consensus 提示词片段（结果输出规范）
	toolConsensusBytes, _ := PromptFiles.ReadFile("prompts/common/output_consensus.md")
	e.OutputConsensusPrompt = string(toolConsensusBytes)
}
