package engine

import (
	"context"
	"fmt"
	"HackerTeam/utils/pretty"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type turnResult struct {
	Code       turnCode
	Reason     string
	OutputPart string
}
type turnCode int

const (
	New      turnCode = 1 //新对话
	Int      turnCode = 2 //用户中断
	Error    turnCode = 3 //错误
	Exit     turnCode = 4 //用户退出
	Continue turnCode = 5 //继续对话
)

// 交互式对话
func (e *Engine) agentRunIteratively(Ctx context.Context, inputContext turnResult) *turnResult {
	Ctx, cancel := context.WithCancel(Ctx)
	defer cancel()
	//根据传入消息的类型输出不同提示语
	if inputContext.Code == New {
		(*e).tui.PrintToMsgView(pretty.TNewConversation(), false)
	} else if inputContext.Code == Error {
		(*e).tui.PrintToMsgView(pretty.TErrorF("对话发生错误: %s", inputContext.Reason), false)
	} else if inputContext.Code == Int { //对话因中断信号而中断,不输出提示语
	}

	var userPrompt string
	for {
		//如果是新对话、继续对话或中断后恢复，用户自行输入prompt
		if inputContext.Code == New || inputContext.Code == Continue || inputContext.Code == Int {
			userPrompt = (*e).tui.ReadInputAreaPromptWithEnter() //启用输入框并将用户输入放进Channel

			{
				checkprompt := strings.ReplaceAll(userPrompt, "\n", "")
				checkprompt = strings.ReplaceAll(checkprompt, " ", "")
				if checkprompt == "/exit" {
					(*e).tui.PrintToMsgView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)), false)
					return &turnResult{
						Code:   Exit,
						Reason: "用户主动结束对话",
					}

				} else if checkprompt == "/new" {
					(*e).tui.PrintToMsgView(pretty.TColoredText(pretty.TColorLightGreen, fmt.Sprintf("\n%s%s\n", pretty.SymbolBullet, checkprompt)), false)
					return &turnResult{
						Code:   New,
						Reason: "用户主动开始新对话",
					}

				} else if checkprompt == "" {
					continue //如果用户输入为空，重新开始本轮循环，等待用户输入

				} else {
					(*e).tui.PrintToMsgView(pretty.TUserInput(userPrompt), false)
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

	// 注册应用级输入捕获器，监听ESC键以取消后续agent的输出。
	(*e).tui.SetAppFuncTriggerWithEsc(cancel)
	// 函数返回前清除应用级捕获器，避免ESC事件被持续拦截
	defer (*e).tui.ClearAppFuncTrigger()

	// AgentRunOnce返回的消息包含本次对话输入输出的所有消息
	AgentError_p := e.agentRunOnce(Ctx, userPrompt)
	if AgentError_p != nil { //如果运行过程中发生错误
		return &turnResult{
			Code:       Error,
			Reason:     fmt.Sprintf("对话过程中发生错误: %v", (*AgentError_p).Error),
			OutputPart: (*AgentError_p).OutputPart,
		}
	}

	//如果ctx被取消，则设置结束状态为中断
	select {
	case <-Ctx.Done():
		(*e).tui.PrintToMsgView(pretty.TInterrupted(), false)
		return &turnResult{
			Code:   Int,
			Reason: "会话已取消，停止接收输入",
		}
	default:
	}

	//单轮对话正常结束，设置状态为continue，session自动维护历史
	return &turnResult{
		Code:   Continue,
		Reason: "单轮对话正常结束",
	}

}

type AgentError struct {
	Error      error
	ErrorType  string
	OutputPart string
}

func (e *Engine) agentRunOnce(Ctx context.Context, userPrompt string) *AgentError {
	toolMsgBuffer.toolMsgMap = map[string]*toolmsg{}
	// 修改状态栏提示，显示正在运行中
	statusBarCtx := context.Background()
	statusBarCtx, cancel := context.WithCancel(statusBarCtx)
	defer cancel() // 确保函数退出时取消状态栏提示的上下文
	go (*e).tui.StatusBarScrollingTip(statusBarCtx, "Processing....", pretty.TColorLightMagenta)

	eventChan, err := (*(*e).AgentRunner_p).Runner.Run(
		Ctx,
		(*(*e).Config_p).User.UserID,
		(*e).SessionId,
		model.Message{
			Role:    model.RoleUser,
			Content: userPrompt,
		},
		agent.WithRequestID((*e).RequestId),
		agent.WithToolCallArgumentsJSONRepairEnabled(true), //开启工具调用参数的JSON修复功能，解决因模型输出格式不规范导致的工具调用失败问题
	)
	if err != nil {
		return &AgentError{
			Error:      fmt.Errorf("AgentRunner.Run发生错误: %v", err),
			ErrorType:  "RunError",
			OutputPart: "",
		}
	}

	OutputPart := ""
	startReasoning := false
	for event := range eventChan {
		//只有terminal error才会中断对话，其他error直接continue
		if event.Error != nil {
			if event.IsTerminalError() {
				//填充err，使得返回的err不为nil，表示对话发生了错误
				err = fmt.Errorf("Event发生TerminalError: %v", event.Error)
				(*e).tui.PrintToMsgView(pretty.TErrorF("%v", err), false)
				return &AgentError{
					Error:      err,
					ErrorType:  "TerminalError",
					OutputPart: OutputPart,
				}
			} else {
				continue
			}

		}
		select {
		case <-Ctx.Done():
			(*e).tui.PrintToMsgView(pretty.TCancelled(), false)
			return nil

		default:
		}
		if event.Response != nil && len((*(*event).Response).Choices) > 0 {
			response := (*event).Response

			// 工具结果事件可能包含多个 Choice（框架将并行工具调用的结果合并到一个事件中），
			// 需要遍历所有 Choice 而非只取 Choices[0]。
			if response.Object == model.ObjectTypeToolResponse {
				for _, Choice := range response.Choices {
					e.printMessage(Choice, &startReasoning, (*(*e).AgentRunner_p).Stream)
					gatherContentMessage(&OutputPart, Choice, (*(*e).AgentRunner_p).Stream)
				}
			} else {
				Choice := response.Choices[0]
				e.printMessage(Choice, &startReasoning, (*(*e).AgentRunner_p).Stream)
				gatherContentMessage(&OutputPart, Choice, (*(*e).AgentRunner_p).Stream)
			}

		}
		// event.IsRunnerCompletion()判断是否完成输出
		if event.IsRunnerCompletion() {
			break
		}

	}

	return nil

}
