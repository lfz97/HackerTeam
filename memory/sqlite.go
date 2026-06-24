package memory

import (
	"HackerTeam/config"
	"HackerTeam/models"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/memory/extractor"
	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// NewSQLiteMemoryService 创建带自动记忆提取的 SQLite 记忆服务
// 后台 extractor 在每个 turn 结束后自动从对话中提取记忆
// agent 端暴露 memory_search/load/add/update 工具作为手动补充
func NewSQLiteMemoryService(m config.Model, dbPath string) (*memorysqlite.Service, error) {
	var extractorModel model.Model
	if m.APIType == "openai" {
		extractorModel = models.Openai(m.Model, m.BaseURL, m.APIKey)
	} else if m.APIType == "anthropic" {
		extractorModel = models.Anthropic(m.Model, m.BaseURL, m.APIKey)
	}

	dsn := dbPath + "?_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	ext := extractor.NewExtractor(extractorModel)
	service, err := memorysqlite.NewService(
		db,
		memorysqlite.WithSoftDelete(true),
		memorysqlite.WithMemoryLimit(100000),
		memorysqlite.WithExtractor(ext),
		memorysqlite.WithAutoMemoryExposedTools([]string{memory.AddToolName, memory.UpdateToolName}...), // 为 agent 额外暴露添加和更新工具，允许手动补充记忆
	)
	if err != nil {
		return nil, fmt.Errorf("create sqlite memory service: %w", err)
	}
	return service, nil
}
