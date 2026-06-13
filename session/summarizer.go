package session

import (
	"HackerTeam/config"
	"HackerTeam/global"
	"HackerTeam/models"
	"HackerTeam/utils/pretty"
	"embed"
	"fmt"
	"regexp"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/tiktoken"
	"trpc.group/trpc-go/trpc-agent-go/session/summary"
)

//go:embed prompt/*
var promptFiles embed.FS

var (
	systemSummarizerPrompt string
	userSummarizerPrompt   string
	reThink                = regexp.MustCompile(`<think>[\s\S]*?<\/think>`)
)

const (
	CheckTokenThresholdPercent float64 = 0.4
	maxSummaryWords            int     = 5000
)

func initSummarizerPrompts() {
	systemSummarizerPrompt_b, _ := promptFiles.ReadFile("prompt/system.md")
	systemSummarizerPrompt = string(systemSummarizerPrompt_b)
	userSummarizerPrompt_b, _ := promptFiles.ReadFile("prompt/user.md")
	userSummarizerPrompt = string(userSummarizerPrompt_b)
}

func NewSummarizer(m config.Model) summary.SessionSummarizer {
	initSummarizerPrompts()
	//设置tiktoken计算方式，默认的方式太不准确了
	counter, _ := tiktoken.New(m.Model)
	summary.SetTokenCounter(counter)
	var summarizerModel model.Model

	if m.APIType == "openai" {
		summarizerModel = models.Openai(m.Model, m.BaseURL, m.APIKey)
	} else if m.APIType == "anthropic" {
		summarizerModel = models.Anthropic(m.Model, m.BaseURL, m.APIKey)
	}
	// ── 创建 summarizer阈值 ───────────────
	sum := summary.NewSummarizer(
		summarizerModel,
		summary.WithChecksAny( // 任一条件满足即触发
			summary.CheckTokenThreshold(int(CheckTokenThresholdPercent*float64(m.ContextWindow))), // 新增 n 个 token 后触发
			summary.CheckTimeThreshold(10*time.Minute),                                            //n 分钟无活动
		),
		summary.WithMaxSummaryWords(maxSummaryWords),     //设置摘要的最大长度，单位为词
		summary.WithSystemPrompt(systemSummarizerPrompt), //设置系统提示词，指导模型如何进行摘要，默认为空，可以根据需要自定义
		summary.WithPrompt(userSummarizerPrompt),         //设置用户提示词，指导模型如何根据会话内容生成摘要，默认为空，可以根据需要自定义
		summary.WithPostSummaryHook(func(s *summary.PostSummaryHookContext) error {
			cleanSummary := reThink.ReplaceAllString(s.Summary, "") //将摘要内容中的<think>...</think>部分去掉
			global.PrintToTui(global.AgentMessage, pretty.TColoredText(pretty.TColorGreen, fmt.Sprintf("\n->已生成摘要：\n%v\n", cleanSummary)), false)
			return nil
		}),
	)
	return sum

}
