package session

import (
	"context"
	"HackerTeam/service/engine/config"
	"time"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// NewMemorySessionService 创建一个基于内存的 SessionService 实例，使用自动摘要功能来管理会话上下文。
func NewMemorySessionService(m config.Model, tui tuiService) *inmemory.SessionService {
	MemSessionService := inmemory.NewSessionService(
		inmemory.WithSummarizer(NewSummarizer(m, tui)),
		inmemory.WithAsyncSummaryNum(2),
		inmemory.WithSummaryQueueSize(100),
		inmemory.WithSummaryJobTimeout(600*time.Second),
	)
	return MemSessionService
}

type tuiService interface {
	AddHelpItems(items []map[string]string)
	ClearAppFuncTrigger()
	PrintToMsgView(content string, clear bool)
	ReadInputAreaPromptWithEnter() string
	ResetHelpItems()
	SetAppFuncTriggerWithEsc(f func())
	ShowErrorInMsgViewAndExit(errmsg string)
	ShowMsgAndExitNoTrigger(msg string)
	ShowSuccessInMsgView(sussessmsg string)
	ShowSuccessInMsgViewAndExit(sussessmsg string)
	StatusBarScrollingTip(ctx context.Context, tip string, TColor string)
	StatusBarUserTip(s string)
}
