package models

import (
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// 兼容openai模型的接口，方便后续替换模型提供商
func Openai(Model string, BaseUrl string, APIkey string) *openai.Model {
	opts := []openai.Option{
		openai.WithBaseURL(BaseUrl),
		openai.WithAPIKey(APIkey),
		// 关闭 system 前置重排：状态栏是动态 system（每次调用内容变化），
		// 重排会把变化部分挪进前缀、破坏自动前缀缓存（实测：尾部95%命中 vs 头部0）。
		// 框架默认对通用 OpenAI 变体开启、DeepSeek 变体默认关闭；显式关闭以统一行为并保住尾部形态。
		openai.WithOptimizeForCache(false),
	}

	if strings.Contains(Model, "deepseek") {
		opts = append(opts,
			openai.WithVariant(openai.VariantDeepSeek),
			openai.WithReasoningContentBackfill(true), //开启推理内容回填，解决模型响应reasoning为空时，框架不拼接推理字段，导致api报错
		)
	}

	modelInstance := openai.New(
		Model,
		opts...,
	)
	return modelInstance
}
