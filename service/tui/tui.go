package tui

import (
	"HackerTeam/utils/pretty"
	"charm.land/glamour/v2"
	"context"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"strings"
	"time"
)

// 定义颜色，配色统一来源于 pretty.TuiXxx 常量，确保界面风格统一且美观
var (
	bg                  tcell.Color = tcell.GetColor(pretty.TuiBg)          // 整体背景色
	borderColor         tcell.Color = tcell.GetColor(pretty.TuiBorderColor) // 边框颜色
	StatusBarBg         tcell.Color = tcell.GetColor(pretty.TuiStatusBarBg) // 标题栏背景色
	inputAreaBg         tcell.Color = tcell.GetColor(pretty.TuiInputAreaBg) // 输入区背景色
	DefaultStatusBarTip string      = pretty.TColoredText(pretty.TColorSkyBlue, "✦ « L’inspiration commence ici. » ✦")
)

type Tui struct {
	app       *tview.Application
	appLayout *layout
	InputChan chan string
}

type layout struct {
	pages        *tview.Pages
	statusBar    *tview.TextView
	agentMessage *tview.TextView
	inputArea    *tview.TextArea
	helpTable    *helptable
}
type helptable struct {
	h               *tview.Table
	helpPageVisible bool
	helpItems       []helpItem
}
type helpItem struct {
	cmd  string
	desc string
}

func (t *Tui) PrintToMsgView(content string, clear bool) {
	(*t).app.QueueUpdateDraw(func() {
		if clear == true {
			(*(*t).appLayout).agentMessage.Clear()
		}
		fmt.Fprint((*(*t).appLayout).agentMessage, content)
		(*(*t).appLayout).agentMessage.ScrollToEnd()
	})
}

func (t *Tui) ReadInputAreaPromptWithEnter() {
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.SetFocus((*(*t).appLayout).inputArea)

		//注册一个输入捕获器，每次用户在输入框敲击键盘时都会触发
		(*(*t).appLayout).inputArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			// Ctrl+K 切换帮助页
			if event.Key() == tcell.KeyCtrlK {
				t.toggleHelpPage()
				return nil
			}

			// Enter 提交输入
			// ModNone = 0，无任何修饰键（Ctrl/Shift/Alt 均未按下），即裸按 Enter
			// bracketed paste 保证粘贴里的 \n 走 PasteEvent 通道，不会产生 KeyEnter 事件
			if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModNone {

				//获取输入文本
				text := (*(*t).appLayout).inputArea.GetText()
				// 发送输入文本到 InputChan。default 分支：对端（引擎循环）未在监听时
				// （如自动 turn 期间）不投递，避免 unbuffered send 阻塞 tview 事件循环
				// 导致 UI 卡死。注意：只有投递成功才清空输入框，default 分支保留文本，
				// 用户输入不丢失。
				select {
				case (*t).InputChan <- text:
					(*(*t).appLayout).inputArea.SetText("", false)
				default:
				}
				return nil //Enter事件不捕获
			}

			// Shift+Enter 插入换行（手动多行输入）
			if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModShift {
				return event
			}

			//传递事件给 TextArea 默认处理（插入字符、换行等）
			return event
		})
	})

}

func (t *Tui) InputChannel() chan string {
	return (*t).InputChan
}

func (t *Tui) StatusBarScrollingTip(ctx context.Context, tip string, TColor string) {
	char := strings.Split(tip, "")
	dynamicWords := []string{}
	increaseWords := []string{}
	//逐渐增加字符，拼接成新的字符串，写入dynamicWords切片中
	for i := 0; i < len(char); i++ {
		if i == 0 {
			increaseWords = append(increaseWords, char[i])
		} else {
			increaseWords = append(increaseWords, increaseWords[i-1]+char[i])
		}
	}

	decreaseWords := []string{}
	for i := 0; i < len(char); i++ {
		char[i] = " "
		decreaseWords = append(decreaseWords, strings.Join(char, ""))
	}
	dynamicWords = append(dynamicWords, increaseWords...)
	dynamicWords = append(dynamicWords, decreaseWords...)
	for {
		for _, word := range dynamicWords {

			select {
			case <-ctx.Done():

				(*t).app.QueueUpdateDraw(func() {
					(*(*t).appLayout).statusBar.Clear()
					fmt.Fprint((*(*t).appLayout).statusBar, pretty.TColoredText(pretty.TColorGreen, DefaultStatusBarTip))
				})
				return
			default:
			}

			time.Sleep(80 * time.Millisecond)
			(*t).app.QueueUpdateDraw(func() {
				(*(*t).appLayout).statusBar.Clear()
				fmt.Fprint((*(*t).appLayout).statusBar, pretty.TColoredText(TColor, word))
			})
		}
	}
}

func (t *Tui) SetAppFuncTriggerWithEsc(f func()) {
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				f() // 执行回调
				return nil
			}
			return event // 其他按键正常传递
		})
	})
}

func (t *Tui) ClearAppFuncTrigger() {
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.SetInputCapture(nil)
	})
}
func (t *Tui) StatusBarUserTip(s string) {
	(*t).app.QueueUpdateDraw(func() {
		(*(*t).appLayout).statusBar.Clear()
		fmt.Fprint((*(*t).appLayout).statusBar, s)
	})
}
func (t *Tui) ShowErrorInMsgViewAndExit(errmsg string) {
	done := make(chan struct{})
	t.PrintToMsgView(errmsg, false)
	(*t).app.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		(*t).app.SetFocus((*(*t).appLayout).agentMessage)
		(*(*t).appLayout).agentMessage.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				(*t).app.Stop()
				return nil
			})
	})
	<-done
}

func (t *Tui) ShowSuccessInMsgView(sussessmsg string) {
	t.PrintToMsgView(pretty.TSuccess(sussessmsg), false)
}
func (t *Tui) ShowSuccessInMsgViewAndExit(sussessmsg string) {
	done := make(chan struct{})
	t.PrintToMsgView(pretty.TSuccess(sussessmsg), false)
	(*t).app.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		(*t).app.SetFocus((*(*t).appLayout).agentMessage)
		(*(*t).appLayout).agentMessage.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				(*t).app.Stop()
				return nil
			})
	})
	<-done
}

func (t *Tui) ShowMsgAndExitNoTrigger(msg string) {
	done := make(chan struct{})
	t.PrintToMsgView(msg, false)
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.Stop()

	})
	<-done
}
func (t *Tui) AddHelpItems(items []map[string]string) {
	for _, i := range items {
		for k, v := range i {
			h := helpItem{
				cmd:  k,
				desc: v,
			}
			(*((*t).appLayout).helpTable).helpItems = append((*((*t).appLayout).helpTable).helpItems, h)
		}
	}
}
func (t *Tui) ResetHelpItems() {
	t.defaultHelpItems()
}
func (t *Tui) NewGlamourRenderer() *glamour.TermRenderer {
	_, _, w, _ := (*(*t).appLayout).agentMessage.GetInnerRect()
	if w < 40 {
		w = 80
	}
	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w),
		glamour.WithStylesFromJSONBytes([]byte(`{
			"document": {
				"margin": 0
			}
		}`)),
	)
	return r
}

func (t *Tui) Run() {
	err := (*t).app.Run()
	if err != nil {
		panic("Error running application: " + err.Error())
	}
}
func GetTuiService() *Tui {
	//设置标题状态栏
	StatusBar := tview.NewTextView()
	StatusBar.SetDynamicColors(true).SetWrap(false).SetText(DefaultStatusBarTip)
	StatusBar.SetTextAlign(tview.AlignCenter)
	StatusBar.SetBackgroundColor(StatusBarBg)

	//设置Agent消息显示区
	AgentMessage := tview.NewTextView().
		SetDynamicColors(true). // 启用颜色
		SetScrollable(true).    // 可滚动
		SetWrap(true)

	AgentMessage.SetBackgroundColor(bg) // 设置背景颜色

	//设置底部输入区
	InputArea := tview.NewTextArea().
		SetLabel(`⇒ `).
		SetWrap(true)
	InputArea.SetBackgroundColor(inputAreaBg)
	InputArea.SetTextStyle(tcell.StyleDefault.
		Background(inputAreaBg).                        // 输入区背景色
		Foreground(tcell.GetColor(pretty.TuiMainText))) // 文字颜色

	// 输入区右侧提示
	InputHint := tview.NewTextView().
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

	HelpTable := tview.NewTable()
	HelpTable.SetBackgroundColor(bg)
	HelpTable.SetBorder(true)
	HelpTable.SetBorderColor(borderColor)
	HelpTable.SetTitle(" 斜杠指令 — Ctrl+K / Esc 关闭 ")
	HelpTable.SetTitleAlign(tview.AlignLeft)
	HelpTable.SetSelectable(true, false) // 行可选，列不可选

	HelpTable.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.GetColor("#2A3A5C")).
		Foreground(tcell.GetColor(pretty.TuiMainText)))

	app := tview.NewApplication()
	pages := tview.NewPages()
	pages.AddPage("AgentPage", MainFlex, true, true)
	app.SetRoot(pages, true) // true = 全屏模式
	app.EnableMouse(true)    //允许接收鼠标事件
	app.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

	tui := &Tui{
		app: app,
		appLayout: &layout{
			pages:        pages,
			statusBar:    StatusBar,
			agentMessage: AgentMessage,
			inputArea:    InputArea,
			helpTable: &helptable{
				h:               HelpTable,
				helpItems:       []helpItem{},
				helpPageVisible: false,
			},
		},
		InputChan: make(chan string),
	}

	(*(*(*tui).appLayout).helpTable).h.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlK {
			tui.toggleHelpPage()
			return nil
		}
		return event
	})
	tui.defaultHelpItems()
	tui.refreshhelpTable()
	return tui

}
