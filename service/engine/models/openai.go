package models

import (
	"HackerTeam/service/engine/config"
	"strings"
	"time"

	openaiopt "github.com/openai/openai-go/option"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// 兼容 openai 协议的模型构造，方便后续替换模型提供商。
func Openai(config config.Model) *openai.Model {
	opts := []openai.Option{
		openai.WithBaseURL(config.BaseURL),
		openai.WithAPIKey(config.APIKey),
		// 关闭 system 前置重排：状态栏是每次变化的动态 system，重排会把它挪进前缀、
		// 破坏自动前缀缓存（实测尾部95%命中 vs 头部0）。
		openai.WithOptimizeForCache(false),
		openai.WithOpenAIOptions(openaiopt.WithRequestTimeout(time.Duration(config.HttpTimeout) * time.Second)),
		openai.WithContextWindow(config.ContextWindow),
	}

	if strings.Contains(config.Model, "deepseek") {
		opts = append(opts,
			openai.WithVariant(openai.VariantDeepSeek),
			// reasoning 为空时框架不拼接推理字段，会导致 API 报错
			openai.WithReasoningContentBackfill(true),
		)
	}

	modelInstance := openai.New(
		config.Model,
		opts...,
	)
	return modelInstance
}
