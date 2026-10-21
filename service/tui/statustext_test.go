package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 状态消息（error/warn/summary/fatal）此前不折行：长文本单行进 viewport 被
// 横向裁切（SoftWrap=false 只裁不换），含 tab 时测量宽 ≠ 实际宽直接越界。
// 修复：errText/warnText/summaryText/successText/renderFatal 宽度作参数，
// compose 阶段词级折行——与正文（glamour）/思考（Width）/工具（Hardwrap）
// 同签"进 viewport 前排完版"契约。

// assertNoWideLine 断言逐行宽度不超 limit（ANSI 感知）。
func assertNoWideLine(t *testing.T, out string, limit int) {
	t.Helper()
	for i, ln := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(ln); w > limit {
			t.Fatalf("line %d overflows (width %d > %d): %q", i, w, limit, ansi.Strip(ln))
		}
	}
}

func TestErrTextWrapsWithinWidth(t *testing.T) {
	// 120 词英文长错误，60 列预算：徽章占 8 列后仍须逐行不超 60。
	text := strings.Repeat("connection reset by peer while streaming response chunk ", 12)
	out := errText(text, 60)
	assertNoWideLine(t, out, 60)
	lines := strings.Split(out, "\n")
	if len(lines) < 10 {
		t.Fatalf("expected wrapped multi-line output, got %d lines", len(lines))
	}
	// 首行带 ERROR 徽章，续行悬挂缩进对齐首行正文（8 列）。
	if !strings.HasPrefix(ansi.Strip(lines[0]), " ERROR  connection") {
		t.Fatalf("first line missing badge: %q", ansi.Strip(lines[0]))
	}
	for i, ln := range lines[1:] {
		if !strings.HasPrefix(ansi.Strip(ln), strings.Repeat(" ", 8)) {
			t.Fatalf("line %d not hanging-indented: %q", i+1, ansi.Strip(ln))
		}
	}
}

func TestErrTextChineseHardwraps(t *testing.T) {
	// 中文无空格词级折行无从下手，必须回退按列硬折，行宽仍不超预算。
	text := strings.Repeat("连接被对方重置流式响应中断", 30)
	assertNoWideLine(t, errText(text, 40), 40)
}

func TestWarnSummarySuccessWrap(t *testing.T) {
	text := strings.Repeat("skill load failed: ", 20)
	assertNoWideLine(t, warnText(text, 50), 50)
	assertNoWideLine(t, summaryText(text, 50), 50)
	assertNoWideLine(t, successText(text, 50), 50)
	// summary 头行保留。
	if !strings.Contains(summaryText(text, 50), "summary generated:") {
		t.Fatalf("summary header lost")
	}
}

func TestStatusTextTabAndCrExpanded(t *testing.T) {
	// 含 tab/孤立 \r 的错误文本：测量宽低估导致终端越界（tabwrap_test 同款坑），
	// 必须先清洗再折行。
	text := "2026-10-13T14:07:41+08:00\tDEBUG\tprocessor/identity.go:121\tIdentity request processor failed" + strings.Repeat(" retrying", 20)
	out := errText(text, 80)
	assertNoWideLine(t, out, 80)
	if strings.Contains(out, "\t") {
		t.Fatalf("tab not expanded")
	}
	if strings.Contains(out, "\r") {
		t.Fatalf("carriage return not stripped")
	}
}

func TestStatusTextPreservesNewlines(t *testing.T) {
	// 多行错误文本：显式换行结构保留，每行独立折行。
	text := "line one " + strings.Repeat("x", 100) + "\nline two"
	out := warnText(text, 40)
	assertNoWideLine(t, out, 40)
	if got := len(strings.Split(out, "\n")); got < 4 {
		t.Fatalf("expected structure preserved and wrapped, got %d lines", got)
	}
}
