package tui

// 帧落地（操作类）：pull 链产出的 frameMsg 在这里落到 model 状态上，
// 以及视图文本写入 viewport 的相关操作。

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// applyFrame 把 pull 链的一帧落到 model 上。
// 续链的 pullCmd 必须无条件排第一个：任何处理路径漏掉它，拉取链就断了。
//
// 帧不做去重，重复帧是常态（30ms 一帧、重组 100ms 节流）——脏活框架做：
// renderer 的 flush 有 viewEquals 整帧短路 + cell 级 diff。本函数的逐字段比较
// 才是真正的防线：view/todo/notice 未变时不触碰 viewport、不做布局重算。
func (t *TUI) applyFrame(m frameMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{t.pullCmd()}

	if m.running != t.running { // spinner 门控是行为语义（Start/Stop），非优化
		t.running = m.running
		if m.running {
			cmds = append(cmds, t.bottom.spinner.Start())
		} else {
			t.bottom.spinner.Stop()
		}
	}

	// 帧无条件应用：各 callee 幂等（SetNotice 纯赋值且 TTL 计时在引擎侧；
	// applyViewText 相同文本滚动语义不变；recalc 同输入同布局），且帧只在
	// 引擎 version 变化时产生——无风暴可挡，diff 判定只买到亚毫秒的省略，
	// 感知口径下不值一层守卫（消融论证见提交记录）。
	t.todoText = m.todo
	t.bottom.SetNotice(m.notice)
	t.applyViewText(m.view)
	t.recalcComponentSize()

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

// applyViewText 把视图全文写入 viewport（视口底部跟随）。
// 不做任何改写/重排：compose 的产物即最终形态（宽度只有一个权威）。
// resize 后旧宽度帧原样显示一瞬（≤一个 pull 周期），widthChanged 重组
// 随即替换——宁可短暂旧排版，不做静默内容改写。
func (t *TUI) applyViewText(text string) {
	t.viewText = text
	wasBottom := t.vp.AtBottom() // 必须在写入前获取：写入后内容变长，同一滚动位置会被误判成"不在底部"
	t.vp.SetContent(strings.TrimRight(text, "\n"))
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


