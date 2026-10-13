package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 词级软换行救不了无空格长 token（JSON 串/长路径）：resize 后按新宽度
// 重排时必须按列硬折，否则超宽行漏给终端硬换行（词中断裂、丢缩进）。
func TestWrapViewTextHardWrapsLongTokens(t *testing.T) {
	token := strings.Repeat("i", 200) + " tail"
	wrapped := wrapViewText("head "+token, 50)
	for i, ln := range strings.Split(wrapped, "\n") {
		if w := ansi.StringWidth(ln); w > 50 {
			t.Fatalf("line %d overflows (width %d): %q", i, w, ln)
		}
	}
}

// wrapViewText 是所有内容进 viewport 的最终宽度闸口：tab 必须在此展开
// （测量 0 列 vs 终端 8 列展开的低估是越界根因之一）。
func TestWrapViewTextExpandsTabs(t *testing.T) {
	wrapped := wrapViewText(strings.Repeat("a", 40)+"\t"+strings.Repeat("b", 40), 50)
	for i, ln := range strings.Split(wrapped, "\n") {
		if strings.Contains(ln, "\t") {
			t.Fatalf("line %d still contains tab", i)
		}
		if w := ansi.StringWidth(ln); w > 50 {
			t.Fatalf("line %d overflows (width %d)", i, w)
		}
	}
}
