package functionTools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/resultformat"
	"trpc.group/trpc-go/trpc-agent-go/tool/todo"
)

// todoWriteToolName 框架内置 todo 工具的注册名（等价于 tool/todo.DefaultToolName）。
const todoWriteToolName string = "todo_write"

// GetTodoTools 返回框架内置的 todo_write 工具（经 todoTool 包装）：为 Agent 提供跨轮持久化的结构化任务清单。
// 清单写入 session state（temp:todos[:branch]），按 invocation branch 分键隔离，
// 因此各子 Agent 读写的是自己的清单，不会覆盖队长的计划。
// 工具的完整使用说明见 GetTodoToolPrompt，需要拼进系统提示词才能约束模型行为。
func GetTodoTools() []tool.Tool {
	return []tool.Tool{todoTool{Tool: todo.New()}}
}

// todoTool 包装框架内置 todo_write，为模型可见结果注册 formatter。清单的当前状态
// 已由队长 BeforeModel 状态栏每跳注入（见 agent/status.go），工具结果只保留 nudge
// 文案，不再回显整份 todos/oldTodos，避免同一清单在上下文里重复占 token。
// 这是框架文档化的机制：tool/function 之外的工具实现只要暴露同名 ResultFormatter()
// 方法，就按与 function.WithResultFormatter 相同的规则参与（processor 经
// resultFormatterProvider 接口断言发现，见 internal/flow/processor/functioncall.go）。
type todoTool struct {
	*todo.Tool
}

// ResultFormatter 实现上述鸭子接口。保持 message 字段的 JSON 形态，TUI 侧按字段
// 提取的渲染逻辑不变；state 持久化走内嵌 Tool 的 StateDeltaForInvocation，不受
// formatter 影响（formatter 不参与状态协议）。
func (t todoTool) ResultFormatter() resultformat.Formatter {
	return resultformat.FormatterFunc[todo.Output](
		func(_ context.Context, out todo.Output) (string, error) {
			b, err := json.Marshal(struct {
				Message string `json:"message"`
			}{Message: out.Message})
			if err != nil {
				return "", fmt.Errorf("todo_write: format result: %w", err)
			}
			return string(b), nil
		},
	)
}

// GetTodoToolPrompt 返回 todo_write 工具的默认使用说明（tool/todo.DefaultToolPrompt），
// 供队长提示词占位符注入。工具的运行时校验（todos 必填、最多一个 in_progress、
// content 唯一等）与这里无关，调整文案不会改变校验规则。
func GetTodoToolPrompt() string {
	return todo.DefaultToolPrompt
}

// 状态栏渲染参数上限：pending 条数，避免清单过长撑爆尾部消息。
const todoStatusBarMaxPending = 8

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

// renderTodoStatusBar 把清单渲染成纵向摘要，每个元素独占一行：
//   [TODO]
//   ◐ 进行中（activeForm）
//   ☐ 待办（content，超过 todoStatusBarMaxPending 折叠为计数）
//   (N more · N done)
// 条目文本不截断，保持完整语义。全 completed 的清单会被 todo_write 自动清空，
// 不会走到这里。输出是带换行的多行文本，同一份喂给 [STATUS] prompt 尾部与
// TodoBar（后者按行数占高）。
func renderTodoStatusBar(items []todo.Item) string {
	var inProgressText string
	var pendingTexts []string
	pending, completed := 0, 0
	for _, it := range items {
		switch it.Status {
		case todo.StatusInProgress:
			inProgressText = strings.TrimSpace(it.ActiveForm)
		case todo.StatusPending:
			pending++
			if len(pendingTexts) < todoStatusBarMaxPending {
				pendingTexts = append(pendingTexts, strings.TrimSpace(it.Content))
			}
		case todo.StatusCompleted:
			completed++
		}
	}
	lines := []string{"[TODO]"}
	if inProgressText != "" {
		lines = append(lines, "◐ "+inProgressText)
	}
	for _, text := range pendingTexts {
		lines = append(lines, "☐ "+text)
	}
	// 计数行：折叠数与完成数合并为一行，无计数时整行省略
	var counts []string
	if hidden := pending - len(pendingTexts); hidden > 0 {
		counts = append(counts, fmt.Sprintf("%d more", hidden))
	}
	if completed > 0 {
		counts = append(counts, fmt.Sprintf("%d done", completed))
	}
	if len(counts) > 0 {
		lines = append(lines, "("+strings.Join(counts, " · ")+")")
	}
	if len(lines) == 1 {
		return "" // 防御：items 非空时必有一种状态命中，正常不会走到
	}
	return strings.Join(lines, "\n")
}
