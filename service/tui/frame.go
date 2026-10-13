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

// recalcPadding 宽终端下内容列按 maxTextWidth 封顶后居中。消息内容在
// composeView 里以 min(宽度, 120) 封顶换行（可读性 cap），viewport 比
// 内容列宽时按左对齐展示，余量全堆在右侧——最大化窗口时表现为"右边
// 填充比左边宽"。修法：封顶余量均分进两侧 padding，整个内容列
// （消息/输入框/清单栏/底栏）作为整体居中，左右留白对称。窄终端
// （不足 120 列）不触发，行为与旧版一致。仅在 WindowSizeMsg（宽度
// 变化）时调用，须先于 applyViewText（其按 componentWidth 重排）。
func (t *TUI) recalcPadding() {
	const basePad = 2
	extra := max(0, t.width-2*basePad-maxTextWidth)
	t.leftPadding = basePad + extra/2
	t.rightPadding = basePad + extra - extra/2
}

// wrapViewText 按宽度软换行。提交时用 lipgloss 根据窗口宽度软换行，
// 不在 viewport 渲染时软换行，提高性能（长文本只重排一次而不是每帧重算）。
// tab 展开是兜底安全网：任何来源（思考/正文/用户粘贴/工具块）的 tab 都在
// 这里展开——ANSI 宽度测量把 \t 计 0 列而终端展开 8 列制表位，不展开则
// 测量低估导致越界（cleanBlock/compactLine 已处理各自入口，此处兜住其余）。
func wrapViewText(text string, width int) string {
	if width <= 0 {
		return strings.TrimRight(text, "\n")
	}
	return lipgloss.NewStyle().Width(width).Render(
		strings.ReplaceAll(strings.TrimRight(text, "\n"), "\t", tabSpaces))
}
