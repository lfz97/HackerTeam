package tui

import (
	"HackerTeam/utils/pretty"
	"slices"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// toggleHelpPage 切换帮助页显示/隐藏。
// 只会在 UI goroutine 上被调用（来自 inputArea / helpTable 的输入捕获器）。
func (t *Tui) toggleHelpPage() {
	ht := t.appLayout.helpTable
	if ht.helpPageVisible {
		t.app.SetRoot(t.appLayout.pages, true)
		t.app.SetFocus(t.appLayout.inputArea)
		ht.helpPageVisible = false
	} else {
		t.refreshhelpTable() // 每次打开时刷新，确保 skills 等动态项可见
		flex := tview.NewFlex().SetDirection(tview.FlexRow)
		flex.SetBackgroundColor(bg)
		flex.AddItem(ht.h, 0, 1, true)
		// SetRoot 内部会 SetFocus(root)，Flex.Focus 再委派给上面 AddItem 时 focus=true
		// 的 table，所以 helpTable 的 Esc/Ctrl+K 捕获器能正常收到按键
		t.app.SetRoot(flex, true)
		ht.helpPageVisible = true
	}
}

func (t *Tui) refreshhelpTable() {
	ht := t.appLayout.helpTable

	// helpItems 可能被引擎 goroutine 的 AddHelpItems 并发追加（loadSkills 在 init 序列里跑），
	// 先在锁内取快照再建单元格，不要持锁操作 table
	ht.mu.Lock()
	items := slices.Clone(ht.helpItems)
	ht.mu.Unlock()

	ht.h.Clear()

	mainColor := tcell.GetColor(pretty.TuiMainText)
	subColor := tcell.GetColor(pretty.TuiSubText)

	for index, item := range items {
		cmdCell := tview.NewTableCell(item.cmd).
			SetTextColor(mainColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(0)

		descCell := tview.NewTableCell(item.desc).
			SetTextColor(subColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(1)

		ht.h.SetCell(index, 0, cmdCell)
		ht.h.SetCell(index, 1, descCell)
	}
}

func (t *Tui) defaultHelpItems() {
	ht := t.appLayout.helpTable
	ht.mu.Lock()
	defer ht.mu.Unlock()
	ht.helpItems = []helpItem{
		{"/new", "开始新对话"},
		{"/exit", "退出程序"},
	}
}
