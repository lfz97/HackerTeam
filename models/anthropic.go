package models

import (
	"HackerTeam/config"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/option"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
)

// 兼容anthropic模型的接口，方便后续替换模型提供商
func Anthropic(config config.Model) *anthropic.Model {
	var modelInstance *anthropic.Model
	if config.AnthropicAuthHeaderTransfer {
		modelInstance = anthropic.New(
			config.Model,
			anthropic.WithBaseURL(config.BaseURL),
			anthropic.WithAnthropicClientOptions(
				option.WithHeader("Authorization", fmt.Sprintf("Bearer %s", config.APIKey)),
			),
		)

	} else {
		modelInstance = anthropic.New(
			config.Model,
			anthropic.WithBaseURL(config.BaseURL),
			anthropic.WithAPIKey(config.APIKey),
		)

	}
	return modelInstance

}
