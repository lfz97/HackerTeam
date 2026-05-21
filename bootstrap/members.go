package bootstrap

import (
	"HackerTeam/functionTools"
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

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithTools(append(systemtools, operationtools...)),
		llmagent.WithAddSessionSummary(true),          //启用上下文压缩注入
		llmagent.WithGlobalInstruction(captainPrompt), //系统提示词
		//llmagent.WithEnableParallelTools(true),        //队长启用子agent的并行调度能力
	}
	agent_p := setAgent("Captain", opts)
	return agent_p

}

func initRecon() *llmagent.LLMAgent {

	reconPrompt := assemblePrompt("prompts/agents/recon.md")
	repo, _ := skill.NewFSRepository(ReconSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true), //启用上下文压缩注入
		llmagent.WithGlobalInstruction(reconPrompt),
		llmagent.WithTools(append(systemtools, operationtools...)),     //系统提示词
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

func initexploit() *llmagent.LLMAgent {

	exploitPrompt := assemblePrompt("prompts/agents/exploit.md")
	repo, _ := skill.NewFSRepository(ExploitSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),          //启用上下文压缩注入
		llmagent.WithGlobalInstruction(exploitPrompt), //系统提示词
		llmagent.WithTools(append(systemtools, operationtools...)),
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

func initpostexploit() *llmagent.LLMAgent {

	postexploitPrompt := assemblePrompt("prompts/agents/post_exploit.md")
	repo, _ := skill.NewFSRepository(PostExploitSkillsFolderPath)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),              //启用上下文压缩注入
		llmagent.WithGlobalInstruction(postexploitPrompt), //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
		llmagent.WithTools(append(systemtools, operationtools...)),
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

func initScanner() *llmagent.LLMAgent {

	scannerPrompt := assemblePrompt("prompts/agents/scanner.md")
	repo, _ := skill.NewFSRepository(ScannerSkillsFolderPath)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),          //启用上下文压缩注入
		llmagent.WithGlobalInstruction(scannerPrompt), //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
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

func setAgent(agentName string, opts []llmagent.Option) *llmagent.LLMAgent {
	if (*Config_p).Model.APIType == "openai" {
		openaimodel := models.Openai((*Config_p).Model.Model,
			(*Config_p).Model.BaseURL,
			(*Config_p).Model.APIKey)
		opts = append(opts, llmagent.WithModel(openaimodel))
		Agent_p := llmagent.New(agentName, opts...)
		return Agent_p

	} else if (*Config_p).Model.APIType == "anthropic" {
		Anthropicagent := models.Anthropic((*Config_p).Model.Model,
			(*Config_p).Model.BaseURL,
			(*Config_p).Model.APIKey)
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
	prompt_b, _ := PromptFiles.ReadFile(path)
	prompt := string(prompt_b)
	prompt = strings.ReplaceAll(prompt, "{{ENV}}", envPrompt)
	prompt = strings.ReplaceAll(prompt, "{{COMMAND_EXECUTION}}", commandExecutionPrompt)
	prompt = strings.ReplaceAll(prompt, "{{VULN_CONSENSUS}}", vulnConsensusPrompt)
	prompt = strings.ReplaceAll(prompt, "{{OUTPUT_CONSENSUS}}", outputConsensusPrompt)
	return prompt
}
