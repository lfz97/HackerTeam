package config

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
)

type Model struct {
	Model                       string `yaml:"model"`
	BaseURL                     string `yaml:"baseurl"`
	APIKey                      string `yaml:"apikey"`
	APIType                     string `yaml:"apitype"`
	AnthropicAuthHeaderTransfer bool   `yaml:"anthropicAuthHeaderTransfer"`
	Stream                      bool   `yaml:"stream"`
	ContextWindow               int    `yaml:"contextwindow"`
	MaxTokens                   int    `yaml:"maxtokens"` // 每次请求的最大生成 token 数，默认 32000
	ShowReasoning               bool   `yaml:"show_reasoning"`
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
	if YamlConfig.Model.MaxTokens == 0 { // maxtokens 未配置时使用默认值 32000
		YamlConfig.Model.MaxTokens = 32000
	}
	return &YamlConfig, nil
}
