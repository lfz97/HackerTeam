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
	ShowReasoning               bool   `yaml:"show_reasoning"`
}

type User struct {
	UserID string `yaml:"userid"`
}

type Config struct {
	Model Model `yaml:"model"`
	User  User  `yaml:"user"`
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
	return &YamlConfig, nil
}
