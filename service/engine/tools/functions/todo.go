package functionTools

import (
	"context"
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/todo"
)

// todoWriteToolName 框架内置 todo 工具的注册名（等价于 tool/todo.DefaultToolName）。
const todoWriteToolName string = "todo_write"

// GetTodoTools 返回框架内置的 todo_write 工具：为 Agent 提供跨轮持久化的结构化任务清单。
// 清单写入 session state（temp:todos[:branch]），按 invocation branch 分键隔离，
// 因此各子 Agent 读写的是自己的清单，不会覆盖队长的计划。
// 工具的完整使用说明见 GetTodoToolPrompt，需要拼进系统提示词才能约束模型行为。
func GetTodoTools() []tool.Tool {
	return []tool.Tool{todo.New()}
}

// GetTodoToolPrompt 返回 todo_write 工具的默认使用说明（tool/todo.DefaultToolPrompt），
// 供队长提示词占位符注入。工具的运行时校验（todos 必填、最多一个 in_progress、
// content 唯一等）与这里无关，调整文案不会改变校验规则。
func GetTodoToolPrompt() string {
	return todo.DefaultToolPrompt
}

// 状态栏渲染参数上限：pending 条数与单条文本长度，避免清单过长撑爆尾部消息。
const (
	todoStatusBarMaxPending   = 8
	todoStatusBarMaxItemRunes = 80
)

// TodoStatusBar 返回当前 invocation 会话中 todo 清单的紧凑状态摘要，供队长 BeforeModel
// 状态栏追加；队长（按 invocation branch 分键）没有清单时返回空串。
// 清单由 todo_write 写入 session state，这里走框架公开的 todo.GetTodos 按 inv.Branch
// 读取，与写入端同 key 对齐：同轮内工具写入后下一跳 LLM 请求即可读到最新状态，
// 跨轮以及上下文压缩稀释掉历史中的 todo_write 记录后同样生效。
func TodoStatusBar(ctx context.Context) string {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	items, err := todo.GetTodos(inv.Session, inv.Branch)
	if err != nil || len(items) == 0 {
		return ""
	}
	return renderTodoStatusBar(items)
}

// renderTodoStatusBar 把清单渲染成单行摘要：进行中的条目置顶（activeForm），
// pending 逐条列出（content，超过 todoStatusBarMaxPending 折叠为计数），
// completed 只显示计数。全 completed 的清单会被 todo_write 自动清空，不会走到这里。
func renderTodoStatusBar(items []todo.Item) string {
	var inProgressText string
	var pendingTexts []string
	pending, completed := 0, 0
	for _, it := range items {
		switch it.Status {
		case todo.StatusInProgress:
			inProgressText = truncateRunes(it.ActiveForm, todoStatusBarMaxItemRunes)
		case todo.StatusPending:
			pending++
			if len(pendingTexts) < todoStatusBarMaxPending {
				pendingTexts = append(pendingTexts, truncateRunes(it.Content, todoStatusBarMaxItemRunes))
			}
		case todo.StatusCompleted:
			completed++
		}
	}
	var b strings.Builder
	b.WriteString("[TODO] ")
	if inProgressText != "" {
		b.WriteString("◐ " + inProgressText)
	}
	if len(pendingTexts) > 0 {
		if b.Len() > len("[TODO] ") {
			b.WriteString(" | ")
		}
		b.WriteString("☐ " + strings.Join(pendingTexts, " | ☐ "))
		if hidden := pending - len(pendingTexts); hidden > 0 {
			b.WriteString(fmt.Sprintf(" (+%d more)", hidden))
		}
	}
	if completed > 0 {
		b.WriteString(fmt.Sprintf(" (%d done)", completed))
	}
	return b.String()
}

// truncateRunes 按 rune 截断并去掉首尾空白，超长补省略号。
func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}
