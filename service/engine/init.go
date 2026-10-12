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
	e.refreshMCPFromConfig() //刷新MCP工具集：增量对齐——同名复用，运行中的stdio子进程/连接不重启
}

// loadMCPFromConfig 从配置文件创建启用的 MCP ToolSet，返回期望运行的新清单。
// 全部新建但惰性未连接（首次 Tools() 才连接），未配置 Name 的 server
// 分配默认名，避免工具前缀冲突。
func (e *Engine) loadMCPFromConfig() []tool.ToolSet {
	latestmcps := []tool.ToolSet{}
	idx := 0
	for _, mcpConfig := range (*(*e).Config_p).HttpMcp {
		if mcpConfig.Enabled == true {
			if mcpConfig.Name == "" {
				mcpConfig.Name = fmt.Sprintf("mcp_%d", idx)
			}
			latestmcps = append(latestmcps, toolsets.HttpMCP(mcpConfig))
			idx++
		}
	}
	for _, stdinMcpConfig := range (*(*e).Config_p).StdinMcp {
		if stdinMcpConfig.Enabled == true {
			if stdinMcpConfig.Name == "" {
				stdinMcpConfig.Name = fmt.Sprintf("mcp_%d", idx)
			}
			latestmcps = append(latestmcps, toolsets.StdinMCP(stdinMcpConfig))
			idx++
		}
	}
	return latestmcps
}

// diffMCPtoolsets 增量对齐 MCP 工具集：同名复用（stdio 子进程/HTTP 连接不重启）、
// 新增启用、消失关闭。不比较配置内容——同 name 改 endpoint/args 不生效，
// 需改名或先禁用再启用触发重启（行为契约由 mcp_test.go 锁定）。
// MCP实例跨agent共享——同一指针挂给全部agent，每个server全进程只有一个子进程。
func (e *Engine) diffMCPtoolsets(latestmcps []tool.ToolSet) {

	common := []tool.ToolSet{}
	new := []tool.ToolSet{}
	//先取新旧的共同清单
	for _, latestItem := range latestmcps {
		for _, runningItem := range (*e).mcpToolsets {
			if runningItem.Name() == latestItem.Name() {
				common = append(common, runningItem)
			}
		}
	}

	//如果无共同清单，直接以新的清单为准，当前的全关闭
	//注意 if/else 互斥不能省：common 为空时下面两段循环体一次都不会执行
	//（Close 永不发生 → 泄漏；new 组装不出任何东西 → 空清单）
	if len(common) == 0 {
		for _, ts := range (*e).mcpToolsets {
			ts.Close()
		}
		(*e).mcpToolsets = latestmcps

		//有共同清单的情况
	} else {
		//用当前的清单和共同清单做对比，匹配到同名的就break；遍历到最后都没匹配上，
		//说明是当前清单独有的，直接关闭
		for _, running := range (*e).mcpToolsets {
			for i := 0; i < len(common); i++ {
				if common[i].Name() == running.Name() {
					break
				}
				if i == len(common)-1 {
					running.Close()
				}
			}
		}
		//用最新的清单和共同清单做对比，匹配到同名的就break；遍历到最后都没匹配上，
		//说明是新增的，追加到new
		for _, latest := range latestmcps {
			for i := 0; i < len(common); i++ {
				if common[i].Name() == latest.Name() {
					break
				}
				if i == len(common)-1 {
					new = append(new, latest)
				}
			}
		}
		//最终清单是新增+交集
		new = append(new, common...)
		(*e).mcpToolsets = new
	}
}

func (e *Engine) refreshMCPFromConfig() {
	//loadMCPFromConfig 新造的实例全是惰性的（未连接），diff 中未被启用的
	//直接丢弃即可，无需 Close；Close 只发生在旧实例离开的分支。
	e.diffMCPtoolsets(e.loadMCPFromConfig())
}

func (e *Engine) LoadConfig() {
	//加载配置文件
	c, err := config.LoadConfig((*e).HackerTeamConfigPath)
	if err != nil {
		(*e).parkWithFatal(FatalError, fmt.Sprintf("加载配置文件错误: %v,按任意键退出", err), true)
	}
	(*e).Config_p = c
}
