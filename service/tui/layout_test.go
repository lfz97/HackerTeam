package tui

import "testing"

// 宽终端下内容列按 maxTextWidth 封顶后居中：封顶余量均分到两侧 padding，
// 最大化窗口时左右留白对称（此前 viewport 左对齐把余量全堆在右侧，
// "右边填充更宽"的根因）。窄终端（不足 120 列）行为不变。
func TestRecalcPaddingCentersCappedColumn(t *testing.T) {
	cases := []struct {
		width    int
		wantLeft int  // 期望左 padding（允许 ±1 内的奇数余量偏差见 diff 断言）
		wantCW   int  // 期望内容列宽
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
