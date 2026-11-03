package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleYAML = `#用户配置
user:
  userid: "uid-1"

# 模型配置
model:
  model: "deepseek-reasoner"
  baseurl: "https://api.deepseek.com"
  apikey: "your-api-key" # 填你的 key
  apitype: "openai" # openai 或者 anthropic
  contextwindow: 128000 # 注释要原样保留

# MCP 服务配置
http_mcp:
  - name: "mcpexec"
    enabled: true
`

func TestUpdateModelFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "HackerTeam.yaml")
	if err := os.WriteFile(path, []byte(sampleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	vals := map[string]string{
		"model":   "glm-5.3",
		"baseurl": "https://open.bigmodel.cn/api/paas/v4",
		"apikey":  "sk-test \"quoted\"",
		"apitype": "anthropic",
	}
	if err := UpdateModelFields(path, vals); err != nil {
		t.Fatalf("UpdateModelFields: %v", err)
	}

	//注释不做断言：yaml.Marshal 重建文件，注释有意不保留。
	//整文件仍需可解析，且值与结构正确。
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("updated yaml unparseable: %v", err)
	}
	if cfg.Model.Model != "glm-5.3" || cfg.Model.BaseURL != vals["baseurl"] ||
		cfg.Model.APIKey != vals["apikey"] || cfg.Model.APIType != "anthropic" {
		t.Fatalf("values not applied: %+v", cfg.Model)
	}

	//未提交的 model 字段与块外结构按原值回写
	if cfg.Model.ContextWindow != 128000 {
		t.Errorf("untouched field changed: contextwindow=%d", cfg.Model.ContextWindow)
	}
	if cfg.User.UserID != "uid-1" {
		t.Errorf("user block lost: %+v", cfg.User)
	}
	if len(cfg.HttpMcp) != 1 || cfg.HttpMcp[0].Name != "mcpexec" || !cfg.HttpMcp[0].Enabled {
		t.Errorf("http_mcp block lost: %+v", cfg.HttpMcp)
	}
}

func TestUpdateModelFieldsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "HackerTeam.yaml")
	if err := UpdateModelFields(path, map[string]string{"apikey": "k"}); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestUpdateModelFieldsTemplate 用真实内嵌模板做整文件读-改-写回归：
// 模板（含 observability 默认段）经更新后仍可被 LoadConfig 正确解析。
func TestUpdateModelFieldsTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "HackerTeam.yaml")
	if err := os.WriteFile(path, []byte(Template), 0644); err != nil {
		t.Fatal(err)
	}

	vals := map[string]string{
		"model":   "glm-5.3",
		"baseurl": "https://open.bigmodel.cn/api/paas/v4",
		"apikey":  "sk-test \"quoted\" # not comment",
		"apitype": "anthropic",
	}
	if err := UpdateModelFields(path, vals); err != nil {
		t.Fatalf("UpdateModelFields: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("updated yaml unparseable: %v", err)
	}
	if cfg.Model.Model != "glm-5.3" || cfg.Model.BaseURL != vals["baseurl"] ||
		cfg.Model.APIKey != vals["apikey"] || cfg.Model.APIType != "anthropic" {
		t.Fatalf("values not applied: %+v", cfg.Model)
	}

	//模板默认段与未提交字段保留（{USERID} 直写模板属预期，替换发生在 bootstrap；
	//模板 mcp 段默认全注释，应为空列表）
	if cfg.User.UserID != "{USERID}" {
		t.Errorf("user block lost: %+v", cfg.User)
	}
	if cfg.Model.ContextWindow != 128000 || !cfg.Model.Stream || cfg.Model.MaxTokens != 12800 {
		t.Fatalf("untouched model fields changed: %+v", cfg.Model)
	}
	if cfg.Observability.Service != "langfuse" {
		t.Errorf("observability block lost: %+v", cfg.Observability)
	}
	if len(cfg.HttpMcp) != 0 || len(cfg.StdinMcp) != 0 {
		t.Errorf("mcp blocks should be empty by default: %+v %+v", cfg.HttpMcp, cfg.StdinMcp)
	}
}
