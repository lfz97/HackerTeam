package tui

import (
	"HackerTeam/bootstrap"
	"HackerTeam/global"
	"HackerTeam/tui/tip"
	"HackerTeam/utils/pretty"
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

func CreateConfigPage(pages *tview.Pages) tview.Primitive {
	// Banner 区域
	global.StatusView_p = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetTextAlign(tview.AlignCenter).
		SetText(pretty.TColoredText(pretty.TColorClaudeCodeOrange, banner))
	global.StatusView_p.SetBackgroundColor(bg)
	global.StatusView_p.SetBorder(true)
	global.StatusView_p.SetBorderColor(borderColor)

	// 日志区域
	global.LogView_p = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	global.LogView_p.SetBackgroundColor(bg)
	global.LogView_p.SetBorder(true)
	global.LogView_p.SetBorderColor(borderColor)

	// 垂直布局: Banner(10行) + 日志(剩余空间)
	ConfigPageFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	ConfigPageFlex.SetBackgroundColor(bg)
	ConfigPageFlex.AddItem(global.StatusView_p, 10, 0, false)
	ConfigPageFlex.AddItem(global.LogView_p, 0, 1, false)

	go func() {
		//初始化AgentRunner
		bootstrap.Init("HackerTeam")
		//如果Init都成功了，创建Agent页面
		AgentPage := createAgentPage()
		global.App_p.QueueUpdateDraw(func() {
			//添加并切换到Agent页面
			pages.AddPage("AgentPage", AgentPage, true, true)
			pages.SwitchToPage("AgentPage")
		})
	}()
	return ConfigPageFlex
}

func createAgentPage() tview.Primitive {

	//设置标题状态栏
	global.StatusBar_p = tview.NewTextView()
	global.StatusBar_p.SetDynamicColors(true).SetWrap(false).SetText(tip.DefaultStatusBarTip)
	global.StatusBar_p.SetTextAlign(tview.AlignCenter)
	global.StatusBar_p.SetBackgroundColor(StatusBarBg)

	//设置Agent消息显示区
	global.AgentMessageView_p = tview.NewTextView().
		SetDynamicColors(true). // 启用颜色
		SetScrollable(true).    // 可滚动
		SetWrap(true)

	global.AgentMessageView_p.SetBackgroundColor(bg)      // 设置背景颜色
	global.AgentMessageView_p.SetBorder(true)             // 设置边框
	global.AgentMessageView_p.SetBorderColor(borderColor) // 设置边框颜色

	//设置底部输入区
	global.InputArea_p = tview.NewTextArea().
		SetLabel(`⇒ `).
		SetWrap(true)
	global.InputArea_p.SetBackgroundColor(inputAreaBg)
	global.InputArea_p.SetTextStyle(tcell.StyleDefault.
		Background(inputAreaBg).                        // 输入区背景色
		Foreground(tcell.GetColor(pretty.TuiMainText))) // 文字颜色

	//设置左侧命令提示区
	global.Sidebar_p = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	global.Sidebar_p.SetBackgroundColor(SidebarBg)
	global.Sidebar_p.SetBorder(true)
	global.Sidebar_p.SetBorderColor(borderColor)

	//设置布局
	//设置中间的sidebar+Agent消息区布局
	MiddleFlex_p := tview.NewFlex()
	MiddleFlex_p.AddItem(global.Sidebar_p, 20, 0, false)         // 左侧的命令提示区占20列
	MiddleFlex_p.AddItem(global.AgentMessageView_p, 0, 1, false) // 消息视图占剩余空间

	//设置整体布局
	MainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	MainFlex.SetBackgroundColor(bg)
	MainFlex.AddItem(global.StatusBar_p, 1, 0, false) // 顶部的状态栏占2行
	MainFlex.AddItem(MiddleFlex_p, 0, 1, false)       // 中间的sidebar+Agent消息区占剩余空间
	MainFlex.AddItem(global.InputArea_p, 1, 0, true)  // 底部的输入区占2行

	go func() {
		bootstrap.AgentStart()
	}()

	return MainFlex
}

func TuiInit() {
	// 创建应用实例和页面容器
	global.App_p = tview.NewApplication()
	pages := tview.NewPages()
	pages.AddPage("config", CreateConfigPage(pages), true, true) // 初始显示配置页

	//设置应用根组件并启动
	global.App_p.SetRoot(pages, true) // true = 全屏模式
	global.App_p.EnableMouse(true)    //允许接收鼠标事件
	global.App_p.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

}
