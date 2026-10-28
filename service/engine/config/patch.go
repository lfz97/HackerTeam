package config

// PatchModelFields 行级更新配置文件 model 块中的指定字段。
// 刻意不做 Unmarshal→Marshal 整文件重写：模板里的 MCP 段注释是给用户看的
// 操作手册，整文件重写会全部丢失。逐行扫描只在 model: 块内做值替换，
// 其余行（含注释、mcp 配置、缩进风格）原样保留。

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// PatchModelFields 把 vals（yaml 字段名 → 新值）写进 path 的 model 块。
// 只替换已存在的行，键不存在视为异常返回错误（模板保证四个键都在）。
func PatchModelFields(path string, vals map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	remaining := make(map[string]string, len(vals))
	for k, v := range vals {
		remaining[k] = v
	}

	inModelBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		isTopLevel := !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")
		if isTopLevel {
			//顶层键决定块边界："model:" 开块，下一个顶层键收块
			inModelBlock = trimmed == "model:"
			continue
		}
		if !inModelBlock {
			continue
		}
		key, ok := blockKey(trimmed)
		if !ok {
			continue
		}
		newVal, pending := remaining[key]
		if !pending {
			continue
		}
		lines[i] = patchLine(line, key, newVal)
		delete(remaining, key)
	}

	if len(remaining) > 0 {
		keys := make([]string, 0, len(remaining))
		for k := range remaining {
			keys = append(keys, k)
		}
		return fmt.Errorf("model block missing keys %v in %s", keys, path)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// blockKey 从块内行提取键名；无键（纯注释已过滤，续行/列表项）返回 false。
func blockKey(trimmed string) (string, bool) {
	idx := strings.Index(trimmed, ":")
	if idx <= 0 {
		return "", false
	}
	return strings.TrimSpace(trimmed[:idx]), true
}

// patchLine 保留原行的缩进与行尾注释，仅替换冒号后的值。
// 值经 yaml 序列化，引号/特殊字符自动转义。
func patchLine(line, key, val string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	comment := ""
	if idx := strings.Index(line, " #"); idx >= 0 {
		comment = " " + line[idx+1:]
	}
	b, _ := yaml.Marshal(val)
	return fmt.Sprintf("%s%s: %s%s", indent, key, strings.TrimSpace(string(b)), comment)
}
