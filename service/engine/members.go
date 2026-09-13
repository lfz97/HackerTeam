package engine

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/models"
	functionTools "HackerTeam/service/engine/tools/functions"
	"errors"
	"fmt"
	"strings"

	"context"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	agenttool "trpc.group/trpc-go/trpc-agent-go/tool/agent"
)

type agentName string

var (
	captain     agentName = "Captain"
	recon       agentName = "Recon"
	exploit     agentName = "Exploit"
	postexploit agentName = "PostExploit"
	scanner     agentName = "Scanner"
	reproducer  agentName = "Reproducer"
)

func (e *Engine) InitTeam() (*llmagent.LLMAgent, error) {
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

	subagents := []*llmagent.LLMAgent{exploit, postexploit, recon, scanner, reproducer}
	subagentTools := []tool.Tool{}
	for _, agent := range subagents {
		subagentTools = append(subagentTools, agenttool.NewTool(
			agent,
			agenttool.WithStreamInner(true), // 开启：把子 Agent 的流式事件转发给父流程
			agenttool.WithInnerTextMode(agenttool.InnerTextModeInclude), //展示子agent完整transcript(正文+tool call+tool result)
			agenttool.WithDescription(agent.Info().Description),
			agenttool.WithPersistentHistory(),                          //在 HistoryScopeIsolated 下使用稳定的子 FilterKey，让子 Agent 能在同一个 session 内跨多次 AgentTool 调用读取自己的历史（而不是每次都从“全新子 key”开始）
			agenttool.WithHistoryScope(agenttool.HistoryScopeIsolated), //子调用使用独立 FilterKey，通常只读取本次工具参数，不继承父历史
		))
	}
	toolCallbacks := tool.NewCallbacks().RegisterAfterTool(afterToolCallback)
	captain, err := e.initCaptain(subagentTools, toolCallbacks)
	if err != nil {
		return nil, err
	}
	return captain, nil
}

func afterToolCallback(ctx context.Context, args *tool.AfterToolArgs) (*tool.AfterToolResult, error) {
	if args.ToolName == string(recon) || args.ToolName == string(exploit) || args.ToolName == string(postexploit) || args.ToolName == string(scanner) || args.ToolName == string(reproducer) {
		if args.Error != nil {
			return nil, nil // 真失败放行：框架沿用原始 result+err 走原有错误路径
		}

		if args.Result == nil {
			return &tool.AfterToolResult{
				CustomResult: fmt.Sprintf("[AGENT %s returned EMPTY RESPONSE - likely a model "+"generation failure (empty content, no toolcalls). Task NOT completed. "+"This is usually transient: re-dispatch the SAME request up to 2 more times. "+"If it fails repeatedly, switch approach or report failure explicitly.]", args.ToolName),
			}, nil
		}
		s, ok := args.Result.(string)
		if ok {
			if strings.TrimSpace(s) == "" {
				return &tool.AfterToolResult{
					CustomResult: fmt.Sprintf("[AGENT %s returned EMPTY RESPONSE - likely a model "+"generation failure (empty content, no toolcalls). Task NOT completed. "+"This is usually transient: re-dispatch the SAME request up to 2 more times. "+"If it fails repeatedly, switch approach or report failure explicitly.]", args.ToolName),
				}, nil
			}
		}
	}

	return nil, nil
}

// 创建队长agent，负责任务规划、分配和总结，队长挂载文件目录、文件读写工具与todo清单
func (e *Engine) initCaptain(subagentTools []tool.Tool, toolCallbacks *tool.Callbacks) (*llmagent.LLMAgent, error) {
	captainPrompt := e.assemblePrompt("prompts/agents/captain.md")

	// 内置工具清单常驻（启动时建一次），拷入私有slice再追加记忆工具，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools)+5)
	tools = append(tools, (*e).builtinTools...)
	tools = append(tools, (*e).SqliteMemoryService.Tools()...)           // 记忆工具：除 memory_clear 外全部暴露，后台 extractor 自动提取 + agent 手动增删改查
	tools = append(tools, functionTools.GetTodoTools()...)               // 框架内置 todo_write：任务清单仅队长持有，子Agent不共享（避免各自写乱计划）
	tools = append(tools, subagentTools...)
	// 配置文件声明的MCP工具集：挂载给全部agent（含队长），每轮run自动刷新
	toolsets := make([]tool.ToolSet, 0, len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).mcpToolsets...)

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithTools(tools),                                        // 队长挂载内置文件/日期工具 + 记忆工具
		llmagent.WithToolSets(toolsets),                                  // 配置文件声明的MCP工具集
		llmagent.WithRefreshToolSetsOnRun(true),                          // 每次run刷新MCP工具列表
		llmagent.WithAddSessionSummary(true),                             // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                           //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                       // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                         // 按需加载被压缩的原始数据（session_load）
		llmagent.WithPreloadMemory(10),                                   // 预加载记忆到上下文中
		llmagent.WithGlobalInstruction(captainPrompt),                    // 系统提示词
		llmagent.WithToolCallbacks(toolCallbacks),
		//llmagent.WithEnableParallelTools(true),        //队长启用子agent的并行调度能力
	}
	agent_p, err := setAgent(captain, (*(*e).Config_p).Model, opts, (*e).tui)
	return agent_p, err

}

// 侦察agent，负责信息收集和环境侦察，挂载相关技能库和工具
func (e *Engine) initRecon() (*llmagent.LLMAgent, error) {

	reconPrompt := e.assemblePrompt("prompts/agents/recon.md")
	repo, _ := skill.NewFSRepository((*e).ReconSkillsFolderPath)

	// 内置工具清单常驻（启动时建一次），拷入私有slice再挂载，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools))
	tools = append(tools, (*e).builtinTools...)

	// 本角色常驻localexec（jobs跨轮续查）+ 配置文件MCP工具集（全体agent共享）
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).builtinToolsets["Recon"])
	toolsets = append(toolsets, (*e).mcpToolsets...)
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                                           // 启用上下文压缩注入
		llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser), //摘要注入到user message，不与system prompt中的SOP规则竞争优先级
		llmagent.WithSyncSummaryIntraRun(true),                                         //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),               // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithDescription("Recon Agent — information gathering ONLY. Use for reconnaissance: subdomain enumeration, DNS records, port scanning, service/OS fingerprinting, web tech-stack and sensitive-path discovery, passive intel (Shodan/Fofa/WHOIS/GitHub leaks). Does NOT scan for vulnerabilities or exploit. Dispatch by passing the task in the `request` field."),
		llmagent.WithGlobalInstruction(reconPrompt),
		llmagent.WithTools(tools),
		llmagent.WithToolSets(toolsets), // 本角色常驻localexec + 配置文件MCP工具集
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p, err := setAgent(recon, (*(*e).Config_p).Model, opts, nil)
	return agent_p, err
}

// 渗透agent，负责漏洞利用和权限提升，挂载相关技能库和工具
func (e *Engine) initexploit() (*llmagent.LLMAgent, error) {

	exploitPrompt := e.assemblePrompt("prompts/agents/exploit.md")
	repo, _ := skill.NewFSRepository((*e).ExploitSkillsFolderPath)

	// 内置工具清单常驻（启动时建一次），拷入私有slice再挂载，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools))
	tools = append(tools, (*e).builtinTools...)

	// 本角色常驻localexec（jobs跨轮续查）+ 配置文件MCP工具集（全体agent共享）
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).builtinToolsets["Exploit"])
	toolsets = append(toolsets, (*e).mcpToolsets...)
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                                           // 启用上下文压缩注入
		llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser), //摘要注入到user message，不与system prompt中的SOP规则竞争优先级
		llmagent.WithSyncSummaryIntraRun(true),                                         //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),               // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithDescription("Exploit Agent — hands-on verification, controlled exploitation, and bounded general execution. Pentest mode requires target context plus a concrete finding or attack hypothesis. General Execution / CTF mode instead requires a bounded objective, supplied artifacts or endpoints, and an explicit success condition; it does not require Recon/Scanner reports or a vulnerability hypothesis. Use this fallback only when no other specialist fits. Dispatch by passing the task in the `request` field."),
		llmagent.WithGlobalInstruction(exploitPrompt), // 系统提示词
		llmagent.WithTools(tools),
		llmagent.WithToolSets(toolsets),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p, err := setAgent(exploit, (*(*e).Config_p).Model, opts, nil)
	return agent_p, err

}

// 后渗透agent，负责权限维持、横向移动和痕迹清除，挂载相关技能库和工具
func (e *Engine) initpostexploit() (*llmagent.LLMAgent, error) {

	postexploitPrompt := e.assemblePrompt("prompts/agents/post_exploit.md")
	repo, _ := skill.NewFSRepository((*e).PostExploitSkillsFolderPath)

	// 内置工具清单常驻（启动时建一次），拷入私有slice再挂载，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools))
	tools = append(tools, (*e).builtinTools...)

	// 本角色常驻localexec（jobs跨轮续查）+ 配置文件MCP工具集（全体agent共享）
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).builtinToolsets["PostExploit"])
	toolsets = append(toolsets, (*e).mcpToolsets...)
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                                           // 启用上下文压缩注入
		llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser), //摘要注入到user message，不与system prompt中的SOP规则竞争优先级
		llmagent.WithSyncSummaryIntraRun(true),                                         //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),               // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithDescription("PostExploit Agent — one-objective post-exploitation from existing valid access. Use only when a session/credential/foothold already exists, and assign one explicit objective at a time: privilege escalation assessment, scoped internal recon, approved credential/data collection, authorized lateral movement, persistence, exfiltration, or cleanup. Provide current session/privilege info and the single authorized objective in the task. Dispatch by passing the task in the `request` field."),
		llmagent.WithGlobalInstruction(postexploitPrompt), // 系统提示词
		llmagent.WithToolSets(toolsets),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p, err := setAgent(postexploit, (*(*e).Config_p).Model, opts, nil)
	return agent_p, err
}

// 扫描agent，负责漏洞扫描和安全评估，挂载相关技能库和工具
func (e *Engine) initScanner() (*llmagent.LLMAgent, error) {

	scannerPrompt := e.assemblePrompt("prompts/agents/scanner.md")
	repo, _ := skill.NewFSRepository((*e).ScannerSkillsFolderPath)

	// 内置工具清单常驻（启动时建一次），拷入私有slice再挂载，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools))
	tools = append(tools, (*e).builtinTools...)

	// 本角色常驻localexec（jobs跨轮续查）+ 配置文件MCP工具集（全体agent共享）
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).builtinToolsets["Scanner"])
	toolsets = append(toolsets, (*e).mcpToolsets...)
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                                           // 启用上下文压缩注入
		llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser), //摘要注入到user message，不与system prompt中的SOP规则竞争优先级
		llmagent.WithSyncSummaryIntraRun(true),                                         //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),               // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithDescription("Scanner Agent — automated vulnerability scanning ONLY (breadth over accuracy). Use for nuclei, sqlmap --batch detection, nikto, and non-invasive service/path-scoped vulnerability checks against concrete targets. Does NOT do asset discovery, tech/WAF fingerprinting, directory enumeration, weak-credential attempts, verification, rating, or exploitation. Dispatch by passing the task in the `request` field."),
		llmagent.WithGlobalInstruction(scannerPrompt), // 系统提示词
		llmagent.WithToolSets(toolsets),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p, err := setAgent(scanner, (*(*e).Config_p).Model, opts, nil)
	return agent_p, err
}

// 复现agent，负责生成漏洞复现脚本并进行离线语法校验，挂载相关技能库和工具
func (e *Engine) initReproducer() (*llmagent.LLMAgent, error) {
	reproducerPrompt := e.assemblePrompt("prompts/agents/reproducer.md")
	repo, _ := skill.NewFSRepository((*e).ReproducerSkillsFolderPath)

	// 内置工具清单常驻（启动时建一次），拷入私有slice再挂载，避免与Engine共享列表产生append别名
	tools := make([]tool.Tool, 0, len((*e).builtinTools))
	tools = append(tools, (*e).builtinTools...)

	// 本角色常驻localexec（jobs跨轮续查）+ 配置文件MCP工具集（全体agent共享）
	toolsets := make([]tool.ToolSet, 0, 1+len((*e).mcpToolsets))
	toolsets = append(toolsets, (*e).builtinToolsets["Reproducer"])
	toolsets = append(toolsets, (*e).mcpToolsets...)
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			MaxTokens: &(*(*e).Config_p).Model.MaxTokens, // 最大生成 token 数，来自配置 maxtokens 字段
			Stream:    (*(*e).Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                                           // 启用上下文压缩注入
		llmagent.WithSessionSummaryInjectionMode(llmagent.SessionSummaryInjectionUser), //摘要注入到user message，不与system prompt中的SOP规则竞争优先级
		llmagent.WithSyncSummaryIntraRun(true),                                         //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192),               // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithDescription("Reproducer Agent — generates standalone Python PoC/Exploit scripts from reviewed vulnerability evidence. Provide MD report paths, raw-output paths, or other Captain-reviewed evidence in prior_results so it can extract complete structured vulnerability data. Never attacks targets. Dispatch by passing the task in the `request` field."),
		llmagent.WithGlobalInstruction(reproducerPrompt), // 系统提示词
		llmagent.WithToolSets(toolsets),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p, err := setAgent(reproducer, (*(*e).Config_p).Model, opts, nil)
	return agent_p, err
}

// setAgent 根据 APIType 创建对应模型的 agent（无状态，包级函数）。
// sink 用于把 TodoBar 文本推给 TUI：只有 Captain（主 agent，与用户对话、持有 todo 清单）
// 传 (*e).tui；五个子 agent 传 nil —— 子 agent 按 branch 读不到清单，每跳推空串会把
// TodoBar 清空再由 Captain 推回造成闪烁，且它们本就非交互。
func setAgent(agentName agentName, m config.Model, opts []llmagent.Option, sink todoTextSink) (*llmagent.LLMAgent, error) {
	opts = append(opts, setBeforeModelStatusCallback(sink)) //设置beforeModel状态栏
	if m.APIType == "openai" {
		openaimodel := models.Openai(m)
		opts = append(opts, llmagent.WithModel(openaimodel))
		Agent_p := llmagent.New(string(agentName), opts...)
		return Agent_p, nil

	} else if m.APIType == "anthropic" {

		Anthropicagent := models.Anthropic(m)
		opts = append(opts, llmagent.WithModel(Anthropicagent))
		Agent_p := llmagent.New(string(agentName), opts...)
		return Agent_p, nil

	} else {
		return nil, errors.New("不支持的API类型，请检查配置文件中的 Model.APIType 字段")
	}
}

// 组装各个agent的提示词
func (e *Engine) assemblePrompt(path string) string {
	prompt_b, _ := PromptFiles.ReadFile(path)
	prompt := string(prompt_b)
	prompt = strings.ReplaceAll(prompt, "{{ENV}}", (*e).EnvPrompt)
	prompt = strings.ReplaceAll(prompt, "{{COMMAND_EXECUTION}}", (*e).CommandExecutionPrompt)
	prompt = strings.ReplaceAll(prompt, "{{VULN_CONSENSUS}}", (*e).VulnConsensusPrompt)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUT_CONSENSUS}}", (*e).OutputConsensusPrompt)
	//todo_write 工具使用说明（框架 tool/todo.DefaultToolPrompt，随框架版本走，不在提示词里硬编码）
	prompt = strings.ReplaceAll(prompt, "{{TODO_PROMPT}}", functionTools.GetTodoToolPrompt())
	return prompt
}
