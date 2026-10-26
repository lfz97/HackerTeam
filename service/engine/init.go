package engine

import (
	"HackerTeam/service/engine/config"
	"context"
	"fmt"

	"HackerTeam/service/engine/tools/toolsets"
	ag "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/taskrun/inprocess"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/taskrun"
)

func (e *Engine) newRunner() {
	Runner := runner.NewRunnerWithAgentFactory(
		(*e).Agentname,
		string(operatorName),
		func(ctx context.Context, ro ag.RunOptions) (ag.Agent, error) {
			if err := e.reload(); err != nil {
				return nil, err
			}
			return e.initOperator()
		},
		// Worker 用独立工厂，且不 reload：它返回 reload 已经构造好的那一代实例。
		// 这样 worker 的构造永远发生在 reload 的同一 goroutine 上，spawn 只做一次原子读，
		// 不会出现 N 个并发 spawn 各自重载配置/刷新 MCP。
		runner.WithAgentFactory(string(workerName), func(ctx context.Context, ro ag.RunOptions) (ag.Agent, error) {
			worker := (*e).worker.Load()
			if worker == nil {
				return nil, fmt.Errorf("worker is not initialized")
			}
			return worker, nil
		}),
		runner.WithSessionService((*e).SessionService_p),
		runner.WithMemoryService((*e).SqliteMemoryService),
	)
	(*e).AgentRunner_p = &Agentrunner{
		Runner: Runner,
		Stream: (*(*e).Config_p).Model.Stream,
	}

	// agent 级后台任务：Operator 用它派 Worker，每个 Worker 跑在自己的 child session 里。
	// 必须在 runner 之后构造——Service 直接持有 runner。
	service, err := inprocess.NewService(Runner)
	if err != nil {
		(*e).parkWithFatal(FatalError, fmt.Sprintf("Failed to initialize task run service: %v (press any key to exit)", err), true)
		return
	}
	// 不能挂 turn 的 ctx：turn 结束不该杀掉还在跑的 run。
	service.Start(context.Background())
	tools := taskrun.NewTools(
		service,
		taskrun.WithDefaultAgentName(string(workerName)),
		taskrun.WithSessionService((*e).SessionService_p), // 打开 read_task_run_transcript
	)
	(*e).taskService = service
	(*e).taskTools = &tools
}

// reload 每次 Operator 运行前重新加载配置，并把"这一代配置"派生出的技能索引和
// Worker 一起重建。
//
// Worker 在这里（主 goroutine）构造后原子发布：taskrun 的 worker 工厂只做一次原子读，
// 所以 worker 的构造不会和 Config_p / mcpToolsets 的写入并发。
func (e *Engine) reload() error {
	e.LoadConfig()           //加载配置文件（agent工厂每次重建技能和提示词）
	e.refreshMCPFromConfig() //刷新MCP工具集：增量对齐——同名复用，运行中的stdio子进程/连接不重启

	repo, err := e.skillRepository()
	if err != nil {
		return fmt.Errorf("failed to index skills: %w", err)
	}
	(*e).skillsRepo = repo

	worker, err := e.initWorker()
	if err != nil {
		return fmt.Errorf("failed to build worker: %w", err)
	}
	(*e).worker.Store(worker)
	return nil
}

// skillRepository 把合并后的 Skills 目录建成唯一索引，Operator 与 Worker 共用。
// 目录里不再有角色边界：谁装配都拿到同一份技能集。
func (e *Engine) skillRepository() (*skill.FSRepository, error) {
	return skill.NewFSRepository((*e).SkillsFolderPath)
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
	for _, latestItem := range latestmcps {
		for _, runningItem := range (*e).mcpToolsets {
			if runningItem.Name() == latestItem.Name() {
				common = append(common, runningItem)
			}
		}
	}

	//无共同清单：当前的全关闭，直接以新清单为准。
	//if/else 互斥不能省：common 为空时 else 分支的循环一次都匹配不上，Close 永不发生（泄漏）。
	if len(common) == 0 {
		for _, ts := range (*e).mcpToolsets {
			ts.Close()
		}
		(*e).mcpToolsets = latestmcps
	} else {
		//遍历完共同清单都没匹配上 → 当前清单独有，关闭
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
		//遍历完共同清单都没匹配上 → 新增，追加到 new
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
	c, err := config.LoadConfig((*e).HackerTeamConfigPath)
	if err != nil {
		(*e).parkWithFatal(FatalError, fmt.Sprintf("Failed to load config: %v (press any key to exit)", err), true)
	}
	(*e).Config_p = c
}
