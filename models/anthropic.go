package models

import (
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
)

// 兼容anthropic模型的接口，方便后续替换模型提供商
func Anthropic(Model string, BaseUrl string, APIkey string) *anthropic.Model {
	modelInstance := anthropic.New(
		Model,
		anthropic.WithBaseURL(BaseUrl),
		anthropic.WithAPIKey(APIkey),
	)
	return modelInstance
}
