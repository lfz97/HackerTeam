package config

// UpdateModelFields 读-改-写整文件更新 model 块：LoadConfig 解析 → vals
// 覆盖对应字段 → yaml.Marshal 序列化落盘。注释不保留（文件被 Marshal 重建），
// 结构性内容（user / mcp / observability 与未提交的 model 字段）按原值回写。
// 前置条件：配置文件已由 bootstrap 在首跑时创建（模板保证四个键都在）。

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// UpdateModelFields 把 vals（yaml 字段名 → 值）覆盖进 path 的 model 块。
// 文件缺失报错（LoadConfig 失败自然上抛，TUI 页内显示后可重试）。
func UpdateModelFields(path string, vals map[string]string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	cfg.Model.Model = vals["model"]
	cfg.Model.BaseURL = vals["baseurl"]
	cfg.Model.APIKey = vals["apikey"]
	cfg.Model.APIType = vals["apitype"]
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %v", err)
	}
	return os.WriteFile(path, data, 0644)
}
