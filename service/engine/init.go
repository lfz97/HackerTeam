package engine

import (
	"HackerTeam/service/engine/config"
	"context"
	"fmt"

	"HackerTeam/service/engine/tools/toolsets"
	ag "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func (e *Engine) newRunner() {
	Runner := runner.NewRunnerWithAgentFactory(
		(*e).Agentname,
		(*e).Agentname,
		func(ctx context.Context, ro ag.RunOptions) (ag.Agent, error) {
			e.reload()
			captain, err := e.InitTeam()
			if err != nil {
				return nil, err
			}
			return captain, nil
		},
		runner.WithSessionService((*e).SessionService_p),
		runner.WithMemoryService((*e).SqliteMemoryService),
	)
	(*e).AgentRunner_p = &Agentrunner{
		Runner: Runner,
		Stream: (*(*e).Config_p).Model.Stream,
	}
}

// 每次runner执行时重新加载以下Module
func (e *Engine) reload() {
	e.LoadConfig()           //加载配置文件（agent工厂每次重建技能和提示词）
	e.refreshMCPFromConfig() //刷新MCP工具集：Close上一轮的连接/子进程，按最新配置重建
}

// loadMCPFromConfig 从配置文件创建启用的 MCP ToolSet，追加到 mcpToolsets。
// 未配置 Name 的 server 分配默认名，避免工具前缀冲突。
func (e *Engine) loadMCPFromConfig() {
	idx := 0
	for _, mcpConfig := range (*(*e).Config_p).HttpMcp {
		if mcpConfig.Enabled == true {
			if mcpConfig.Name == "" {
				mcpConfig.Name = fmt.Sprintf("mcp_%d", idx)
			}
			(*e).mcpToolsets = append((*e).mcpToolsets, toolsets.HttpMCP(mcpConfig))
			idx++
		}
	}
	for _, stdinMcpConfig := range (*(*e).Config_p).StdinMcp {
		if stdinMcpConfig.Enabled == true {
			if stdinMcpConfig.Name == "" {
				stdinMcpConfig.Name = fmt.Sprintf("mcp_%d", idx)
			}
			(*e).mcpToolsets = append((*e).mcpToolsets, toolsets.StdinMCP(stdinMcpConfig))
			idx++
		}
	}
}

// refreshMCPFromConfig 每轮run刷新MCP工具集：先Close上一轮的实例（释放stdio子进程与连接），
// 再按最新配置重建。MCP实例跨agent共享——同一指针挂给全部agent，
// 每个server全进程只有一个子进程。内置工具/工具集不在此刷新（启动时建立，跨轮常驻）。
func (e *Engine) refreshMCPFromConfig() {
	for _, ts := range (*e).mcpToolsets {
		ts.Close()
	}
	(*e).mcpToolsets = []tool.ToolSet{}
	e.loadMCPFromConfig()
}

func (e *Engine) LoadConfig() {
	//加载配置文件
	c, err := config.LoadConfig((*e).HackerTeamConfigPath)
	if err != nil {
		(*e).parkWithFatal(FatalError, fmt.Sprintf("加载配置文件错误: %v,按任意键退出", err), true)
	}
	(*e).Config_p = c
}
