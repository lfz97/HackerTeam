package toolsets

import (
	"time"
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

func StdinMCP(command string, args []string) *mcp.ToolSet {

	mcpToolSet := mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport: "stdio",
			Command:   command,
			Args:      args,
			Timeout:   10 * time.Second,
		},
		mcp.WithSessionReconnect(3),
	)
	return mcpToolSet
}
