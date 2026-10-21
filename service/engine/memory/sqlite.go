package memory

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
)

// NewSQLiteMemoryService 创建 SQLite 记忆服务（纯手动模式，无后台 extractor）：
// 记忆的写入与维护完全靠 agent 显式调用工具；agentic 模式下 enabledTools 是暴露闸门
// （Delete 需显式打开，Clear 默认不暴露——危险操作）。
func NewSQLiteMemoryService(dbPath string) (*memorysqlite.Service, error) {
	dsn := dbPath + "?_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	service, err := memorysqlite.NewService(
		db,
		memorysqlite.WithSoftDelete(true),
		memorysqlite.WithMemoryLimit(100000),
		memorysqlite.WithToolEnabled(memory.DeleteToolName, true),
	)
	if err != nil {
		return nil, fmt.Errorf("create sqlite memory service: %w", err)
	}
	return service, nil
}
