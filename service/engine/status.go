package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	functionTools "HackerTeam/service/engine/tools/functions"

	"github.com/mackerelio/go-osstat/memory"
	"github.com/rivo/uniseg"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	agentmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	memoryQuerySentences = 5
	memoryChannelResults = 5 // 每路 5 条，双路合计最多 10 条
)

// todoTextSink 接收渲染好的 todo 文本，供 TUI 的 TodoBar 显示。
// 在 engine 包内声明（1 个方法），避免把 init.go 的 tuiService 胖接口渗进调用方——
// 状态回调只需要"能把文本推给 UI"这一个能力。
// Worker 装配时 withTodo=false：它没有 todo 工具，也不该去读 Operator 的计划。
type todoTextSink interface {
	SetTodoText(text string)
}

// beforeModelStatus 注入模型调用前的状态栏（时间、工作目录、内存，并按 spec 追加
// todo 清单、相关记忆、在途任务）。Operator 全开；Worker 只留基础状态——它是一次性
// 工人，看得到机器状态就够，不需要计划、记忆和兄弟任务的进度。
//
// 注意：追加在末尾而非前置 —— 自动前缀缓存要求请求头部保持稳定，
// 状态栏每次调用内容变化，放头部会破坏整个前缀缓存（实测：尾部99%命中 vs 头部0）。
// 使用本功能必须关闭框架的 system 前置重排（openai.WithOptimizeForCache），否则
// 尾部状态栏会被框架挪回头部、缓存收益失效；关闭位置：service/engine/models/openai.go。
// 状态栏不进 session（仅存在于当次请求副本），不污染摘要/上下文压缩。
func (e *Engine) beforeModelStatus(spec agentSpec) llmagent.Option {

	modelCallbacks := model.NewCallbacks().RegisterBeforeModel(
		func(ctx context.Context, args *model.BeforeModelArgs) (*model.BeforeModelResult, error) {
			var status string
			injectBaseStatus(ctx, &status)
			if spec.withTodo {
				injectTodoStatus(ctx, spec.todoSink, &status)
			}
			if spec.withMemory {
				injectMemoryStatus(ctx, args.Request.Messages, &status)
			}
			if spec.withFlight {
				(*e).injectFlightStatus(ctx, &status)
			}

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

// 每次模型调用独立检索：用户完整输入召回相似任务，当前轮所有 reasoning 的尾部
// 五句召回执行现场经验。reasoning 不存在时只查用户输入，不使用 content 兜底。
func injectMemoryStatus(ctx context.Context, messages []model.Message, status *string) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.MemoryService == nil || inv.Session == nil {
		return
	}
	userQuery, reasoningQuery := buildMemoryQueries(messages)
	if userQuery == "" {
		return
	}
	userKey := agentmemory.UserKey{AppName: inv.Session.AppName, UserID: inv.Session.UserID}
	search := func(query string) ([]*agentmemory.Entry, error) {
		return inv.MemoryService.SearchMemories(ctx, userKey, query,
			agentmemory.WithSearchOptions(agentmemory.SearchOptions{
				Query:               query,
				MaxResults:          memoryChannelResults,
				SimilarityThreshold: 0.03,
			}))
	}

	userEntries, err := search(userQuery)
	if err != nil {
		log.Printf("[MEMORY] turn=%s user search failed: %v", inv.InvocationID, err)
		return
	}
	var reasoningEntries []*agentmemory.Entry
	if reasoningQuery != "" {
		reasoningEntries, err = search(reasoningQuery)
		if err != nil {
			log.Printf("[MEMORY] turn=%s reasoning search failed: %v", inv.InvocationID, err)
			return
		}
	}

	block, logLine := renderMemoryEntries(userEntries, reasoningEntries, reasoningQuery != "")
	log.Printf("[MEMORY] turn=%s user_query=%q reasoning_query=%q %s",
		inv.InvocationID, userQuery, reasoningQuery, logLine)
	appendStatusBlock(status, block)
}

// buildMemoryQueries 收集当前轮所有 assistant 的 reasoning，遇到最近 user 即停。
func buildMemoryQueries(messages []model.Message) (string, string) {
	var reasoningParts []string
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		switch msg.Role {
		case model.RoleUser:
			if content := strings.TrimSpace(msg.Content); content != "" {
				slices.Reverse(reasoningParts)
				reasoning := strings.Join(reasoningParts, " ")
				return content, tailSentences(reasoning, memoryQuerySentences)
			}
		case model.RoleAssistant:
			if reasoning := strings.TrimSpace(msg.ReasoningContent); reasoning != "" {
				reasoningParts = append(reasoningParts, reasoning)
			}
		}
	}
	return "", ""
}

func tailSentences(s string, maxSentences int) string {
	rest := strings.TrimSpace(s)
	if rest == "" || maxSentences <= 0 {
		return ""
	}
	var sentences []string
	state := -1
	for rest != "" {
		var seg string
		seg, rest, state = uniseg.FirstSentenceInString(rest, state)
		if seg == "" {
			break
		}
		if sentence := strings.TrimSpace(seg); sentence != "" {
			sentences = append(sentences, sentence)
		}
	}
	if len(sentences) > maxSentences {
		sentences = sentences[len(sentences)-maxSentences:]
	}
	return strings.Join(sentences, " ")
}

// renderMemoryEntries 分渠道展示并按渠道内最高分归一化；不同 query 的原始分不直接比较。
func renderMemoryEntries(userEntries, reasoningEntries []*agentmemory.Entry, includeReasoning bool) (string, string) {
	seen := make(map[string]bool)
	userEntries = prepareMemoryEntries(userEntries, seen)
	reasoningEntries = prepareMemoryEntries(reasoningEntries, seen)

	var sb strings.Builder
	sb.WriteString("[MEMORY]")
	appendMemoryChannel(&sb, "USER", userEntries)
	logLine := fmt.Sprintf("user=%d %s", len(userEntries), memoryEntryTags(userEntries))
	if includeReasoning {
		appendMemoryChannel(&sb, "REASONING", reasoningEntries)
		logLine += fmt.Sprintf(" reasoning=%d %s", len(reasoningEntries), memoryEntryTags(reasoningEntries))
	}
	return sb.String(), strings.TrimSpace(logLine)
}

func prepareMemoryEntries(entries []*agentmemory.Entry, seen map[string]bool) []*agentmemory.Entry {
	prepared := make([]*agentmemory.Entry, 0, memoryChannelResults)
	for _, entry := range entries {
		if entry == nil || entry.Memory == nil || strings.TrimSpace(entry.Memory.Memory) == "" || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		prepared = append(prepared, entry)
		if len(prepared) == memoryChannelResults {
			break
		}
	}
	return prepared
}

func appendMemoryChannel(sb *strings.Builder, label string, entries []*agentmemory.Entry) {
	sb.WriteString("\n[" + label + "]")
	if len(entries) == 0 {
		sb.WriteString(" 0 hits")
		return
	}
	maxScore := 0.0
	for _, entry := range entries {
		if entry.Score > maxScore {
			maxScore = entry.Score
		}
	}
	for _, entry := range entries {
		percentage := 0
		if maxScore > 0 {
			percentage = int(entry.Score/maxScore*100 + 0.5)
		}
		sb.WriteString(fmt.Sprintf("\n- 可能与当前相关（%d%%）：%s", percentage, entryText(entry)))
	}
}

func memoryEntryTags(entries []*agentmemory.Entry) string {
	tags := make([]string, 0, len(entries))
	for _, entry := range entries {
		tags = append(tags, fmt.Sprintf("%s:%.2f", entry.ID, entry.Score))
	}
	return strings.Join(tags, " ")
}

func appendStatusBlock(status *string, block string) {
	if block == "" {
		return
	}
	if *status != "" {
		*status += "\n"
	}
	*status += block
}

func entryText(e *agentmemory.Entry) string {
	if e == nil || e.Memory == nil {
		return ""
	}
	return strings.Join(strings.Fields(e.Memory.Memory), " ")
}
