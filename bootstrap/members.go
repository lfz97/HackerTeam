package bootstrap

import (
	"HackerTeam/functionTools"
	"HackerTeam/global"
	"HackerTeam/models"
	"HackerTeam/toolsets/localexec"
	"HackerTeam/utils/pretty"

	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// 创建队长agent，负责任务规划、分配和总结，队长只挂载文件目录及文件读写工具
func initCaptain() *llmagent.LLMAgent {
	captainPrompt := assemblePrompt("prompts/agents/captain.md")

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)
	tools = append(tools, global.SqliteMemoryService.Tools()...) // 记忆工具：memory_search / memory_load / memory_add / memory_update / memory_delete（纯agent驱动，无自动提取）

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithTools(tools),                                       // 队长挂载文件系统工具、文件操作工具、日期工具和记忆工具
		llmagent.WithAddSessionSummary(true),                            // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                           //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                      // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                        // 按需加载被压缩的原始数据（session_load）
		llmagent.WithPreloadMemory(10),                                  // 预加载记忆到上下文中
		llmagent.WithGlobalInstruction(captainPrompt),                   // 系统提示词
		//llmagent.WithEnableParallelTools(true),        //队长启用子agent的并行调度能力
	}
	agent_p := setAgent("Captain", opts)
	return agent_p

}

// 侦察agent，负责信息收集和环境侦察，挂载相关技能库和工具
func initRecon() *llmagent.LLMAgent {

	reconPrompt := assemblePrompt("prompts/agents/recon.md")
	repo, _ := skill.NewFSRepository(global.ReconSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                          //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithGlobalInstruction(reconPrompt),
		llmagent.WithTools(tools),
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())), //侦察员挂载LocalExec工具集，包含本地命令执行工具
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p := setAgent("Recon", opts)
	return agent_p
}

// 渗透agent，负责漏洞利用和权限提升，挂载相关技能库和工具
func initexploit() *llmagent.LLMAgent {

	exploitPrompt := assemblePrompt("prompts/agents/exploit.md")
	repo, _ := skill.NewFSRepository(global.ExploitSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                          //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithGlobalInstruction(exploitPrompt),                  // 系统提示词
		llmagent.WithTools(tools),
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p := setAgent("Exploit", opts)
	return agent_p

}

// 后渗透agent，负责权限维持、横向移动和痕迹清除，挂载相关技能库和工具
func initpostexploit() *llmagent.LLMAgent {

	postexploitPrompt := assemblePrompt("prompts/agents/post_exploit.md")
	repo, _ := skill.NewFSRepository(global.PostExploitSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                          //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithGlobalInstruction(postexploitPrompt),              // 系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p := setAgent("PostExploit", opts)
	return agent_p
}

// 扫描agent，负责漏洞扫描和安全评估，挂载相关技能库和工具
func initScanner() *llmagent.LLMAgent {

	scannerPrompt := assemblePrompt("prompts/agents/scanner.md")
	repo, _ := skill.NewFSRepository(global.ScannerSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                          //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithGlobalInstruction(scannerPrompt),                  // 系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p := setAgent("Scanner", opts)
	return agent_p
}

// 复现agent，负责漏洞复现和验证，挂载相关技能库和工具
func initReproducer() *llmagent.LLMAgent {
	reproducerPrompt := assemblePrompt("prompts/agents/reproducer.md")
	repo, _ := skill.NewFSRepository(global.ReproducerSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()
	datetools := functionTools.GetDateTools()

	tools := append(systemtools, operationtools...)
	tools = append(tools, datetools...)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*global.Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           // 启用上下文压缩注入
		llmagent.WithSyncSummaryIntraRun(true),                          //在同一次对话中同步更新摘要
		llmagent.WithEnableContextCompaction(true),                     // 启用 tool result 压缩（Pass 1+2）
		llmagent.WithContextCompactionOversizedToolResultMaxTokens(8192), // Pass 2: 超大 tool result 首尾保留截断
		llmagent.WithEnableOnDemandSession(true),                       // 按需加载被压缩的原始数据（session_load）
		llmagent.WithGlobalInstruction(reproducerPrompt),               // 系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
		llmagent.WithTools(tools),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
	}
	agent_p := setAgent("Reproducer", opts)
	return agent_p
}

func setAgent(agentName string, opts []llmagent.Option) *llmagent.LLMAgent {
	if (*global.Config_p).Model.APIType == "openai" {
		openaimodel := models.Openai((*global.Config_p).Model.Model,
			(*global.Config_p).Model.BaseURL,
			(*global.Config_p).Model.APIKey)
		opts = append(opts, llmagent.WithModel(openaimodel))
		Agent_p := llmagent.New(agentName, opts...)
		return Agent_p

	} else if (*global.Config_p).Model.APIType == "anthropic" {
		Anthropicagent := models.Anthropic((*global.Config_p).Model.Model,
			(*global.Config_p).Model.BaseURL,
			(*global.Config_p).Model.APIKey)
		opts = append(opts, llmagent.WithModel(Anthropicagent))
		Agent_p := llmagent.New(agentName, opts...)
		return Agent_p

	} else {
		pretty.ErrorWithExit("不支持的API类型，请检查配置文件中的 Model.APIType 字段")
		return nil
	}
}

// 组装各个agent的提示词
func assemblePrompt(path string) string {
	prompt_b, _ := global.PromptFiles.ReadFile(path)
	prompt := string(prompt_b)
	prompt = strings.ReplaceAll(prompt, "{{ENV}}", global.EnvPrompt)
	prompt = strings.ReplaceAll(prompt, "{{COMMAND_EXECUTION}}", global.CommandExecutionPrompt)
	prompt = strings.ReplaceAll(prompt, "{{VULN_CONSENSUS}}", global.VulnConsensusPrompt)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUT_CONSENSUS}}", global.OutputConsensusPrompt)
	return prompt
}
