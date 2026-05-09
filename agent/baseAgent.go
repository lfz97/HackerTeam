package agent

import (
	"HyperBot/models"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func ConfigBaseAgent(agentName string, systemPrompt string, genConfig model.GenerationConfig, tools []tool.Tool, toolsets []tool.ToolSet, Model string, BaseUrl string, APIkey string, ApiType string, skillsPath string) *llmagent.LLMAgent {

	repo, _ := skill.NewFSRepository(skillsPath)

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(genConfig),
		llmagent.WithTools(tools),
		llmagent.WithGlobalInstruction(systemPrompt), //系统提示词
		llmagent.WithToolSets(toolsets),
		llmagent.WithRefreshToolSetsOnRun(true),
		llmagent.WithSkillsLoadedContentInToolResults(true),
		//仅注入知识，不注入执行工具的能力，统一通过localexec执行
		llmagent.WithSkills(repo),
		llmagent.WithSkillToolProfile(
			llmagent.SkillToolProfileKnowledgeOnly,
		),
		llmagent.WithAddSessionSummary(true), //启用上下文压缩注入

	}

	if ApiType == "openai" {
		OpenaiModel_p := models.Openai(Model, BaseUrl, APIkey)
		opts = append(opts, llmagent.WithModel(OpenaiModel_p))
	} else if ApiType == "anthropic" {
		AnthropicModel_p := models.Anthropic(Model, BaseUrl, APIkey)
		opts = append(opts, llmagent.WithModel(AnthropicModel_p))
	}

	agent_p := llmagent.New(agentName,
		opts...,
	)
	return agent_p

}
