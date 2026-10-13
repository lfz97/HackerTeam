package tui

// wire 集成回归测试：engine 侧 wireToolMsg 产出的记录形状（meta 为原始 JSON
// 对象）必须在 TUI 侧完整解析并渲染出工具块。历史上 wireRecord.Meta 曾被
// 定义为 string，导致带 meta 的记录解析失败被静默跳过——所有工具块消失。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"trpc.group/trpc-go/trpc-agent-go/model"

	tools "HackerTeam/service/engine/tools"
)

// engineWireToolRecord 构造与 engine.wireToolMsg 同形的记录 JSON。
func engineWireToolRecord(t *testing.T, typ string, m model.Message, meta json.RawMessage) string {
	t.Helper()
	b, err := json.Marshal(struct {
		Type string          `json:"type"`
		Msg  *model.Message  `json:"msg,omitempty"`
		Meta json.RawMessage `json:"meta,omitempty"`
	}{typ, &m, meta})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWireRecordWithMetaRendersToolBlock(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"write": {Name: "write", In: []string{"Path"}, Out: []string{"BytesWritten"}},
	})
	toolBuf := map[string]*wireToolCall{}

	// 1) assistant 消息携带工具调用（delta 分片形态，两个片段参数按字节续接）
	args := `{"Path":"x.go"}`
	for _, frag := range []string{args[:4], args[4:]} {
		callRec := engineWireToolRecord(t, "delta", model.Message{
			Role: "assistant",
			ToolCalls: []model.ToolCall{{
				ID: "t1",
				Function: model.FunctionDefinitionParam{
					Name:      "write",
					Arguments: []byte(frag),
				},
			}},
		}, nil)
		var rec wireRecord
		if err := json.Unmarshal([]byte(callRec), &rec); err != nil {
			t.Fatalf("call record unmarshal: %v", err)
		}
		var m model.Message
		if err := json.Unmarshal(rec.Msg, &m); err != nil {
			t.Fatalf("call msg unmarshal: %v", err)
		}
		bufferToolCalls(&m, toolBuf)
	}

	// 2) 工具结果记录带 sidecar meta（原始 JSON 对象——回归触发点）
	resultRec := engineWireToolRecord(t, "message", model.Message{
		Role:    "tool",
		ToolID:  "t1",
		Content: `{"BytesWritten":44}`,
	}, json.RawMessage(`{"result":{"BytesWritten":44}}`))
	var rec2 wireRecord
	if err := json.Unmarshal([]byte(resultRec), &rec2); err != nil {
		t.Fatalf("REGRESSION: result record with meta failed to parse: %v", err)
	}
	var m2 model.Message
	if err := json.Unmarshal(rec2.Msg, &m2); err != nil {
		t.Fatalf("result msg unmarshal: %v", err)
	}

	// 3) composeView 同款路径：Role=tool → renderToolResult(meta)
	out := renderToolResult(&m2, toolBuf, 120, string(rec2.Meta))
	if out == "" {
		t.Fatalf("tool block not rendered (entry not matched?)")
	}
	flat := ansi.Strip(out)
	for _, want := range []string{"write", "Path", "x.go", "44"} {
		if !strings.Contains(flat, want) {
			t.Errorf("%q missing from tool block:\n%s", want, flat)
		}
	}
}
