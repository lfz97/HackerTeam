package agent

import (
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"

	"trpc.group/trpc-go/trpc-agent-go/model"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func OpenaiAgent(agentName string, systemPrompt string, genConfig model.GenerationConfig, tools []tool.Tool, toolsets []tool.ToolSet, Model string, BaseUrl string, APIkey string, skillsPath string) *llmagent.LLMAgent {

	agent_p := ConfigBaseAgent(agentName, systemPrompt, genConfig, tools, toolsets, Model, BaseUrl, APIkey, "openai", skillsPath)
	return agent_p
}
