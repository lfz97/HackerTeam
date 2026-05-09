package global_object

import (
	"fmt"
	"github.com/rivo/tview"
)

// 定义App为全局变量，方便在其他包中访问和操作TUI应用实例
var App_p *tview.Application

// page 1
var (
	StatusView_p *tview.TextView
	LogView_p    *tview.TextView
)

// page 2
var (
	StatusBar_p        *tview.TextView
	AgentMessageView_p *tview.TextView
	InputArea_p        *tview.TextArea
	Sidebar_p          *tview.TextView
)

func Print2AgentMessageView(content string) {
	App_p.QueueUpdateDraw(func() {
		fmt.Fprint(AgentMessageView_p, content)
		AgentMessageView_p.ScrollToEnd()
	})
}

func Print2LogView(content string) {
	App_p.QueueUpdateDraw(func() {
		fmt.Fprint(LogView_p, content)
		LogView_p.ScrollToEnd()
	})
}
