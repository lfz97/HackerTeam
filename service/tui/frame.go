package tui

// 帧落地：pull 链产出的 frameMsg 在这里落到 model 状态上。

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// applyFrame 把 pull 链的一帧落到 model 上。
//
// 续链的 pullCmd 必须无条件排第一个：任何处理路径漏掉它，拉取链就断了。
// 帧不做去重（重复帧是常态，去重交给 renderer 的整帧短路 + cell diff）；
// 本函数的逐字段比较才是防线：未变时不触碰 viewport、不做布局重算。
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

	// 帧无条件应用：各 callee 幂等，且帧只在引擎 version 变化时产生，
	// 再加 diff 守卫只买到亚毫秒的省略，不值。
	// 尾段消费检测（collapse.go）：旧帧有 tail:think、新帧没有 = 流式尾段
	// 被定稿/边界消费，清一次展开状态（定稿即收起）。放在事件循环的状态
	// 迁移处而非 compose 重放路径——重放每帧扫历史记录，会把用户刚点开的
	// 展开抹掉。若新帧也有 tail:think（消费后新尾段已开局）则继承展开态。
	if _, was := spanByID(t.spans, tailThinkID); was {
		if _, still := spanByID(m.spans, tailThinkID); !still {
			t.expand.clearTail()
		}
	}
	t.todoText = m.todo
	t.bottom.SetNotice(m.notice)
	t.spans = m.spans
	t.applyViewText(m.view)
	t.recalcComponentSize()

	// 点击锚定（collapse.go）：折叠切换重组后把目标块头部钉回点击时的
	// 屏幕行，覆盖 applyViewText 的底部跟随——展开上方块时跳底很晃眼。
	// 锚只消费一次；目标块消失（流式重组重排）则放弃锚定走常规跟随。
	// 版本守卫：点击前已在飞的旧帧（expandVer 更小）不能消费锚点——否则
	// 它把点击前的排版落到屏上，锚又已销毁，展开块会把视口顶走。旧帧直接
	// 跳过，锚留给点击后 forceCompose 产出、带新版本的那一帧。
	if a := t.anchor; a != nil && m.expandVer >= a.ver {
		t.anchor = nil
		if s, ok := spanByID(m.spans, a.id); ok {
			t.vp.SetYOffset(s.start - a.row)
		}
	}

	// 终态：waitKey=true 的消息（init 错误等）已随 view 上屏，等任意键退出。
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
// 不做任何改写/重排：compose 的产物即最终形态，宽度只有一个权威。
func (t *TUI) applyViewText(text string) {
	t.viewText = text
	wasBottom := t.vp.AtBottom() // 必须在写入前获取：写入后内容变长，同一滚动位置会被误判成"不在底部"
	t.vp.SetContent(strings.TrimRight(text, "\n"))
	if wasBottom { // 在底部就跳底刷新，实现消息持续跟随
		t.vp.GotoBottom()
	}
}

// componentWidth 消息区/输入区的可用列宽（去掉两侧 padding）。
func (t *TUI) componentWidth() int {
	return t.width - t.leftPadding - t.rightPadding
}

// recalcPadding 宽终端下内容列按 maxTextWidth 封顶后居中：封顶余量均分进
// 两侧 padding，整个内容列作为整体居中（否则 viewport 左对齐会表现为
// "右边填充比左边宽"）。须先于 applyViewText（其按 componentWidth 重排）。
func (t *TUI) recalcPadding() {
	const basePad = 2
	extra := max(0, t.width-2*basePad-maxTextWidth)
	t.leftPadding = basePad + extra/2
	t.rightPadding = basePad + extra - extra/2
}
