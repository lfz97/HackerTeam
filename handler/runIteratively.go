package handler

import (
	"HackerTeam/global"
	"HackerTeam/tui/tip"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// 交互式对话
func AgentRunIteratively(Ctx context.Context, inputContext TurnResult) *TurnResult {
	Ctx, cancel := context.WithCancel(Ctx)
	defer cancel()
	//根据传入消息的类型输出不同提示语
	if inputContext.Code == New {
		global.Print2AgentMessageView(pretty.TNewConversation())
	} else if inputContext.Code == Error {
		global.Print2AgentMessageView(pretty.TErrorF("对话发生错误: %s", inputContext.Reason))
	} else if inputContext.Code == Flush {
		global.Print2AgentMessageView(pretty.TSuccess("工具已刷新，请继续对话"))
	} else if inputContext.Code == Int { //对话因中断信号而中断,不输出提示语
	}

	var keyboardInputMessage chan string = make(chan string)
	var userPrompt string
	for {
		//如果是新对话、继续对话或中断后恢复，用户自行输入prompt
		if inputContext.Code == New || inputContext.Code == Continue || inputContext.Code == Int || inputContext.Code == Flush {
			//更新侧边栏提示语，引导用户输入
			global.App_p.QueueUpdateDraw(func() {
				global.Sidebar_p.Clear() //先清空侧边栏内容，再输出提示语
				fmt.Fprint(global.Sidebar_p, tip.SidebarUserInputTip())
			})
			global.App_p.QueueUpdateDraw(func() {
				global.App_p.SetFocus(global.InputArea_p)
				global.InputArea_p.SetDisabled(false) //启用输入框

				//注册一个输入捕获器，每次用户在输入框敲击键盘时都会触发
				global.InputArea_p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
					//当用户按下ctrl+enter时，获取输入内容，清空输入框，并发送信号继续执行后续逻辑
					if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModCtrl { //按下Ctrl+Enter时触发输入获取和信号发送
						//在获取输入内容之前先注销输入捕获器。解决输入长文本情况下，twiev内部可能出现卡死情况
						global.InputArea_p.SetInputCapture(nil)

						//获取输入文本（文本量大时GetText可能耗时）
						text := global.InputArea_p.GetText()
						global.InputArea_p.SetText("", false)
						global.InputArea_p.SetDisabled(true) //输入完成后就禁用输入框，防止用户多次输入ctrl+enter导致keyboardInputMessage <- global.InputArea_p.GetText()阻塞(因为只会被消费一次)
						keyboardInputMessage <- text
						//不传递按键
						return nil
					}

					//传递按键，框架会正常处理这个按键，比如展示在输入框上
					return event
				})
			})

			userPrompt = <-keyboardInputMessage //通过等待信号的方式阻塞代码，直到用户输入完成

			{
				checkprompt := strings.ReplaceAll(userPrompt, "\n", "")
				checkprompt = strings.ReplaceAll(checkprompt, " ", "")
				if checkprompt == "/exit" {
					global.Print2AgentMessageView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)))
					return &TurnResult{
						Code:   Exit,
						Reason: "用户主动结束对话",
					}

				} else if checkprompt == "/new" {
					global.Print2AgentMessageView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)))
					return &TurnResult{
						Code:   New,
						Reason: "用户主动开始新对话",
					}

				} else if checkprompt == "/flush" {
					global.Print2AgentMessageView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)))
					return &TurnResult{
						Code:   Flush,
						Reason: "用户主动刷新工具",
					}

				} else if checkprompt == "" {
					continue //如果用户输入为空，重新开始本轮循环，等待用户输入

				} else {
					global.Print2AgentMessageView(pretty.TUserInput(userPrompt))
					break //正常输入，继续执行后续逻辑
				}
			}

		} else if inputContext.Code == Error {
			if inputContext.OutputPart != "" {
				userPrompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 之前的输出内容是: %s, 请基于这些信息调整你的回答并继续完成对话", inputContext.Reason, inputContext.OutputPart)
			} else {
				userPrompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 请基于这个信息调整你的回答并继续完成对话", inputContext.Reason)
			}
			break
		}
	}

	// 注册一个全局的输入捕获器，监听ESC键以取消后续agent的输出。
	global.App_p.QueueUpdateDraw(func() {
		global.App_p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				cancel() // 取消 context
				return nil
			}
			return event // 其他按键正常传递
		})
	})
	// 函数返回前清除全局捕获器，避免ESC事件被持续拦截
	defer global.App_p.QueueUpdateDraw(func() {
		global.App_p.SetInputCapture(nil)
	})

	// AgentRunOnce返回的消息包含本次对话输入输出的所有消息
	AgentError_p := AgentRunOnce(Ctx, userPrompt)
	if AgentError_p != nil { //如果运行过程中发生错误
		return &TurnResult{
			Code:       Error,
			Reason:     fmt.Sprintf("对话过程中发生错误: %v", (*AgentError_p).Error),
			OutputPart: (*AgentError_p).OutputPart,
		}
	}

	//如果ctx被取消，则设置结束状态为中断
	select {
	case <-Ctx.Done():
		global.Print2AgentMessageView(pretty.TInterrupted())
		return &TurnResult{
			Code:   Int,
			Reason: "会话已取消，停止接收输入",
		}
	default:
	}

	//单轮对话正常结束，设置状态为continue，session自动维护历史
	return &TurnResult{
		Code:   Continue,
		Reason: "单轮对话正常结束",
	}

}
