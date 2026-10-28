package tui

// 全量重放性能基准：真实规模会话（20 轮×重内容）测量 composeView 成本。
// ms 级差异人不可感知，缓存已全部删除；本基准的真正用途是诊断规模化悬崖
// （全量重放随会话长度线性增长）——数百轮重会话感到卡顿时先跑这里拿基线。
//
// 运行：go test ./service/tui/ -run XXX -bench BenchmarkCompose -benchtime 5x -count 3
// 帧预算参考：单次 compose 冷成本 < 30ms 流畅；> 100ms 可感知卡顿
//（composeInterval=100ms、pullInterval=30ms）。

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// fakeView 固定返回预置记录集的假引擎（只实现基准需要的部分）。
type fakeView struct {
	records   []string
	denyNew   bool // NewConversation 模拟 running 拒绝
	denyClear bool // ClearRecords 模拟 running 拒绝
}

func (f *fakeView) Version() uint64               { return 1 }
func (f *fakeView) Records() []string             { return f.records }
func (f *fakeView) RunStateJSON() string          { return `{"running":false}` }
func (f *fakeView) TodoText() string              { return "" }
func (f *fakeView) NoticeJSON() string            { return `{"kind":"none"}` }
func (f *fakeView) StartupInfo() ([]string, bool) { return nil, false }
func (f *fakeView) SubmitInput(line string) bool  { return true }
func (f *fakeView) Interrupt() bool               { return true }

// ── 斜杠命令的能力接口（注册表与分发在前端 slash.go）──
func (f *fakeView) Shutdown()                       {}
func (f *fakeView) NewConversation() bool           { return !f.denyNew }
func (f *fakeView) ClearRecords() bool              { return !f.denyClear }
func (f *fakeView) ListResumeItems() string         { return "[]" }
func (f *fakeView) CheckoutSession(id string) error { return nil }

// ── 首次启动配置页 ──
func (f *fakeView) FirstStartupJSON() string { return `{"state":"ready"}` }
func (f *fakeView) ApplyFirstStartupConfig(vals map[string]string) error {
	return nil
}

// buildSession 构造真实规模的会话：20 轮 ×（用户输入 + 思考 + 表格正文 + 工具调用与结果）。
func buildSession() []string {
	var recs []string
	wire := func(typ string, m model.Message) string {
		b, _ := json.Marshal(struct {
			Type string         `json:"type"`
			Msg  *model.Message `json:"msg,omitempty"`
		}{typ, &m})
		return string(b)
	}
	typed := func(typ, text string) string {
		b, _ := json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		}{typ, text})
		return string(b)
	}

	tableRow := func(i int) string {
		return fmt.Sprintf("| 检查项%d Some Item | 状态 | 结果列放中英文混合长文本 Linux / amd64 用户 lfz 当前目录 /home/lfz/hyperbot 第 %d 行 |", i, i)
	}
	var table strings.Builder
	table.WriteString("| 检查项 | 状态 | 结果 |\n|---|---|---|\n")
	for i := 0; i < 10; i++ {
		table.WriteString(tableRow(i) + "\n")
	}

	diff := "--- a/x.go\n+++ b/x.go\n@@ -10,4 +10,6 @@\n ctx line here\n-old code one\n-old code two\n+new code one\n+new code two\n+new code three\n ctx line tail\n"
	code := strings.Repeat("func handler(n int) error {\n\tif n == 0 {\n\t\treturn nil\n\t}\n\treturn handler(n - 1)\n}\n\n", 12) // ~120 行
	output := ""
	for i := 0; i < 60; i++ {
		output += fmt.Sprintf("2026-10-13T12:21:%02d+08:00    DEBUG    processor/functioncall.go:2748    Executing tool WriteFile with args: {\"Path\": \"/home/lfz/hyperbot/x.go\"}\n", i%60)
	}
	thinking := strings.Repeat("用户要求自检。我需要逐项验证：环境、文件系统、记忆、定时任务。先并行调用 LS 与 Glob 获取基线，再依次测试文件读写与编辑，最后清理测试产物。\n", 30)

	for turn := 0; turn < 20; turn++ {
		recs = append(recs, typed("user", fmt.Sprintf("第 %d 轮：请做一次完整自检，包括环境、文件、记忆与定时任务", turn)))
		recs = append(recs, wire("message", model.Message{
			Role:             "assistant",
			ReasoningContent: thinking,
			ToolCalls: []model.ToolCall{
				{ID: fmt.Sprintf("t%d-a", turn), Function: model.FunctionDefinitionParam{Name: "edit", Arguments: []byte(fmt.Sprintf(`{"path":"/home/lfz/hyperbot/service/main.go","old":"x%d","new":"y%d"}`, turn, turn))}},
				{ID: fmt.Sprintf("t%d-b", turn), Function: model.FunctionDefinitionParam{Name: "write", Arguments: []byte(fmt.Sprintf(`{"path":"/tmp/x%d.go","content":%s}`, turn, strconv.Quote(code)))}},
				{ID: fmt.Sprintf("t%d-c", turn), Function: model.FunctionDefinitionParam{Name: "CommandRun", Arguments: []byte(fmt.Sprintf(`{"process":"bash","args":["-c","tail -n 50 log%d.log"]}`, turn))}},
			},
		}))
		recs = append(recs, wire("message", model.Message{Role: "tool", ToolID: fmt.Sprintf("t%d-a", turn), Content: `{"diff":` + strconv.Quote(diff) + `}`}))
		recs = append(recs, wire("message", model.Message{Role: "tool", ToolID: fmt.Sprintf("t%d-b", turn), Content: fmt.Sprintf(`{"wrote":%d,"content":%s}`, len(code), strconv.Quote(code))}))
		recs = append(recs, wire("message", model.Message{Role: "tool", ToolID: fmt.Sprintf("t%d-c", turn), Content: fmt.Sprintf(`{"status":"done","exitCode":0,"id":"p%d","output":%s}`, turn, strconv.Quote(output))}))
		recs = append(recs, wire("message", model.Message{Role: "assistant", Content: "# 自检报告\n\n" + table.String() + "\n结论：全部正常。" + strings.Repeat(" 详细说明段落，包含中英文混合内容与 markdown 强调。", 20)}))
	}
	return recs
}

func newBenchTUI() *TUI {
	t := NewTui(&fakeView{records: buildSession()})
	t.width = 124
	t.height = 40
	return t
}

// BenchmarkComposeWarm 稳态：缓存全命中的每帧全量重放（当前架构的持续成本）。
func BenchmarkComposeWarm(b *testing.B) {
	t := newBenchTUI()
	st := newPullState()
	st.width = 120
	_, _ = t.composeView(st) // 预热
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.composeView(st)
	}
}
