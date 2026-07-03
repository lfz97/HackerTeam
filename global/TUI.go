package global

import (
	"HackerTeam/utils/pretty"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// 定义颜色，配色统一来源于 pretty.TuiXxx 常量，确保界面风格统一且美观
var (
	bg          tcell.Color = tcell.GetColor(pretty.TuiBg)          // 整体背景色
	borderColor tcell.Color = tcell.GetColor(pretty.TuiBorderColor) // 边框颜色
	StatusBarBg tcell.Color = tcell.GetColor(pretty.TuiStatusBarBg) // 标题栏背景色
	inputAreaBg tcell.Color = tcell.GetColor(pretty.TuiInputAreaBg) // 输入区背景色
)

func agentPage() tview.Primitive {

	//设置标题状态栏
	StatusBar = tview.NewTextView()
	StatusBar.SetDynamicColors(true).SetWrap(false).SetText(DefaultStatusBarTip)
	StatusBar.SetTextAlign(tview.AlignCenter)
	StatusBar.SetBackgroundColor(StatusBarBg)

	//设置Agent消息显示区
	AgentMessage = tview.NewTextView().
		SetDynamicColors(true). // 启用颜色
		SetScrollable(true).    // 可滚动
		SetWrap(true)

	AgentMessage.SetBackgroundColor(bg) // 设置背景颜色

	//设置底部输入区
	InputArea = tview.NewTextArea().
		SetLabel(`⇒ `).
		SetWrap(true)
	InputArea.SetBackgroundColor(inputAreaBg)
	InputArea.SetTextStyle(tcell.StyleDefault.
		Background(inputAreaBg).                        // 输入区背景色
		Foreground(tcell.GetColor(pretty.TuiMainText))) // 文字颜色

	// 输入区右侧提示
	InputHint = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignRight)
	InputHint.SetBackgroundColor(bg)
	InputHint.SetText("[gray::d]Ctrl+K 帮助[-:-:-]")

	InputRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	InputRow.SetBackgroundColor(bg)
	InputRow.AddItem(InputArea, 0, 1, true)
	InputRow.AddItem(InputHint, 15, 0, false)

	//设置整体布局
	MainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	MainFlex.SetBackgroundColor(bg)
	MainFlex.AddItem(StatusBar, 1, 0, false)    // 顶部的状态栏占1行
	MainFlex.AddItem(AgentMessage, 0, 1, false) // Agent消息区占剩余空间
	MainFlex.AddItem(InputRow, 1, 0, true)      // 底部的输入区+提示

	return MainFlex
}

func PageCreate() {
	// 创建应用实例和页面容器
	app_p = tview.NewApplication()
	pages = tview.NewPages()
	pages.AddPage("AgentPage", agentPage(), true, true) // Agent页面
	InitHelpTable()                                     // 初始化帮助页（通过 SetRoot 切换，不放入 Pages）

	//设置应用根组件
	app_p.SetRoot(pages, true) // true = 全屏模式
	app_p.EnableMouse(true)    //允许接收鼠标事件
	app_p.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

}

// InitHelpTable 初始化斜杠指令帮助页（Table 组件，在 ToggleHelpPage 中通过 SetRoot 全屏展示）
// 左右两栏：左栏为指令名，右栏为功能描述
type HelpItem struct {
	Cmd  string
	Desc string
}

func InitHelpTable() {
	HelpTable = tview.NewTable()
	HelpTable.SetBackgroundColor(bg)
	HelpTable.SetBorder(true)
	HelpTable.SetBorderColor(borderColor)
	HelpTable.SetTitle(" 斜杠指令 — Ctrl+K / Esc 关闭 ")
	HelpTable.SetTitleAlign(tview.AlignLeft)
	HelpTable.SetSelectable(true, false) // 行可选，列不可选

	HelpTable.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.GetColor("#2A3A5C")).
		Foreground(tcell.GetColor(pretty.TuiMainText)))

	HelpTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlK {
			ToggleHelpPage()
			return nil
		}
		return event
	})

	RefreshHelpTable()
}

// RefreshHelpTable 根据当前 helpItems 重建 Table 行数据
func RefreshHelpTable() {
	HelpTable.Clear()

	mainColor := tcell.GetColor(pretty.TuiMainText)
	subColor := tcell.GetColor(pretty.TuiSubText)

	for index, item := range helpItems {
		cmdCell := tview.NewTableCell(item.Cmd).
			SetTextColor(mainColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(0)

		descCell := tview.NewTableCell(item.Desc).
			SetTextColor(subColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(1)

		HelpTable.SetCell(index, 0, cmdCell)
		HelpTable.SetCell(index, 1, descCell)
	}
}


func TuiRun() {
	if err := app_p.Run(); err != nil { // main goroutine 阻塞在事件循环
		fmt.Printf("Error running application: %v\n", err)
	}
}
