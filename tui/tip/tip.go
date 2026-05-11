package tip

import (
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"github.com/rivo/tview"
	"strings"
	"time"
)

var DefaultStatusBarTip string = pretty.TColoredText(pretty.TColorSkyBlue, "✦ « L’inspiration commence ici. » ✦")

// SidebarUserInputTip 返回侧边栏的用户输入提示信息
func SidebarUserInputTip() string {
	coloredtip := fmt.Sprintf(
		"%s %s  [gray]新对话[-]\n%s %s  [gray]退出[-]\n%s %s [gray]刷新工具[-]\n%s %s [gray]发送[-]",
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/new"),
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/exit"),
		pretty.TColoredText(pretty.TColorSkyBlue, "➤"), pretty.TColoredText(pretty.TColorSkyBlue, "/flush"),
		pretty.TColoredText(pretty.TColorSkyBlue, "⏎"), pretty.TColoredText(pretty.TColorSkyBlue, "Ctrl+Enter"),
	)
	return coloredtip
}

// DisplayScrollingTip 在指定的TextView中显示平滑滚动的提示信息
func StatusBarScrollingTip(ctx context.Context, tip string, TColor string, App_p *tview.Application, View_p *tview.TextView) {
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
				StatusBarUserTip(App_p, View_p, pretty.TColoredText(pretty.TColorGreen, DefaultStatusBarTip))
				return
			default:
			}

			time.Sleep(80 * time.Millisecond)
			App_p.QueueUpdateDraw(func() {
				View_p.Clear()
				fmt.Fprint(View_p, pretty.TColoredText(TColor, word))
			})
		}
	}
}

// StatusBarDefaultTip 在状态栏显示默认提示信息
func StatusBarUserTip(App_p *tview.Application, View_p *tview.TextView, s string) {
	App_p.QueueUpdateDraw(func() {
		View_p.Clear()
		fmt.Fprint(View_p, s)
	})
}
