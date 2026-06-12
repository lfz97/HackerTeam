package handler

import (
	"HackerTeam/global"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"strings"
)

// 交互式对话
func AgentRunIteratively(Ctx context.Context, inputContext TurnResult) *TurnResult {
	Ctx, cancel := context.WithCancel(Ctx)
	defer cancel()
	//根据传入消息的类型输出不同提示语
	if inputContext.Code == New {
		global.PrintToTui(global.AgentMessage, pretty.TNewConversation(), false)
	} else if inputContext.Code == Error {
		global.PrintToTui(global.AgentMessage, pretty.TErrorF("对话发生错误: %s", inputContext.Reason), false)
	} else if inputContext.Code == Flush {
		global.PrintToTui(global.AgentMessage, pretty.TSuccess("工具已刷新，请继续对话"), false)
	} else if inputContext.Code == Int { //对话因中断信号而中断,不输出提示语
	}

	var userPrompt string
	for {
		//如果是新对话、继续对话或中断后恢复，用户自行输入prompt
		if inputContext.Code == New || inputContext.Code == Continue || inputContext.Code == Int || inputContext.Code == Flush {
			//更新侧边栏提示语，引导用户输入
			global.PrintToTui(global.Sidebar, global.SidebarUserInputTip(), true)
			userPrompt = global.LoadTextAreaWithCtrlEnter(global.InputArea) //启用输入框并将用户输入放进Channel

			{
				checkprompt := strings.ReplaceAll(userPrompt, "\n", "")
				checkprompt = strings.ReplaceAll(checkprompt, " ", "")
				if checkprompt == "/exit" {
					global.PrintToTui(global.AgentMessage, pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)), false)
					return &TurnResult{
						Code:   Exit,
						Reason: "用户主动结束对话",
					}

				} else if checkprompt == "/new" {
					global.PrintToTui(global.AgentMessage, pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)), false)
					return &TurnResult{
						Code:   New,
						Reason: "用户主动开始新对话",
					}

				} else if checkprompt == "/flush" {
					global.PrintToTui(global.AgentMessage, pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)), false)
					return &TurnResult{
						Code:   Flush,
						Reason: "用户主动刷新工具",
					}

				} else if checkprompt == "" {
					continue //如果用户输入为空，重新开始本轮循环，等待用户输入

				} else {
					global.PrintToTui(global.AgentMessage, pretty.TUserInput(userPrompt), false)
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
	global.SetAppFuncTriggerWithEsc(cancel)
	// 函数返回前清除全局捕获器，避免ESC事件被持续拦截
	defer global.ClearAppFuncTrigger()

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
		global.PrintToTui(global.AgentMessage, pretty.TInterrupted(), false)
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
