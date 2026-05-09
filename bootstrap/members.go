package bootstrap

import (
	"HackerTeam/functionTools"
	"HackerTeam/models"
	"HackerTeam/toolsets/localexec"

	"HackerTeam/utils/pretty"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"

	"fmt"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// 创建队长agent，负责任务规划、分配和总结，队长只挂载文件目录及文件读写工具
func initCaptain() *llmagent.LLMAgent {
	captainPromptBytes, err := PromptFiles.ReadFile("prompt/captain.md")
	if err != nil {
		pretty.ErrorWithExit(fmt.Sprintf("读取队长提示词失败: %v", err))
	}
	captainPrompt := string(captainPromptBytes)
	captainPrompt = strings.ReplaceAll(captainPrompt, "{{ENV}}", envPrompt)

	systemtools := functionTools.GetFileSystemTools()
	operationtools := functionTools.GetFileOperationsTools()

	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithTools(append(systemtools, operationtools...)),
		llmagent.WithAddSessionSummary(true),          //启用上下文压缩注入
		llmagent.WithGlobalInstruction(captainPrompt), //系统提示词
	}
	agent_p := setAgent("Captain", opts)
	return agent_p

}

func initRecon() *llmagent.LLMAgent {
	reconPromptBytes, err := PromptFiles.ReadFile("prompt/recon.md")
	if err != nil {
		pretty.ErrorWithExit(fmt.Sprintf("读取侦察员提示词失败: %v", err))
	}
	reconPrompt := string(reconPromptBytes)
	reconPrompt = strings.ReplaceAll(reconPrompt, "{{ENV}}", envPrompt)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),                           //启用上下文压缩注入
		llmagent.WithGlobalInstruction(reconPrompt),                    //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())), //侦察员挂载LocalExec工具集，包含本地命令执行工具
	}
	agent_p := setAgent("Recon", opts)
	return agent_p
}

func initexploit() *llmagent.LLMAgent {
	exploitPromptBytes, err := PromptFiles.ReadFile("prompt/exploit.md")
	if err != nil {
		pretty.ErrorWithExit(fmt.Sprintf("读取攻击者提示词失败: %v", err))
	}
	exploitPrompt := string(exploitPromptBytes)
	exploitPrompt = strings.ReplaceAll(exploitPrompt, "{{ENV}}", envPrompt)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),          //启用上下文压缩注入
		llmagent.WithGlobalInstruction(exploitPrompt), //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
	}
	agent_p := setAgent("Exploit", opts)
	return agent_p

}

func initpostexploit() *llmagent.LLMAgent {
	postexploitPromptBytes, err := PromptFiles.ReadFile("prompt/post_exploit.md")
	if err != nil {
		pretty.ErrorWithExit(fmt.Sprintf("读取后渗透者提示词失败: %v", err))
	}
	postexploitPrompt := string(postexploitPromptBytes)
	postexploitPrompt = strings.ReplaceAll(postexploitPrompt, "{{ENV}}", envPrompt)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),              //启用上下文压缩注入
		llmagent.WithGlobalInstruction(postexploitPrompt), //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
	}
	agent_p := setAgent("PostExploit", opts)
	return agent_p
}

func initvulnanalyst() *llmagent.LLMAgent {
	vulnanalystPromptBytes, err := PromptFiles.ReadFile("prompt/vuln_analyst.md")
	if err != nil {
		pretty.ErrorWithExit(fmt.Sprintf("读取漏洞分析师提示词失败: %v", err))
	}
	vulnanalystPrompt := string(vulnanalystPromptBytes)
	vulnanalystPrompt = strings.ReplaceAll(vulnanalystPrompt, "{{ENV}}", envPrompt)

	toolsets := []tool.ToolSet{}
	opts := []llmagent.Option{
		llmagent.WithGenerationConfig(model.GenerationConfig{
			Stream: (*Config_p).Model.Stream,
		}),
		llmagent.WithAddSessionSummary(true),              //启用上下文压缩注入
		llmagent.WithGlobalInstruction(vulnanalystPrompt), //系统提示词
		llmagent.WithToolSets(append(toolsets, localexec.LocalExec())),
	}
	agent_p := setAgent("VulnAnalyst", opts)
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
