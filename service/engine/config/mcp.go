package config

// MCPtype HTTP 类 MCP 传输类型
type MCPtype string

const (
	SSE             MCPtype = "sse"
	Streamable_HTTP MCPtype = "streamable_http"
)

// HttpMCP HTTP 传输（SSE / streamable_http）的 MCP server 配置
type HttpMCP struct {
	Enabled     bool
	Name        string // 决定工具前缀 {Name}_{toolName}，多个 MCP server 间必须唯一
	Type        MCPtype
	Endpoint    string
	Headers     map[string]string
	Description string
	// Timeout 单次工具调用的超时，单位/秒（沿用 model.httptimeout 惯例）。
	// 长分析型 server 务必调大；<=0 用默认值 defaultMCPTimeout。
	Timeout int
}

// StdinMCP stdio 传输的 MCP server 配置。
// 每个 ToolSet 实例对应一个子进程；HackerTeam 中同一实例共享给全部 agent，
// 因此每个 server 全进程只有一个子进程。
type StdinMCP struct {
	Enabled     bool
	Name        string // 决定工具前缀 {Name}_{toolName}，多个 MCP server 间必须唯一
	Command     string
	Args        []string
	Description string
	// Timeout 同 HttpMCP.Timeout
	Timeout int
}
