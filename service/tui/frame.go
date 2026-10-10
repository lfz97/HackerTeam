package tui

// 帧落地（操作类）：pull 链产出的 frameMsg 在这里落到 model 状态上，
// 以及视图文本写入 viewport 的相关操作。

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// applyFrame 把 pull 链的一帧落到 model 上。
// 续链的 pullCmd 必须无条件排第一个：任何处理路径漏掉它，拉取链就断了。
//
// 帧不做去重，重复帧是常态（30ms 一帧、重组 100ms 节流）——脏活框架做：
// renderer 的 flush 有 viewEquals 整帧短路 + cell 级 diff。本函数的逐字段比较
// 才是真正的防线：view/todo/notice 未变时不触碰 viewport、不做布局重算。
func (t *TUI) applyFrame(m frameMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{t.pullCmd()}

	changed := false
	if m.running != t.running {
		t.running = m.running
		changed = true
		if m.running {
			cmds = append(cmds, t.bottom.spinner.Start())
		} else {
			t.bottom.spinner.Stop()
		}
	}

	if m.todo != t.todoText {
		t.todoText = m.todo
		changed = true
	}

	if m.notice != t.bottom.notice {
		t.bottom.SetNotice(m.notice)
		changed = true
	}

	if m.view != t.viewText {
		t.applyViewText(m.view)
		changed = true
	}

	if changed {
		t.recalcComponentSize()
	}

	// 终态：waitKey=true 的消息（init 错误等）已随 view 上屏、等任意键退出；
	// waitKey=false（/exit）无告别语，直接退出。
	if m.fatal != nil && !t.fatalHandled {
		t.fatalHandled = true
		if m.fatal.WaitKey {
			t.waitingKey = true // 任意键退出（keyMsgHandler 里拦截）
		} else {
			return t, tea.Quit
		}
	}
	return t, tea.Batch(cmds...)
}

// applyViewText 把视图全文写入 viewport（按当前宽度软换行，视口底部跟随）。
func (t *TUI) applyViewText(text string) {
	t.viewText = text
	wasBottom := t.vp.AtBottom() // 必须在写入前获取：写入后内容变长，同一滚动位置会被误判成"不在底部"
	t.vp.SetContent(wrapViewText(text, t.componentWidth()))
	if wasBottom { //在底部就跳底刷新，实现消息持续跟随
		t.vp.GotoBottom()
	}
}

// componentWidth 消息区/输入区的可用列宽（去掉两侧 padding）。
func (t *TUI) componentWidth() int {
	return t.width - t.leftPadding - t.rightPadding
}

// wrapViewText 按宽度软换行。提交时用 lipgloss 根据窗口宽度软换行，
// 不在 viewport 渲染时软换行，提高性能（长文本只重排一次而不是每帧重算）。
func wrapViewText(text string, width int) string {
	if width <= 0 {
		return strings.TrimRight(text, "\n")
	}
	return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(text, "\n"))
}
