package tui

// wire 集成回归测试：engine 侧 wireRecord 产出的记录形状必须在 TUI 侧完整
// 解析并渲染出工具块（delta 分片参数续接 + Role=tool 结果配对）。历史上
// 记录字段类型不匹配曾导致解析失败被静默跳过——所有工具块消失。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"trpc.group/trpc-go/trpc-agent-go/model"

	tools "HackerTeam/service/engine/tools"
)

// engineWireToolRecord 构造与 engine.wireMsg 同形的记录 JSON。
func engineWireToolRecord(t *testing.T, typ string, m model.Message) string {
	t.Helper()
	b, err := json.Marshal(struct {
		Type string         `json:"type"`
		Msg  *model.Message `json:"msg,omitempty"`
	}{typ, &m})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWireRecordWithMetaRendersToolBlock(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"write": {Name: "write", In: []string{"Path"}, Out: []string{"BytesWritten"}},
	})

	// 1) assistant 消息携带工具调用（delta 分片形态，两个片段参数按字节续接）+
	//    2) 工具结果记录：与 engine.wireMsg 同形的 wire JSON 原样进假引擎。
	args := `{"Path":"x.go"}`
	callRec1 := engineWireToolRecord(t, "delta", model.Message{
		Role: "assistant",
		ToolCalls: []model.ToolCall{{
			ID:       "t1",
			Function: model.FunctionDefinitionParam{Name: "write", Arguments: []byte(args[:4])},
		}},
	})
	callRec2 := engineWireToolRecord(t, "delta", model.Message{
		Role: "assistant",
		ToolCalls: []model.ToolCall{{
			ID:       "t1",
			Function: model.FunctionDefinitionParam{Name: "write", Arguments: []byte(args[4:])},
		}},
	})
	resultRec := engineWireToolRecord(t, "message", model.Message{
		Role:    "tool",
		ToolID:  "t1",
		Content: `{"BytesWritten":44}`,
	})

	// 3) composeView 全链路：记录进假引擎 → 默认折叠只出头部行 →
	//    展开（点开）后完整内容（Path/x.go/BytesWritten 值）可见。
	fake := &fakeView{records: []string{callRec1, callRec2, resultRec}}
	ft := NewTui(fake)
	ft.pull.width = 120
	view, spans := ft.composeView(ft.pull)
	flat := ansi.Strip(view)
	if !strings.Contains(flat, "write") || !strings.Contains(flat, "x.go") {
		t.Fatalf("collapsed tool header missing name/path:\n%s", flat)
	}
	if strings.Contains(flat, "44") {
		t.Fatalf("collapsed tool block leaked result content:\n%s", flat)
	}
	if len(spans) != 1 || spans[0].id != "tool:2" {
		t.Fatalf("want one tool span with stable id, got %#v", spans)
	}
	ft.expand.toggle("tool:2")
	view, spans = ft.composeView(ft.pull)
	flat = ansi.Strip(view)
	for _, want := range []string{"write", "Path", "x.go", "44"} {
		if !strings.Contains(flat, want) {
			t.Errorf("%q missing from expanded tool block:\n%s", want, flat)
		}
	}
	if len(spans) != 1 || spans[0].start != 0 {
		t.Fatalf("expanded block header must stay at line 0, got %#v", spans)
	}
}
