package toolsets

import (
	"time"
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

func MCP(mcptype string, url string, headers map[string]string) *mcp.ToolSet {

	mcpToolSet := mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport: mcptype, // 注意：使用完整名称
			ServerURL: url,
			Timeout:   10 * time.Second,
			Headers:   headers,
		},
		mcp.WithSessionReconnect(3),
	)
	return mcpToolSet
}
