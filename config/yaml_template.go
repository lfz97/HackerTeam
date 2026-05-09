package config

const Template string = `#用户配置
user:
  userid: "{USERID}"

# 模型配置
model:
  model: "deepseek-reasoner"
  baseurl: "https://api.deepseek.com"
  apikey: "your-api-key"
  apitype: "openai" # openai 或者 anthropic
  contextwindow: 64000 # 上下文窗口大小，请参考模型文档设置，影响自动摘要功能的触发时机
  stream: true # 是否开启流式输出，开启后可以实时看到模型的推理过程和工具调用信息

# MCP 服务配置
# Type 可选: "sse" 或 "streamable_http"
mcp:
  - name: "mcpexec"
    enabled: true
    type: "sse"
    endpoint: "http://127.0.0.1:8080/mcp"
    headers: {}  # 可选，如: {"Authorization": "Bearer xxx"}
  # - name: "another-mcp"
  #   enabled: true
  #   type: "streamable_http"
  #   endpoint: "http://127.0.0.1:8080/mcp"
  #   headers: {}

# 标准输入 MCP 配置
stdin_mcp:
  - name: "stdin-tool"
    enabled: true
    command: "npx"
    args: ["-y", "mcp-exec"]
  # - name: "another-stdin-mcp"
  #   enabled: true
  #   command: "node"
  #   args: ["path/to/another/mcp"]
`
