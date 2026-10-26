package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// todo 栏出现/变高会压矮视口。压矮前若已贴底，压矮后必须仍然贴底：
// 否则下一帧 applyViewText 读到的 AtBottom 为 false，本轮后续消息与最终
// 回答都停在上屏位置之外（引擎记录是完整的，只是不再跟随）。
func TestRecalcComponentSizeKeepsBottomFollow(t *testing.T) {
	tu := NewTui(&fakeView{})
	ta, _ := NewPrettyTextArea(90, 1, 0)
	tu.ta = ta
	tu.width, tu.height = 100, 24
	tu.recalcComponentSize()

	tu.vp.SetContent(strings.Repeat("line\n", 200))
	tu.vp.GotoBottom()
	if !tu.vp.AtBottom() {
		t.Fatalf("precondition: viewport not at bottom")
	}

	tu.todoText = strings.Repeat("[TODO] item\n", 8)
	tu.recalcComponentSize()
	if !tu.vp.AtBottom() {
		t.Fatalf("viewport lost bottom follow after todo bar shrank it (offset=%d)", tu.vp.YOffset())
	}
}

// 宽终端下内容列按 maxTextWidth 封顶后居中：封顶余量均分到两侧 padding
// （"最大化窗口右边填充更宽"问题的回归）。窄终端行为不变。
func TestRecalcPaddingCentersCappedColumn(t *testing.T) {
	cases := []struct {
		width    int
		wantLeft int // 期望左 padding（允许 ±1 内的奇数余量偏差见 diff 断言）
		wantCW   int // 期望内容列宽
	}{
		{170, 25, 120}, // 最大化宽终端：extra=46 均分 23+23
		{100, 2, 96},   // 窄终端：不足封顶，padding 不变
		{124, 2, 120},  // 恰好贴着封顶
	}
	for _, c := range cases {
		tt := &TUI{leftPadding: 2, rightPadding: 2}
		tt.width = c.width
		tt.recalcPadding()
		if d := tt.leftPadding - tt.rightPadding; d < -1 || d > 1 {
			t.Errorf("width=%d: paddings asymmetric beyond rounding (%d/%d)",
				c.width, tt.leftPadding, tt.rightPadding)
		}
		if tt.leftPadding != c.wantLeft {
			t.Errorf("width=%d: leftPadding=%d, want %d", c.width, tt.leftPadding, c.wantLeft)
		}
		if got := tt.componentWidth(); got != c.wantCW {
			t.Errorf("width=%d: componentWidth=%d, want %d", c.width, got, c.wantCW)
		}
	}
}

func TestRecalcPaddingOddExtra(t *testing.T) {
	// 奇数余量：左右差 1（浏览器/终端 resize 常见的奇偶列宽）
	tt := &TUI{leftPadding: 2, rightPadding: 2}
	tt.width = 171 // extra=47
	tt.recalcPadding()
	if d := tt.leftPadding - tt.rightPadding; d != 0 && d != -1 && d != 1 {
		t.Errorf("odd extra split wrong: %d/%d", tt.leftPadding, tt.rightPadding)
	}
	if got := tt.componentWidth(); got != 120 {
		t.Errorf("componentWidth=%d, want 120", got)
	}
}

func TestComposeHintFitsWidth(t *testing.T) {
	// 底部是单行栏：hint 超宽会折行、改变视口高度。每个宽度档都必须
	// 至少选到一条能放下的完整提示（极窄时退化为单箭头）。
	for _, w := range []int{120, 80, 71, 70, 60, 52, 51, 40, 24, 20, 0} {
		for _, running := range []bool{false, true} {
			h := ansi.Strip(composeHint(running, w))
			limit := max(1, w-hintSpinnerW)
			if got := ansi.StringWidth(h); got > limit {
				t.Errorf("w=%d running=%v: hint width %d > %d: %q", w, running, got, limit, h)
			}
			if h == "" {
				t.Errorf("w=%d running=%v: empty hint", w, running)
			}
		}
	}
}
