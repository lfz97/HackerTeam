package memory

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
)

// NewSQLiteMemoryService 创建纯 agent 驱动的 SQLite 记忆服务（无后台自动提取）
// 暴露 5 个工具：memory_search, memory_load, memory_add, memory_update, memory_delete
// memory_clear 不暴露（清空全部记忆风险过高）
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
