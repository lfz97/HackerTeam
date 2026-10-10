package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Model struct {
	Model                       string `yaml:"model"`
	BaseURL                     string `yaml:"baseurl"`
	APIKey                      string `yaml:"apikey"`
	APIType                     string `yaml:"apitype"`
	AnthropicAuthHeaderTransfer bool   `yaml:"anthropicAuthHeaderTransfer"`
	Stream                      bool   `yaml:"stream"`
	ContextWindow               int    `yaml:"contextwindow"`
	MaxTokens                   int    `yaml:"maxtokens"` // 每次请求的最大生成 token 数，默认 12800
	ShowReasoning               bool   `yaml:"show_reasoning"`
	ReasoningEffort             string `yaml:"reasoning_effort"` // 思考强度档位（如 low/medium/high/max），留空则不下发该参数
	HttpTimeout                 int    `yaml:"httptimeout"`
}

// ReasoningEffortPtr 返回思考强度档位的指针；未配置（空串）返回 nil——
// 框架的 GenerationConfig 以指针判空决定是否下发该参数。
func (m *Model) ReasoningEffortPtr() *string {
	if m.ReasoningEffort == "" {
		return nil
	}
	return &m.ReasoningEffort
}

type User struct {
	UserID string `yaml:"userid"`
}

type Config struct {
	Model    Model      `yaml:"model"`
	User     User       `yaml:"user"`
	HttpMcp  []HttpMCP  `yaml:"http_mcp"`  // HTTP 传输 MCP server 列表
	StdinMcp []StdinMCP `yaml:"stdin_mcp"` // stdio 传输 MCP server 列表
}

// LoadConfig 读取并解析配置文件
func LoadConfig(path string) (*Config, error) {
	YamlConfig := Config{}
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件错误：%v", err)
	}
	err = yaml.Unmarshal(yamlFile, &YamlConfig)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件错误：%v", err)
	}
	if YamlConfig.Model.MaxTokens == 0 { // maxtokens 未配置时使用默认值 12800
		YamlConfig.Model.MaxTokens = 12800
	}
	return &YamlConfig, nil
}
