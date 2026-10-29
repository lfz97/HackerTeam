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

// GetTodoTools 返回框架内置的 todo_write 工具（经 todoTool 包装），为 Agent 提供跨轮持久化的
// 结构化任务清单；清单按 invocation branch 分键隔离，子 Agent 不会覆盖队长的计划。
// 完整使用说明见 GetTodoToolPrompt，需拼进系统提示词才能约束模型行为。
func GetTodoTools() []tool.Tool {
	return []tool.Tool{todoTool{Tool: todo.New()}}
}

// todoTool 包装框架内置 todo_write，为模型可见结果注册 formatter：清单当前状态已由
// 队长 BeforeModel 状态栏每跳注入，工具结果只保留 nudge 文案，避免同一清单重复占 token。
// 框架通过同名 ResultFormatter() 方法的接口断言发现 formatter，规则同 function.WithResultFormatter。
type todoTool struct {
	*todo.Tool
}

// ResultFormatter 实现上述鸭子接口。保持 message 字段的 JSON 形态，TUI 按字段提取的
// 渲染逻辑不变；formatter 不参与状态持久化协议。
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

// customTodoToolPrompt 是注入队长系统提示词的 todo_write 使用说明。
//
// 不用框架 todo.DefaultToolPrompt：那份文案教的是"单条线串行执行纪律"（at most one
// in_progress、开始前先标记），会把计划时刻的路由决策冻结成全自留清单。这里改定义为
// "路由台账"：每项带路由（自己/脚本/派发 Worker），派发的 run 记在条目文本里，事实
// 变了要重路由。
//
// 文案必须遵守的运行时硬约束（todo.validateTodos 代码级强制，违反调用直接报错）：
// content/activeForm 非空、content 唯一、status ∈ pending|in_progress|completed、
// 至多一个 in_progress。文案里"Rules (enforced by the tool)"一节与其保持一致。
const customTodoToolPrompt = `## Task ledger (todo_write)

todo_write is the routing ledger for the engagement, not a queue you must grind through yourself.

Each item records one piece of work with its route:
- content:    the question or deliverable plus its route — yourself, a script, or a Worker dispatch (include async run ids once dispatched).
- activeForm: present-continuous form, e.g. "Verifying captcha bypass".
- status:     pending | in_progress | completed.

Rules (enforced by the tool — violating calls fail):
- At most ONE item in_progress: your own critical path. Dispatched runs are recorded in item text and tracked via list_task_runs, not as extra in_progress items.
- content and activeForm non-empty; content unique across the list.
- Each call replaces the entire list — send the full updated list every time.
- Mark a task completed immediately when it is done; keep it pending until then.

Re-route as facts change: an item planned as self-work that turns out to be a pre-writable, self-contained discriminator becomes a dispatch; an item blocked on a Worker becomes yours again when it returns needs_input.`

// GetTodoToolPrompt 返回 todo_write 工具的使用说明，供队长提示词占位符注入。
// 文案见 customTodoToolPrompt（自定义的"路由台账"版本，运行时校验规则不受影响）。
func GetTodoToolPrompt() string {
	return customTodoToolPrompt
}

// 状态栏渲染参数上限：pending 条数，避免清单过长撑爆尾部消息。
const todoStatusBarMaxPending = 8

// TodoStatusBar 返回当前 invocation 会话中 todo 清单的紧凑状态摘要，供队长 BeforeModel
// 状态栏追加；队长（按 invocation branch 分键）没有清单时返回空串。
// 经框架公开的 todo.GetTodos 按 inv.Branch 读取，与写入端同 key 对齐：同轮内工具写入
// 后下一跳 LLM 请求即可读到最新状态，跨轮及上下文压缩后同样生效。
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
//
//	[TODO]
//	◐ 进行中（activeForm）
//	☐ 待办（content，超过 todoStatusBarMaxPending 折叠为计数）
//	(N more · N done)
//
// 条目文本不截断，保持完整语义。输出是带换行的多行文本，同一份喂给 [STATUS]
// prompt 尾部与 TodoBar（后者按行数占高）。
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
