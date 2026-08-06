package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"HackerTeam/utils/pretty"
)

// ToggleHelpPage 切换帮助页显示/隐藏
func (t *Tui) toggleHelpPage() {
	if (*(*(*t).appLayout).helpTable).helpPageVisible {
		(*t).app.SetRoot((*(*t).appLayout).pages, true)
		(*t).app.SetFocus((*(*t).appLayout).inputArea)
		(*(*(*t).appLayout).helpTable).helpPageVisible = false
	} else {
		t.refreshhelpTable() // 每次打开时刷新，确保 skills 等动态项可见
		flex := tview.NewFlex().SetDirection(tview.FlexRow)
		flex.SetBackgroundColor(bg)
		flex.AddItem((*(*(*t).appLayout).helpTable).h, 0, 1, true)
		(*t).app.SetRoot(flex, true)
		(*(*(*t).appLayout).helpTable).helpPageVisible = true
	}
}

func (t *Tui) refreshhelpTable() {
	(*(*t).appLayout).helpTable.h.Clear()

	mainColor := tcell.GetColor(pretty.TuiMainText)
	subColor := tcell.GetColor(pretty.TuiSubText)

	for index, item := range (*(*(*t).appLayout).helpTable).helpItems {
		cmdCell := tview.NewTableCell(item.cmd).
			SetTextColor(mainColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(0)

		descCell := tview.NewTableCell(item.desc).
			SetTextColor(subColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(1)

		(*(*t).appLayout).helpTable.h.SetCell(index, 0, cmdCell)
		(*(*t).appLayout).helpTable.h.SetCell(index, 1, descCell)
	}
}

func (t *Tui) defaultHelpItems() {
	(*((*t).appLayout).helpTable).helpItems = []helpItem{
		{"/new", "开始新对话"},
		{"/exit", "退出程序"},
	}
}
