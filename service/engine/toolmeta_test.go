package engine

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestToolSidecarRecordAndConsume(t *testing.T) {
	s := newToolSidecar()
	cb := s.afterTool()

	// 正常记录：原始 Result 按 ToolCallID 存取
	if _, err := cb(context.Background(), &tool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "edit_file",
		Result:     map[string]string{"Diff": "--- a\n+++ b\n"},
	}); err != nil {
		t.Fatalf("afterTool error: %v", err)
	}
	raw := s.consume("call_1")
	if raw == nil {
		t.Fatalf("sidecar empty for call_1")
	}
	var payload struct {
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if payload.Result["Diff"] != "--- a\n+++ b\n" {
		t.Errorf("raw result lost: %#v", payload.Result)
	}
	// consume 后清空（wireRecord 只挂一次）
	if s.consume("call_1") != nil {
		t.Errorf("consume should be take-once")
	}
	// 未知 ID
	if s.consume("nope") != nil {
		t.Errorf("unknown id should return nil")
	}
}

func TestToolSidecarEmptyIDAndMeta(t *testing.T) {
	s := newToolSidecar()
	cb := s.afterTool()
	// 空 ToolCallID 不记录
	if _, err := cb(context.Background(), &tool.AfterToolArgs{ToolName: "x"}); err != nil {
		t.Fatalf("error: %v", err)
	}
	if s.consume("") != nil {
		t.Errorf("empty id should not be recorded")
	}
	// MCP _meta 透传
	if _, err := cb(context.Background(), &tool.AfterToolArgs{
		ToolCallID: "call_2",
		Result:     "ok",
		Meta:       map[string]any{"_meta": map[string]any{"k": "v"}},
	}); err != nil {
		t.Fatalf("error: %v", err)
	}
	raw := s.consume("call_2")
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["result"] != "ok" {
		t.Errorf("result wrong: %v", payload["result"])
	}
	if _, ok := payload["meta"]; !ok {
		t.Errorf("meta not carried")
	}
}

func TestToolSidecarCapacityAndConcurrent(t *testing.T) {
	s := newToolSidecar()
	cb := s.afterTool()
	// 容量兜底：超过 64 条整体清空
	for i := 0; i < 70; i++ {
		_, _ = cb(context.Background(), &tool.AfterToolArgs{
			ToolCallID: "call_" + string(rune('a'+i%26)) + string(rune(i)),
			Result:     i,
		})
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cb(context.Background(), &tool.AfterToolArgs{ToolCallID: "cx", Result: 1})
			_ = s.consume("cx")
		}()
	}
	wg.Wait()
}
