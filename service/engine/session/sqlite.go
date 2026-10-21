package session

import (
	"HackerTeam/service/engine/config"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	sqlitesession "trpc.group/trpc-go/trpc-agent-go/session/sqlite"
)

// NewSQLiteSessionService 创建基于 SQLite 的会话服务（会话跨进程持久化），
// 摘要参数与 inmemory 版一致，建表幂等（目录须已存在）。
// DSN 参数与 memory/sqlite.go 一致，rationale 见彼处注释。
func NewSQLiteSessionService(dbPath string, m config.Model, sink summarySink) (*sqlitesession.Service, error) {
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
