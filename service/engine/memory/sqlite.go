package memory

import (
	"HackerTeam/service/engine/config"
	"HackerTeam/service/engine/models"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/memory/extractor"
	memorysqlite "trpc.group/trpc-go/trpc-agent-go/memory/sqlite"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// NewSQLiteMemoryService 创建带自动记忆提取的 SQLite 记忆服务
// 后台 extractor 在每个 turn 结束后自动从对话中提取记忆
// agent 端暴露除 memory_clear 外的全部记忆工具（search/load/add/update/delete）作为手动补充
func NewSQLiteMemoryService(m config.Model, dbPath string) (*memorysqlite.Service, error) {
	var extractorModel model.Model
	if m.APIType == "openai" {
		extractorModel = models.Openai(m)
	} else if m.APIType == "anthropic" {
		extractorModel = models.Anthropic(m)
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
		memorysqlite.WithToolEnabled(memory.LoadToolName, true), // auto 模式下 load 默认 disabled，必须显式 enable 才能被 exposed 生效
		memorysqlite.WithAutoMemoryExposedTools([]string{memory.AddToolName, memory.UpdateToolName, memory.LoadToolName, memory.DeleteToolName, memory.SearchToolName}...), // 为 agent 暴露除了clear以外的所有记忆操作工具
		memorysqlite.WithMemoryJobTimeout(600*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create sqlite memory service: %w", err)
	}
	return service, nil
}
