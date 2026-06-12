package global

import (
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

func CreateConfigPage(pages *tview.Pages, initFn, startFn func()) tview.Primitive {
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

	go func() {
		//启动检测
		initFn()
		//如果Init都成功了，创建Agent页面
		AgentPage := createAgentPage(startFn)
		app_p.QueueUpdateDraw(func() {
			//添加并切换到Agent页面
			pages.AddPage("AgentPage", AgentPage, true, true)
			pages.SwitchToPage("AgentPage")
		})
	}()
	return ConfigPageFlex
}

func createAgentPage(f func()) tview.Primitive {

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

	AgentMessage.SetBackgroundColor(bg)      // 设置背景颜色
	AgentMessage.SetBorder(true)             // 设置边框
	AgentMessage.SetBorderColor(borderColor) // 设置边框颜色

	//设置底部输入区
	InputArea = tview.NewTextArea().
		SetLabel(`⇒ `).
		SetWrap(true)
	InputArea.SetBackgroundColor(inputAreaBg)
	InputArea.SetTextStyle(tcell.StyleDefault.
		Background(inputAreaBg).                        // 输入区背景色
		Foreground(tcell.GetColor(pretty.TuiMainText))) // 文字颜色

	//设置左侧命令提示区
	Sidebar = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	Sidebar.SetBackgroundColor(SidebarBg)
	Sidebar.SetBorder(true)
	Sidebar.SetBorderColor(borderColor)

	//设置布局
	//设置中间的sidebar+Agent消息区布局
	MiddleFlex_p := tview.NewFlex()
	MiddleFlex_p.AddItem(Sidebar, 20, 0, false)     // 左侧的命令提示区占20列
	MiddleFlex_p.AddItem(AgentMessage, 0, 1, false) // 消息视图占剩余空间

	//设置整体布局
	MainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	MainFlex.SetBackgroundColor(bg)
	MainFlex.AddItem(StatusBar, 1, 0, false)    // 顶部的状态栏占2行
	MainFlex.AddItem(MiddleFlex_p, 0, 1, false) // 中间的sidebar+Agent消息区占剩余空间
	MainFlex.AddItem(InputArea, 1, 0, true)     // 底部的输入区占2行

	go func() {
		f()
	}()

	return MainFlex
}

func TuiInit(initFn, startFn func()) {
	// 创建应用实例和页面容器
	app_p = tview.NewApplication()
	pages := tview.NewPages()
	pages.AddPage("config", CreateConfigPage(pages, initFn, startFn), true, true) // 初始显示配置页

	//设置应用根组件并启动
	app_p.SetRoot(pages, true) // true = 全屏模式
	app_p.EnableMouse(true)    //允许接收鼠标事件
	app_p.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

}

func TuiRun() {
	if err := app_p.Run(); err != nil {
		panic(err)
	}
}
