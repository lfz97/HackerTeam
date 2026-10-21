package session

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/models"
	"embed"
	"fmt"
	"regexp"

	"strings"

	"trpc.group/trpc-go/trpc-agent-go/event"
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
	CheckTokenThresholdPercent float64 = 0.6
	maxSummaryWords            int     = 5000
)

func initSummarizerPrompts() {
	systemSummarizerPrompt_b, _ := promptFiles.ReadFile("prompt/system.md")
	systemSummarizerPrompt = string(systemSummarizerPrompt_b)
	userSummarizerPrompt_b, _ := promptFiles.ReadFile("prompt/user.md")
	userSummarizerPrompt = string(userSummarizerPrompt_b)
}

// summarySink 摘要生成后投一条摘要记录（pull 契约，渲染是消费端的事）；
// 消费方小接口，session 包不依赖任何 TUI 类型。
type summarySink interface {
	AppendSummary(text string)
}

// NewSummarizer 构造摘要器，sink 用于摘要生成后投递展示（可传 nil，跳过投递）。
func NewSummarizer(m config.Model, sink summarySink) summary.SessionSummarizer {
	return NewSummarizerWithThreshold(m, sink, CheckTokenThresholdPercent)
}

// NewSummarizerWithThreshold 用调用方给定的阈值百分比替代包级默认（相对 m.ContextWindow）。
func NewSummarizerWithThreshold(m config.Model, sink summarySink, thresholdPercent float64) summary.SessionSummarizer {
	initSummarizerPrompts()
	//设置tiktoken计算方式，默认的方式太不准确了
	counter, _ := tiktoken.New(m.Model)
	summary.SetTokenCounter(counter)
	var summarizerModel model.Model

	if m.APIType == "openai" {
		summarizerModel = models.Openai(m)
	} else if m.APIType == "anthropic" {
		summarizerModel = models.Anthropic(m)
	}
	// 触发条件：请求占用超过窗口 thresholdPercent 时触发压缩。勿与其他 threshold 机制混用：
	// s.checks 是 AND 短路求值，叠加会变成"两者都过才触发"。
	opts := []summary.Option{
		summary.WithContextThreshold(
			summary.WithContextThresholdRatio(thresholdPercent),
		),
		summary.WithMaxSummaryWords(maxSummaryWords), // 单位为词
		summary.WithSystemPrompt(systemSummarizerPrompt),
		summary.WithPrompt(userSummarizerPrompt),
		summary.WithSkipRecent(func(events []event.Event) int { // 保留最后一轮完整对话，压缩后最新语境仍在
			skip := 0
			for i := len(events) - 1; i >= 0; i-- {
				skip++
				if events[i].Author == "user" {
					return skip
				}
			}
			return 0
		}),
		summary.WithToolResultFormatter(func(msg model.Message) string { // 过长的 tool result 会让摘要质量下降
			content := strings.TrimSpace(msg.Content)
			runes := []rune(content)
			if len(runes) > 1000 {
				head := string(runes[:500])
				tail := string(runes[len(runes)-500:])
				content = head + "\n...[output truncated]...\n" + tail
			}
			return fmt.Sprintf("[%s returned: %s]", msg.ToolName, content)
		}),
	}
	if sink != nil { // 无展示端（如后台 agent 传 nil）时跳过投递，防 panic
		opts = append(opts, summary.WithPostSummaryHook(func(s *summary.PostSummaryHookContext) error {
			cleanSummary := reThink.ReplaceAllString(s.Summary, "") // 去掉 <think>...</think>部分去掉
			sink.AppendSummary(cleanSummary)
			return nil
		}))
	}
	sum := summary.NewSummarizer(
		summarizerModel,
		opts...,
	)
	return sum

}
