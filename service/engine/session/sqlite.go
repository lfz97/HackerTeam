package session

import (
	"HackerTeam/service/engine/config"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	sqlitesession "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

// NewSQLiteSessionService 创建基于 SQLite 的会话服务实例（默认模式，会话跨进程持久化）。
// 摘要参数与 inmemory 版保持一致；框架的 NewService 会以 CREATE TABLE/INDEX IF NOT EXISTS
// 幂等建表（session.db 不存在时由驱动自动创建文件，目录须已存在）。
//
// DSN 连接参数与 memory/sqlite.go 的 memory.db 完全一致（rationale 见彼处注释）：
// WAL 保证读写并发（每轮 GetSession 都会带着事件与摘要做长读，rollback 模式下会挡写）；
// _txlock=immediate 把写锁提到事务开头，规避框架延迟事务中途升级撞锁时
// BUSY 不走 busy handler 的问题；busy_timeout 兜底等待。
// 以上参数由 mattn/go-sqlite3 在每条新连接建立时自动 PRAGMA，框架不参与。
func NewSQLiteSessionService(dbPath string, m config.Model, sink msgPrinter) (*sqlitesession.Service, error) {
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=15000&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	service, err := sqlitesession.NewService(
		db,
		sqlitesession.WithSummarizer(NewSummarizer(m, sink)),
		sqlitesession.WithAsyncSummaryNum(2),
		sqlitesession.WithSummaryQueueSize(100),
		sqlitesession.WithSummaryJobTimeout(600*time.Second),
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create sqlite session service: %w", err)
	}
	return service, nil
}
