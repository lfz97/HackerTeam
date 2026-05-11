package session

import (
	"fmt"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// 自定义工具调用在摘要输入中的格式
func toolcallFormatter(tc model.ToolCall) string {
	name := tc.Function.Name
	if name == "" {
		return ""
	}
	args := string(tc.Function.Arguments)
	const maxLen = 100
	if len(args) > maxLen {
		args = args[:maxLen] + fmt.Sprintf("...(truncated, total: %d)", len(args))
		return fmt.Sprintf("[Tool: %s, Args: %s]", name, args)
	}
	return fmt.Sprintf("[Tool: %s, Args: %s]", name, args)
}

// 自定义工具结果在摘要输入中的格式
func toolResultFormatter(msg model.Message) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}
	name := msg.ToolName
	if name == "" {
		name = "tool"
	}
	const maxLen = 300
	if len(content) > maxLen {
		content = fmt.Sprintf("%s returned: %s...(truncated, total: %d)", name, content[:maxLen], len(content))
	} else {
		content = fmt.Sprintf("%s returned: %s", name, content)
	}
	return content
}
