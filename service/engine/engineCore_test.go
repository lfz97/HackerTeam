package engine

// /new 的引擎侧语义：换 session 与清屏必须原子完成；running 拒绝时历史保持原样。

import (
	"encoding/json"
	"testing"
)

func noticeKind(t *testing.T, e *Engine) string {
	t.Helper()
	var n struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(e.NoticeJSON()), &n); err != nil {
		t.Fatalf("notice json: %v", err)
	}
	return n.Kind
}

func TestNewConversationClearsRecords(t *testing.T) {
	e := &Engine{}
	e.appendTyped(RecUser, "old message")
	before := e.Version()

	if !e.NewConversation() {
		t.Fatal("NewConversation must succeed when idle")
	}
	if got := e.Records(); len(got) != 0 {
		t.Fatalf("/new must clear records, got %d: %v", len(got), got)
	}
	if e.Version() <= before {
		t.Fatalf("/new must bump version: before=%d after=%d", before, e.Version())
	}
	if e.SessionId == "" || e.RequestId == "" {
		t.Fatal("/new must reset session/request ids")
	}
	if got := noticeKind(t, e); got != NoticeNewConversation {
		t.Fatalf("notice kind=%q, want %q", got, NoticeNewConversation)
	}
}

func TestNewConversationRejectedWhileRunning(t *testing.T) {
	e := &Engine{}
	e.appendTyped(RecUser, "keep me")
	e.setRunning(true)
	before := e.Version()

	if e.NewConversation() {
		t.Fatal("NewConversation must reject while running")
	}
	if got := e.Records(); len(got) != 1 {
		t.Fatalf("rejected /new must keep records, got %d: %v", len(got), got)
	}
	if e.Version() != before {
		t.Fatalf("rejected /new must not bump version: before=%d after=%d", before, e.Version())
	}
	if got := noticeKind(t, e); got != NoticeWarning {
		t.Fatalf("notice kind=%q, want %q", got, NoticeWarning)
	}
}
