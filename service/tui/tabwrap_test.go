package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ANSI 宽度测量把 \t 计为 0 列，终端展开到 8 列制表位——含 tab 的行
// "测量达标、实际超宽"，越过 padding 被终端硬换行（词中断裂的根因）。
// 修复：内容入口（cleanBlock/compactLine）统一把 tab 展开为 4 空格
// （crush diffview 的 replaceTabs 同款）。
func TestCleanBlockExpandsTabs(t *testing.T) {
	if got := cleanBlock("a\tb\tc"); got != "a    b    c" {
		t.Fatalf("cleanBlock tabs = %q, want expanded to 4 spaces", got)
	}
}

func TestCompactLineExpandsTabs(t *testing.T) {
	if got := compactLine("a\tb"); !strings.HasPrefix(got, "a") || strings.Contains(got, "\t") {
		t.Fatalf("compactLine keeps tabs: %q", got)
	}
}

func TestBlockWithTabsNoOverflow(t *testing.T) {
	// 真实形态：日志行含 tab 分隔（测量低估 → 旧版不折行 → 终端越界）
	logLine := "2026-10-13T14:07:41+08:00\tDEBUG\tprocessor/identity.go:121\tIdentity request processor: sent preprocessing event"
	out := toolView("LocalExec_run",
		[]byte(`{"Process":"bash","Args":["-c","tail"]}`),
		`{"Id":"x","Status":"done","ExitCode":0,"Output":`+quoteJSON(logLine)+`}`,
		110, "")
	padded := msgIndent.Render(out)
	for i, ln := range strings.Split(padded, "\n") {
		if w := ansi.StringWidth(ln); w > 110 {
			t.Fatalf("line %d overflows (width %d): %q", i, w, ansi.Strip(ln))
		}
		if strings.Contains(ln, "\t") {
			t.Fatalf("line %d still contains tab", i)
		}
	}
}

// quoteJSON 把含换行/引号的文本编码为 JSON 字符串字面量（测试构造用）。
func quoteJSON(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return `"` + b.String() + `"`
}
