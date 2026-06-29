package global

import (
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"strings"
	"time"
)

// 定义App为全局变量，方便在其他包中访问和操作TUI应用实例
var app_p *tview.Application

type View = *tview.TextView
type TextArea = *tview.TextArea
type Pages = *tview.Pages

var (
	pages Pages
	//page config
	bannerBar View
	Log       View

	//page agent
	StatusBar    View
	AgentMessage View
	Sidebar      View
	InputArea    TextArea
)

var DefaultStatusBarTip string = pretty.TColoredText(pretty.TColorSkyBlue, "✦ « L'inspiration commence ici. » ✦")

func PrintToTui(viewType View, content string, clear bool) {
	app_p.QueueUpdateDraw(func() {
		if clear == true {
			viewType.Clear()
		}
		fmt.Fprint(viewType, content)
		viewType.ScrollToEnd()
	})
}

func LoadTextAreaWithEnter(textArea TextArea) string {
	var ch chan string = make(chan string)
	app_p.QueueUpdateDraw(func() {
		app_p.SetFocus(textArea)

		//注册一个输入捕获器，每次用户在输入框敲击键盘时都会触发
		textArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			// Enter 提交输入
			// ModNone = 0，无任何修饰键（Ctrl/Shift/Alt 均未按下），即裸按 Enter
			// bracketed paste 保证粘贴里的 \n 走 PasteEvent 通道，不会产生 KeyEnter 事件
			if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModNone {
				//提交后注销输入捕获器，避免回复期间再次Enter向无人接收的channel发送导致UI阻塞
				textArea.SetInputCapture(nil)

				//获取输入文本
				text := textArea.GetText()
				textArea.SetText("", false)
				ch <- text
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
	return <-ch //等待channel直到获取到输入内容
}

// SidebarUserInputTip 返回侧边栏的用户输入提示信息
func SidebarUserInputTip() string {
	coloredtip := fmt.Sprintf(
		"%s %s  [gray]新对话[-]\n%s %s  [gray]退出[-]\n%s %s [gray]刷新工具[-]\n%s %s [gray]发送[-]",
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/new"),
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/exit"),
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/flush"),
		pretty.TColoredText(pretty.TColorSkyBlue, "⏎"), pretty.TColoredText(pretty.TColorSkyBlue, "Enter"),
	)
	return coloredtip
}

// DisplayScrollingTip 在指定的TextView中显示平滑滚动的提示信息
func StatusBarScrollingTip(ctx context.Context, tip string, TColor string) {
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
				StatusBarUserTip(pretty.TColoredText(pretty.TColorGreen, DefaultStatusBarTip))
				return
			default:
			}

			time.Sleep(80 * time.Millisecond)
			app_p.QueueUpdateDraw(func() {
				StatusBar.Clear()
				fmt.Fprint(StatusBar, pretty.TColoredText(TColor, word))
			})
		}
	}
}

func SetAppFuncTriggerWithEsc(f func()) {
	app_p.QueueUpdateDraw(func() {
		app_p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				f() // 执行回调
				return nil
			}
			return event // 其他按键正常传递
		})
	})
}

func ClearAppFuncTrigger() {
	app_p.QueueUpdateDraw(func() {
		app_p.SetInputCapture(nil)
	})
}

// StatusBarDefaultTip 在状态栏显示默认提示信息
func StatusBarUserTip(s string) {
	app_p.QueueUpdateDraw(func() {
		StatusBar.Clear()
		fmt.Fprint(StatusBar, s)
	})
}

func ShowErrorAndExit(view View, errmsg string) {
	done := make(chan struct{})
	PrintToTui(view, errmsg, false)
	app_p.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		app_p.SetFocus(view)
		view.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				app_p.Stop()
				return nil
			})
	})
	<-done
}

func ShowSuccess(view View, sussessmsg string) {
	PrintToTui(view, pretty.TSuccess(sussessmsg), false)
}

func ShowSuccessAndExit(view View, sussessmsg string) {
	done := make(chan struct{})
	PrintToTui(view, pretty.TSuccess(sussessmsg), false)
	app_p.QueueUpdateDraw(func() {
		//只要有按键就退出程序
		app_p.SetFocus(view)
		view.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				app_p.Stop()
				return nil
			})
	})
	<-done
}

func ShowMsgAndExitNoTrigger(view View, msg string) {
	done := make(chan struct{})
	PrintToTui(view, msg, false)
	app_p.QueueUpdateDraw(func() {
		app_p.Stop()

	})
	<-done
}
