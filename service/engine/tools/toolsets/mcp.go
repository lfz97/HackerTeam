package toolsets

import (
	"HackerTeam/service/engine/config"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// HttpMCP 根据配置创建 HTTP 传输（SSE / streamable_http）的 MCP ToolSet。
// WithName 决定工具前缀 {Name}_{toolName}：多个 MCP server 必须各自唯一，
// 否则同名工具会冲突（框架默认名是 "mcp"）。
func HttpMCP(cfg config.HttpMCP) *mcp.ToolSet {
	return mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport:   string(cfg.Type),
			ServerURL:   cfg.Endpoint,
			Timeout:     10 * time.Second,
			Headers:     cfg.Headers,
			Description: cfg.Description,
		},
		mcp.WithName(cfg.Name),
		mcp.WithSessionReconnect(3),
	)
}

// StdinMCP 根据配置创建 stdio 传输的 MCP ToolSet。
// 每个 ToolSet 实例对应一个 MCP server 子进程（懒连接：首次 Tools/调用时启动）。
// HackerTeam 中同一实例共享挂载给全部 agent，因此每个 server 全进程只有一个子进程；
// 生命周期由 Engine 管理（每轮 run 重建前 Close 旧实例，程序退出统一 Close）。
func StdinMCP(cfg config.StdinMCP) *mcp.ToolSet {
	return mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport:   "stdio",
			Command:     cfg.Command,
			Args:        cfg.Args,
			Timeout:     10 * time.Second,
			Description: cfg.Description,
		},
		mcp.WithName(cfg.Name),
		mcp.WithSessionReconnect(3),
	)
}
