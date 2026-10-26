package engine

import (
	"errors"
	"strings"

	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/models"
	functionTools "HackerTeam/service/engine/tools/functions"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type agentName string

var (
	operatorName agentName = "Operator"
	workerName   agentName = "Worker"
)

// agentSpec 是一次 agent 装配里"有差异"的部分。
//
// Operator 与 Worker 共用同一套模型参数、内置工具、执行工具集、技能库和上下文
// 压缩策略——差别只有提示词、额外工具、以及是否向 TUI 推状态。它们是同一份实现
// 的两个实例，不是两套代码：给 Operator 加工具或改压缩参数，Worker 自动同步。
type agentSpec struct {
	name        agentName
	promptPath  string
	description string

	// extraTools 只给 Operator：记忆、todo、taskrun 调度面。
	// Worker 一个都不挂——它调不动别人，也不维护跨轮状态。
	extraTools []tool.Tool

	// 状态栏开关。Worker 是无记忆的一次性工人，只看时间和工作目录：
	// 不查记忆、不读 todo、不关心在途的兄弟任务。
	todoSink   todoTextSink
	withTodo   bool
	withMemory bool
	withFlight bool

	preloadMemory int  // >0 时预加载记忆（仅 Operator）
	summaryToUser bool // 摘要注入 user message，避免与 system prompt 里的 SOP 争优先级
}

// buildAgent 装配 Operator 与 Worker 的唯一入口。
func (e *Engine) buildAgent(spec agentSpec) (*llmagent.LLMAgent, error) {
	prompt := e.assemblePrompt(spec.promptPath)

	tools := make([]tool.Tool, 0, len((*e).builtinTools)+len(spec.extraTools))
	tools = append(tools, (*e).builtinTools...)
	tools = append(tools, spec.extraTools...)

	// 执行工具集与 MCP 工具集是共享实例：同一份作业表、同一批 MCP 子进程。
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	if (*e).execToolSet != nil {
		toolsets = append(toolsets, (*e).execToolSet)
	}
	toolsets = append(toolsets, (*e).mcpToolsets...)

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens:       &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			ReasoningEffort: (*(*e).Config_p).Model.ReasoningEffortPtr(),
			Stream:          (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithTools(tools),
		llmagent.WithToolSets(toolsets),         // 共享 localexec + 配置文件声明的 MCP 工具集
		llmagent.WithRefreshToolSetsOnRun(true), // 每次run刷新MCP工具列表
		llmagent.WithAddSessionSummary(true),    // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),  // 在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),
		llmagent.WithContextCompactionToolResultMaxTokens(8192),
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),
		llmagent.WithEnableOnDemandSession(true),
		llmagent.WithDescription(spec.description),
		llmagent.WithGlobalInstruction(prompt),
	}
	if spec.summaryToUser {
		opts = append(opts, llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser))
	}
	if spec.preloadMemory > 0 {
		opts = append(opts, llmagent.WithPreloadMemory(spec.preloadMemory))
	}
	if (*e).skillsRepo != nil {
		opts = append(opts,
			llmagent.WithSkills((*e).skillsRepo),
			// 只注入知识，不给执行能力：命令统一走 localexec 作业表，
			// 否则 skill 自带的执行器会绕开作业表，长任务就查不到了。
			llmagent.WithSkillToolProfile(llmagent.SkillToolProfileKnowledgeOnly),
			// 技能正文随 tool result 返回而不是塞进 system prompt：system 段保持稳定，前缀缓存不被打断
			llmagent.WithSkillsLoadedContentInToolResults(true),
		)
	}
	opts = append(opts, (*e).beforeModelStatus(spec))
	return setAgent(spec.name, (*(*e).Config_p).Model, opts)
}

// initOperator 装配主 agent：与用户对话、自己执行、按需派 Worker。
func (e *Engine) initOperator() (*llmagent.LLMAgent, error) {
	extra := make([]tool.Tool, 0, 16)
	if (*e).SqliteMemoryService != nil {
		extra = append(extra, (*e).SqliteMemoryService.Tools()...) // 记忆工具：写入全靠 Operator 手动调用
	}
	extra = append(extra, functionTools.GetTodoTools()...) // 框架内置 todo_write：计划只由 Operator 持有
	if (*e).taskTools != nil {
		extra = append(extra, (*e).taskTools.All()...) // start/list/get/wait/cancel/read_task_run_transcript
	}
	return e.buildAgent(agentSpec{
		name:          operatorName,
		promptPath:    "prompts/agents/operator.md",
		description:   "Operator — owns the engagement: executes hands-on work directly and dispatches stateless Workers for breadth tasks. Talks to the user.",
		extraTools:    extra,
		todoSink:      e,
		withTodo:      true,
		withMemory:    true,
		withFlight:    true,
		preloadMemory: 10,
	})
}

// initWorker 装配一次性助手：同一套工具、同一份技能库、同一个模型，
// 但没有记忆、没有 todo、没有调度权——它只把手里那一件事做完并交回证据。
func (e *Engine) initWorker() (*llmagent.LLMAgent, error) {
	return e.buildAgent(agentSpec{
		name:          workerName,
		promptPath:    "prompts/agents/worker.md",
		description:   "Worker — stateless execution assistant. Give it one self-contained task; it executes with the shared tools and returns a report path plus a verdict. No memory, no delegation, no follow-up questions.",
		summaryToUser: true,
	})
}

// setAgent 根据 APIType 创建对应模型的 agent（无状态，包级函数）。
func setAgent(name agentName, m config.Model, opts []llmagent.Option) (*llmagent.LLMAgent, error) {
	switch m.APIType {
	case "openai":
		opts = append(opts, llmagent.WithModel(models.Openai(m)))
	case "anthropic":
		opts = append(opts, llmagent.WithModel(models.Anthropic(m)))
	default:
		return nil, errors.New("unsupported API type, check the Model.APIType field in your config")
	}
	return llmagent.New(string(name), opts...), nil
}

// 组装各个agent的提示词
func (e *Engine) assemblePrompt(path string) string {
	prompt_b, _ := PromptFiles.ReadFile(path)
	prompt := string(prompt_b)
	prompt = strings.ReplaceAll(prompt, "{{ENV}}", (*e).EnvPrompt)
	prompt = strings.ReplaceAll(prompt, "{{COMMAND_EXECUTION}}", (*e).CommandExecutionPrompt)
	prompt = strings.ReplaceAll(prompt, "{{VULN_CONSENSUS}}", (*e).VulnConsensusPrompt)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUT_CONSENSUS}}", (*e).OutputConsensusPrompt)
	// {{OUTPUTDIR}} 之前在 env.md 里替换，agent 提示词（报告路径、结果目录）拿到的
	// 一直是字面量；这里补上，两处统一到同一个本次运行的输出根目录。
	prompt = strings.ReplaceAll(prompt, "{{OUTPUTDIR}}", (*e).OutputDir)
	//todo_write 工具使用说明（框架 tool/todo.DefaultToolPrompt，随框架版本走，不在提示词里硬编码）
	prompt = strings.ReplaceAll(prompt, "{{TODO_PROMPT}}", functionTools.GetTodoToolPrompt())
	return prompt
}
