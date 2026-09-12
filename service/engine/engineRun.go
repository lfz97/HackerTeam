package engine

import (
	"HackerTeam/service/engine/messagerender"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type turnInfo struct {
	Code          turnCode
	Reason        string
	PartialOutput string
}
type turnCode int

// turnCode 一轮对话的结束状态，决定下一轮 agentRunIteratively 的行为。
// Startup 刻意取 0：turnInfo 的零值就是它，未显式设置 Code 时会落到最安静、
// 最安全的默认行为（不推通知、等用户输入）。
const (
	Startup  turnCode = 0 //程序启动，尚未有任何一轮对话
	New      turnCode = 1 //新对话（用户敲 /new）
	Int      turnCode = 2 //用户中断
	Error    turnCode = 3 //错误
	Exit     turnCode = 4 //用户退出
	Continue turnCode = 5 //继续对话
)

// 交互式对话
func (e *Engine) agentRunIteratively(Ctx context.Context, inputContext turnInfo) *turnInfo {
	Ctx, cancel := context.WithCancel(Ctx)
	defer cancel()
	//Startup 只在启动那一轮出现且不会被本函数返回，横幅必然只打印一次；
	//New（用户敲 /new）才推"新对话已开始"，刚启动时没有"上一轮对话"，推了是噪音。
	//Error 与 Int 不在这里打印——各自已在 agentRunOnce 里打过，再打就是重复。
	if inputContext.Code == Startup {
		(*e).tui.ShowStartupBanner((*e).startupInfoLines())
	} else if inputContext.Code == New {
		(*e).tui.ShowNotice(pretty.TBarNewConversation())
	}

	var userPrompt string
	for {
		// Error 且还有重试预算：自动构造重试 prompt，不打扰用户。
		// 其余全部情况（New / Continue / Int / 重试已耗尽）都走 else 回到输入循环等用户。
		//
		// 这里刻意用 "Error 优先 + else 兜底"，而不是列举 New||Continue||Int：
		// ① 原来的写法没有最终 else，而 inputContext 是入参、循环体内从不被重新赋值，
		//    一旦有未列举的 code 走进来，两个分支都不执行 → 循环体空转、100% CPU。
		//    今天 Exit 到不了这里只是因为 showMsgAndExit 末尾的 select{} 永久阻塞，
		//    那是个脆弱前提。
		// ② 兜底分支是"等用户输入"，比"自动构造 prompt 再打一次 API"安全得多：
		//    将来新增 turnCode 忘记登记，后果是多等一次用户输入，而不是无上限烧 API。
		//
		// 预算判定刻意内联、不抽中间变量：errorStreak 只在下面"提交非空输入"那条路上
		// 归零，而那条路紧接着 break 出循环、不会重新求值；空输入走 continue 时
		// errorStreak 未被修改，第二次判定结果一致。所以内联与循环外算一次等价。
		if inputContext.Code == Error && (*e).errorStreak < errorMaxTimes {
			if inputContext.PartialOutput != "" {
				userPrompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 之前的输出内容是: %s, 请基于这些信息调整你的回答并继续完成对话", inputContext.Reason, inputContext.PartialOutput)
			} else {
				userPrompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 请基于这个信息调整你的回答并继续完成对话", inputContext.Reason)
			}
			break

		} else {
			select {
			case userPrompt = <-(*e).tui.ListenUserInput(): //启用输入框并将用户输入放进Channel

			}

			{
				checkprompt := strings.ReplaceAll(userPrompt, "\n", "")
				checkprompt = strings.ReplaceAll(checkprompt, " ", "")
				if checkprompt == "/exit" {
					(*e).tui.PrintToMsgView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s\n", checkprompt)), false)
					return &turnInfo{
						Code:          Exit,
						Reason:        "用户主动结束对话",
						PartialOutput: "",
					}

				} else if checkprompt == "/new" {
					(*e).tui.PrintToMsgView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s\n", checkprompt)), false)
					return &turnInfo{
						Code:   New,
						Reason: "用户主动开始新对话",
					}

				} else if checkprompt == "" {
					continue //如果用户输入为空，重新开始本轮循环，等待用户输入

				} else {
					// 用户手动提交了一次新输入，给这一轮一份全新的重试预算。
					// 只对"重试已耗尽"路径有实际意义（其余路径 errorStreak 本来就是 0）。
					(*e).errorStreak = 0
					(*e).tui.PrintToMsgView(pretty.TUserInput(userPrompt), false)
					break //正常输入，继续执行后续逻辑
				}
			}
		}
	}

	// 注册应用级输入捕获器，监听ESC键以取消后续agent的输出。
	(*e).tui.SetAppFuncTriggerWithEsc(cancel)
	// 函数返回前清除应用级捕获器，避免ESC事件被持续拦截
	defer (*e).tui.ClearAppFuncTrigger()

	// AgentRunOnce返回的消息包含本次对话输入输出的所有消息。
	// 运行指示器的开关紧贴这次调用：用 defer 复位是为了 panic 安全——agentRunOnce
	// 内部跑的是框架代码，panic 时指示器会永远转下去。
	// 不要挂到本函数开头那个 Ctx 上：那个 ctx 的生命周期包含前面等用户输入的阶段，
	// 挂上去 spinner 会在用户还没打字时就转起来。
	(*e).tui.SetAgentRunning(true)
	defer (*e).tui.SetAgentRunning(false)
	AgentError_p := e.agentRunOnce(Ctx, userPrompt)
	if AgentError_p != nil { //如果运行过程中发生错误
		return &turnInfo{
			Code:          Error,
			Reason:        fmt.Sprintf("对话过程中发生错误: %v", (*AgentError_p).Error),
			PartialOutput: (*AgentError_p).PartialOutput,
		}
	}

	//如果ctx被取消，则设置结束状态为中断。
	//提示语已由 agentRunOnce 的 Ctx.Done 分支打过（"会话已取消"），这里不再重复。
	select {
	case <-Ctx.Done():
		return &turnInfo{
			Code:   Int,
			Reason: "会话已取消，停止接收输入",
		}
	default:
	}

	//单轮对话正常结束，设置状态为continue，session自动维护历史
	return &turnInfo{
		Code:   Continue,
		Reason: "单轮对话正常结束",
	}

}

type AgentError struct {
	Error         error
	ErrorType     string
	PartialOutput string
}

func (e *Engine) agentRunOnce(Ctx context.Context, userPrompt string) *AgentError {
	eventChan, err := (*(*e).AgentRunner_p).Runner.Run(
		Ctx,
		(*(*e).Config_p).User.UserID,
		(*e).SessionId,
		model.Message{
			Role:    model.RoleUser,
			Content: userPrompt,
		},
		agent.WithRequestID((*e).RequestId), //HackerTeam 保留 RequestId 机制（区别于 HyperBot v3 的删除）
		agent.WithToolCallArgumentsJSONRepairEnabled(true), //开启工具调用参数的JSON修复功能，解决因模型输出格式不规范导致的工具调用失败问题
	)
	if err != nil {
		err = fmt.Errorf("AgentRunner.Run发生错误: %v", err)
		// 在源头打印，与下面的 TerminalError 分支保持一致：每类错误只打一次。
		// 改前这里不打，只靠 agentRunIteratively 循环顶部打一次，与 TerminalError
		// 打两次的行为不一致。
		(*e).tui.PrintToMsgView(pretty.TErrorF("%v", err), false)
		return &AgentError{
			Error:         err,
			ErrorType:     "RunError",
			PartialOutput: "",
		}
	}

	partialOutput := ""
	msgRender := messagerender.NewMessageRender((*e).tui, (*(*e).Config_p).Model.ShowReasoning, (*(*e).AgentRunner_p).Stream)
	for event := range eventChan {
		//只有terminal error才会中断对话，其他error直接continue
		if event.Error != nil {
			if event.IsTerminalError() {
				//填充err，使得返回的err不为nil，表示对话发生了错误
				err = fmt.Errorf("Event发生TerminalError: %v", event.Error)
				(*e).tui.PrintToMsgView(pretty.TErrorF("%v", err), false)
				return &AgentError{
					Error:         err,
					ErrorType:     "TerminalError",
					PartialOutput: partialOutput,
				}
			} else {
				continue
			}

		}
		select {
		case <-Ctx.Done():
			(*e).tui.ShowNotice(pretty.TBarCancelled())
			return nil

		default:
		}
		if (*event).Response != nil && len((*(*event).Response).Choices) > 0 {

			for _, choice := range (*(*event).Response).Choices {

				msgRender.RenderResponse(choice)
				gatherPartialOutput(&partialOutput, choice, (*(*e).AgentRunner_p).Stream)
			}

		}
		// event.IsRunnerCompletion()判断是否完成输出
		if event.IsRunnerCompletion() {
			break
		}

	}

	return nil

}

// 收集失败前已产出的部分输出。出现错误时通过这段文本在下一轮对 llm 进行提示，
// 帮助模型理解之前发生了什么，从而调整后续输出。
//
// 不能删、也不能靠框架 session 代替：流式 delta 一律不落盘（框架 shouldPersistEvent
// 要求 !IsPartial），中途失败时那个"完整最终 event"根本没产生，这一轮 assistant 文本
// 在 session 里一个字都没有。框架的补救机制 WithPersistInterruptedAssistant 默认关闭、
// 本项目未开启，且它只在 ctx 被取消时触发，覆盖不到 TerminalError。所以这里是唯一记录。
//
// 注意只累积 Choice 的 Content —— 不含 ReasoningContent，也不含 ToolCalls。
func gatherPartialOutput(Container_p *string, Choice model.Choice, Stream bool) {
	if Stream {
		*Container_p += Choice.Delta.Content
	} else {
		*Container_p += Choice.Message.Content
	}
}
