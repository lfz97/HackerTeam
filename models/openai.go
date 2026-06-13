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
