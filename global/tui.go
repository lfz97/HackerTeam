package global

import (
	"HackerTeam/utils/pretty"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const banner string = `    //    / /                                  //   ) )
   //___ / /         ___      ___      __     //___/ /   ___    __  ___
  / ___   //   / / //   ) ) //___) ) //  ) ) / __  (   //   ) )  / /
 //    / ((___/ / //___/ / //       //      //    ) ) //   / /  / /
//    / /    / / //       ((____   //      //____/ / ((___/ /  / /
`

// 定义颜色，配色统一来源于 pretty.TuiXxx 常量，确保界面风格统一且美观
var (
	bg          tcell.Color = tcell.GetColor(pretty.TuiBg)          // 整体背景色
	SidebarBg   tcell.Color = tcell.GetColor(pretty.TuiPanelBg)     // 侧边栏背景色
	borderColor tcell.Color = tcell.GetColor(pretty.TuiBorderColor) // 边框颜色
	StatusBarBg tcell.Color = tcell.GetColor(pretty.TuiStatusBarBg) // 标题栏背景色
	inputAreaBg tcell.Color = tcell.GetColor(pretty.TuiInputAreaBg) // 输入区背景色
)

func CreateConfigPage() tview.Primitive {
	// Banner 区域
	bannerBar = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetTextAlign(tview.AlignCenter).
		SetText(pretty.TColoredText(pretty.TColorClaudeCodeOrange, banner))
	bannerBar.SetBackgroundColor(bg)
	bannerBar.SetBorder(true)
	bannerBar.SetBorderColor(borderColor)

	// 日志区域
	Log = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	Log.SetBackgroundColor(bg)
	Log.SetBorder(true)
	Log.SetBorderColor(borderColor)

	// 垂直布局: Banner(10行) + 日志(剩余空间)
	ConfigPageFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	ConfigPageFlex.SetBackgroundColor(bg)
	ConfigPageFlex.AddItem(bannerBar, 10, 0, false)
	ConfigPageFlex.AddItem(Log, 0, 1, false)

	return ConfigPageFlex
}

func createAgentPage() tview.Primitive {

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

func Frontendinit() {
	// 创建应用实例和页面容器
	app_p = tview.NewApplication()
	pages = tview.NewPages()
	pages.AddPage("ConfigCheck", CreateConfigPage(), true, true) // 初始配置页，默认显示
	pages.AddPage("AgentPage", createAgentPage(), true, true)    // Agent页面
	InitHelpList() // 初始化帮助页（通过 SetRoot 切换，不放入 Pages）

	//设置应用根组件
	app_p.SetRoot(pages, true) // true = 全屏模式
	app_p.EnableMouse(true)    //允许接收鼠标事件
	app_p.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

}

// InitHelpList 初始化斜杠指令帮助页（List 组件，在 ShowHelpPage 中通过 SetRoot 全屏展示）
func InitHelpList() {
	HelpList = tview.NewList()
	HelpList.SetBackgroundColor(bg)
	HelpList.SetMainTextColor(tcell.GetColor(pretty.TuiMainText))
	HelpList.SetSecondaryTextColor(tcell.GetColor(pretty.TuiSubText))
	HelpList.SetSelectedBackgroundColor(tcell.GetColor("#2A3A5C"))
	HelpList.SetBorder(true)
	HelpList.SetBorderColor(borderColor)
	HelpList.SetTitle(" 斜杠指令 — Ctrl+K / Esc 关闭 ")
	HelpList.SetTitleAlign(tview.AlignLeft)

	for _, cmd := range DefaultSlashCommands {
		HelpList.AddItem(cmd.Command, cmd.Description, 0, nil)
	}
	HelpList.AddItem("Enter", "提交输入", 0, nil)
	HelpList.AddItem("Shift+Enter", "插入换行", 0, nil)
	HelpList.AddItem("ESC", "取消当前回复", 0, nil)
	HelpList.AddItem("Ctrl+K", "切换此帮助页", 0, nil)

	HelpList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlK {
			HideHelpPage()
			return nil
		}
		return event
	})
}

func Backendinit(initFn, startFn func()) {
	go func() {
		app_p.QueueUpdateDraw(func() {
			pages.SwitchToPage("ConfigCheck") // 确保初始化期间 ConfigCheck 在最前面
		})
		initFn()
		// Init 成功后切换到 Agent 页面并开始对话循环
		app_p.QueueUpdateDraw(func() {
			pages.SwitchToPage("AgentPage")
		})
		startFn()
	}()
}

func TuiRun() {
	if err := app_p.Run(); err != nil { // main goroutine 阻塞在事件循环
		fmt.Printf("Error running application: %v\n", err)
	}
}
