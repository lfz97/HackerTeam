package session

import (
	"HackerTeam/config"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// NewMemorySessionService 创建一个基于内存的 SessionService 实例，使用自动摘要功能来管理会话上下文。
func NewMemorySessionService(m config.Model) *inmemory.SessionService {
	MemSessionService := inmemory.NewSessionService(
		inmemory.WithSummarizer(NewSummarizer(m)),
		inmemory.WithAsyncSummaryNum(2),
		inmemory.WithSummaryQueueSize(100),
		inmemory.WithSummaryJobTimeout(60*time.Second),
	)
	return MemSessionService
}
