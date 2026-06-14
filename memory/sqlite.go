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
		memorysqlite.WithAutoMemoryExposedTools([]string{memory.AddToolName, memory.UpdateToolName}...), //为agent额外暴露添加工具和更新工具的能力，以便agent能够在记忆中添加和更新工具信息
	)
	if err != nil {
		return nil, fmt.Errorf("create sqlite memory service: %w", err)
	}
	return service, nil
}
