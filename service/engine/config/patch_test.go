package config

import (
	"os"
	"path/filepath"
	"strings"
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
  contextwindow: 64000 # 注释要原样保留

# MCP 服务配置
http_mcp:
  - name: "mcpexec"
    enabled: true
`

func TestPatchModelFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hyperbot.yaml")
	if err := os.WriteFile(path, []byte(sampleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	vals := map[string]string{
		"model":   "glm-5.3",
		"baseurl": "https://open.bigmodel.cn/api/paas/v4",
		"apikey":  "sk-test \"quoted\"",
		"apitype": "anthropic",
	}
	if err := PatchModelFields(path, vals); err != nil {
		t.Fatalf("PatchModelFields: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	//值已更新（apikey 含引号，yaml 序列化后应仍可解析回原文）
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("patched yaml unparseable: %v", err)
	}
	if cfg.Model.Model != "glm-5.3" || cfg.Model.BaseURL != vals["baseurl"] ||
		cfg.Model.APIKey != vals["apikey"] || cfg.Model.APIType != "anthropic" {
		t.Fatalf("values not applied: %+v", cfg.Model)
	}

	//注释与块外内容原样保留
	for _, want := range []string{
		"# 模型配置",
		"# 填你的 key",
		"# openai 或者 anthropic",
		"# 注释要原样保留",
		"# MCP 服务配置",
		"userid: \"uid-1\"",
		"contextwindow: 64000",
		"    enabled: true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("comment/content lost: %q\ngot:\n%s", want, text)
		}
	}
}

func TestPatchModelFieldsMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hyperbot.yaml")
	if err := os.WriteFile(path, []byte("model:\n  model: \"a\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PatchModelFields(path, map[string]string{"apikey": "k"}); err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}
