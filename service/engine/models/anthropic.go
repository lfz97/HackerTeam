package models

import (
	"HackerTeam/service/engine/config"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
)

// 兼容 anthropic 协议的模型构造，方便后续替换模型提供商。
func Anthropic(config config.Model) *anthropic.Model {
	var modelInstance *anthropic.Model
	opts := []anthropic.Option{
		anthropic.WithBaseURL(config.BaseURL),
		anthropic.WithAnthropicClientOptions(option.WithRequestTimeout(time.Duration(config.HttpTimeout) * time.Second)),
		anthropic.WithContextWindow(config.ContextWindow),
	}
	if config.AnthropicAuthHeaderTransfer {
		opts = append(opts, anthropic.WithAnthropicClientOptions(option.WithHeader("Authorization", fmt.Sprintf("Bearer %s", config.APIKey))))
		modelInstance = anthropic.New(
			config.Model,
			opts...,
		)

	} else {
		opts = append(opts, anthropic.WithAPIKey(config.APIKey))
		modelInstance = anthropic.New(
			config.Model,
			opts...,
		)

	}
	return modelInstance
}
