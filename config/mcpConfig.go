package config

type MCPtype string

const (
	SSE             MCPtype = "sse"
	Streamable_HTTP MCPtype = "streamable_http"
)

type MCP struct {
	Enabled  bool
	Type     MCPtype
	Endpoint string
	Headers  map[string]string
}
