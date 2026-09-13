package session

import (
	"HackerTeam/service/engine/config"
	"time"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// NewMemorySessionService 创建一个基于内存的 SessionService 实例，使用自动摘要功能来管理会话上下文。
// NewMemorySessionService 创建一个基于内存的 SessionService 实例，使用自动摘要功能来管理会话上下文。
// tui 只用于摘要生成后打一行提示，见 msgPrinter。
func NewMemorySessionService(m config.Model, tui msgPrinter) *inmemory.SessionService {
	MemSessionService := inmemory.NewSessionService(
		inmemory.WithSummarizer(NewSummarizer(m, tui)),
		inmemory.WithAsyncSummaryNum(2),
		inmemory.WithSummaryQueueSize(100),
		inmemory.WithSummaryJobTimeout(600*time.Second),
	)
	return MemSessionService
}

