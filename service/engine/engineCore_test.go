package engine

// /new 的引擎侧语义：只原子重置 session 并追加 /new 回显；
// 清屏是 TUI 的编排职责，引擎不越界。

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

func TestNewConversationResetsSessionKeepsRecords(t *testing.T) {
	e := &Engine{}
	e.appendTyped(RecUser, "old message")
	before := e.Version()

	if !e.NewConversation() {
		t.Fatal("NewConversation must succeed when idle")
	}
	recs := e.Records()
	if len(recs) != 2 {
		t.Fatalf("NewConversation must keep old records and append /new, got %d: %v", len(recs), recs)
	}
	var last struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(recs[1]), &last); err != nil {
		t.Fatalf("slash record json: %v", err)
	}
	if last.Type != RecSlash || last.Text != "/new" {
		t.Fatalf("last record=%+v, want slash /new", last)
	}
	if e.Version() != before+1 {
		t.Fatalf("NewConversation must bump version once: before=%d after=%d", before, e.Version())
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
