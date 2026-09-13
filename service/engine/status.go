package engine

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	functionTools "HackerTeam/service/engine/tools/functions"

	"github.com/mackerelio/go-osstat/memory"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	agentmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	memoryQueryRunes    = 100 // user prompt 取头部、assistant content 取尾部的字数
	memoryMaxResults    = 10
	memoryEntryMaxRunes = 200
)

// todoTextSink 接收渲染好的 todo 文本，供 TUI 的 TodoBar 显示。
// 在 engine 包内声明（1 个方法），避免把 init.go 的 tuiService 胖接口渗进调用方——
// 状态回调只需要"能把文本推给 UI"这一个能力。
// 非交互调用方（子 agent 装配）传 nil：仍然注入 prompt，只是不推 UI。
type todoTextSink interface {
	SetTodoText(text string)
}

// 注入模型调用前callback，在消息末尾追加当前状态栏（时间、工作目录、内存、todo清单、相关记忆）。
// 注意：追加在末尾而非前置 —— 自动前缀缓存要求请求头部保持稳定，
// 状态栏每次调用内容变化，放头部会破坏整个前缀缓存（实测：尾部99%命中 vs 头部0）。
// 使用本功能必须关闭框架的 system 前置重排（openai.WithOptimizeForCache），否则
// 尾部状态栏会被框架挪回头部、缓存收益失效；关闭位置：service/engine/models/openai.go。
// 状态栏不进 session（仅存在于当次请求副本），不污染摘要/上下文压缩。
func setBeforeModelStatusCallback(sink todoTextSink) llmagent.Option {

	modelCallbacks := model.NewCallbacks().RegisterBeforeModel(
		func(ctx context.Context, args *model.BeforeModelArgs) (*model.BeforeModelResult, error) {
			var status string
			injectBaseStatus(ctx, &status)
			injectTodoStatus(ctx, sink, &status)
			injectMemoryStatus(ctx, args.Request.Messages, &status)

			args.Request.Messages = append(args.Request.Messages, model.NewSystemMessage(status)) //在末尾追加状态栏
			return nil, nil
		},
	)
	return llmagent.WithModelCallbacks(modelCallbacks)
}

// 基础状态栏：时间、工作目录、内存。
func injectBaseStatus(_ context.Context, status *string) {
	//获取时间
	datenow := time.Now().Format("2006-01-02 15:04:05")
	//获取工作目录
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "UNKNOWN"
	}
	//获取总内存、当前内存
	var memTotalStr string
	var memNowStr string
	memoryInfo, err := memory.Get()
	if err == nil {
		memTotalStr = strconv.FormatUint(memoryInfo.Total/1024/1024, 10)
		memNowStr = strconv.FormatUint(memoryInfo.Used/1024/1024, 10)
	} else {
		memTotalStr = "UNKNOWN"
		memNowStr = "UNKNOWN"
	}
	*status += fmt.Sprintf(`[STATUS] TIMENOW: %s , CWD: %s , MEMORY USAGE: %s/%s MB`, datenow, cwd, memNowStr, memTotalStr)
}

// 追加当前agent的todo清单状态（todo_write写入session state，按invocation branch读取，
// 无清单时为空串不追加）。清单变化只影响尾部消息，不破坏前缀缓存；
// 同轮内工具写入后下一跳请求即生效，上下文压缩掉历史后清单也不会丢。
//
// 同一份文本推两个去处：注入 prompt 的 [STATUS] 尾部，以及推给 sink 让 TodoBar 显示。
// 因此 todo.go 不需要任何改动，也不需要渲染两份。
//
// ⚠️ 空串也要推给 sink：清单做完后 TodoStatusBar 返回 ""（todo_write 在全 completed
// 时自动清空清单），TodoBar 靠这个空串把高度收回 0。只在非空时推的话，
// bar 会永远挂着最后一版内容、高度也收不回来。
func injectTodoStatus(ctx context.Context, sink todoTextSink, status *string) {
	todoStatus := functionTools.TodoStatusBar(ctx)
	if sink != nil {
		sink.SetTodoText(todoStatus)
	}
	if todoStatus != "" {
		if *status != "" {
			*status += "\n"
		}
		*status += todoStatus
	}
}

// 根据当前agent推理进度匹配可能相关的记忆：从消息队列尾部回溯抓取本轮 user prompt（头100字）与最近的
// assistant content（尾100字），拼成 query 去 memory service 检索，最多10条追加到状态栏。
// 末尾是 user（新一轮）时 assistant 取的是上一轮的回复；末尾是 assistant/tool（ReAct 中途）
// 时取的是本轮已产出的 assistant content。检索失败或无结果时静默不追加。
func injectMemoryStatus(ctx context.Context, messages []model.Message, status *string) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.MemoryService == nil || inv.Session == nil {
		return
	}
	query := buildMemoryQuery(messages)
	if query == "" {
		return
	}
	userKey := agentmemory.UserKey{AppName: inv.Session.AppName, UserID: inv.Session.UserID}
	entries, err := inv.MemoryService.SearchMemories(ctx, userKey, query,
		agentmemory.WithSearchOptions(agentmemory.SearchOptions{
			Query:               query,
			MaxResults:          memoryMaxResults,
			SimilarityThreshold: 0.03, //定义召回评分，默认是0.3，这个评分在这个场景下太苛刻，改成0.03
		}))
	if err != nil || len(entries) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("[MEMORY]")
	count := 0
	for _, e := range entries {
		if e == nil || e.Memory == nil || strings.TrimSpace(e.Memory.Memory) == "" {
			continue
		}
		line := strings.Join(strings.Fields(e.Memory.Memory), " ")
		sb.WriteString("\n- " + headRunes(line, memoryEntryMaxRunes))
		count++
		if count >= memoryMaxResults {
			break
		}
	}
	if count == 0 {
		return
	}
	if *status != "" {
		*status += "\n"
	}
	*status += sb.String()
}

// buildMemoryQuery 从尾到头回溯消息队列，返回 "user头100字 assistant尾100字"。
// 跳过 system/tool 消息；assistant 仅取文本 content（纯 tool_call 的空消息忽略），
// 多段 assistant 拼接后再截尾。
func buildMemoryQuery(messages []model.Message) string {
	var userPrompt string
	var assistantParts []string
	foundUser := false
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		switch msg.Role {
		case model.RoleUser:
			if foundUser {
				continue
			}
			if content := strings.TrimSpace(msg.Content); content != "" {
				userPrompt = content
				foundUser = true
			}
		case model.RoleAssistant:
			if content := strings.TrimSpace(msg.Content); content != "" {
				assistantParts = append(assistantParts, content)
			}
		}
		// 末尾是 user：继续向上直到拿到上一轮 assistant；末尾是 assistant/tool：拿到本轮 user 即停
		if foundUser && len(assistantParts) > 0 {
			break
		}
	}
	// assistantParts 是倒序收集的，翻转回时间顺序再截尾
	slices.Reverse(assistantParts)
	assistantContent := strings.Join(assistantParts, "\n")

	parts := []string{}
	if userPrompt != "" {
		parts = append(parts, headRunes(userPrompt, memoryQueryRunes))
	}
	if assistantContent != "" {
		parts = append(parts, tailRunes(assistantContent, memoryQueryRunes))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func headRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
