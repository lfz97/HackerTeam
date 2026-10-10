package tui

import (
	"strings"
)

// bottom 底部单行栏（demo 同款）：spinner + gap + notice。
// notice 是已渲染的最终形态文本（由 frameMsg 从 pull 循环投递）。
type bottom struct {
	spinner *spinner
	gap     int
	notice  string
}

func (b *bottom) UpdateGap(gap int) {
	b.gap = gap
}

// SetNotice 更新通知文本（调用方保证只在事件循环上）。
func (b *bottom) SetNotice(notice string) {
	b.notice = notice
}

// spinnerView 暴露 spinner 视图给宽度计算。
func (b *bottom) spinnerView() string {
	return b.spinner.View()
}

func (b *bottom) View() string {
	// 终端过窄或启动早期(componentWidth 尚未就绪)时 gap 可能为负,
	// strings.Repeat 遇负数会 panic,这里夹到 0 兜底
	gap := max(0, b.gap)
	return b.spinner.View() + strings.Repeat(" ", gap) + b.notice
}
