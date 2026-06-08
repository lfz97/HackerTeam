package handler

import (
	"HackerTeam/global"
	"HackerTeam/tui/tip"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type AgentError struct {
	Error      error
	ErrorType  string
	OutputPart string
}

func AgentRunOnce(Ctx context.Context, userPrompt string) *AgentError {
	// 修改状态栏提示，显示正在运行中
	statusBarCtx := context.Background()
	statusBarCtx, cancel := context.WithCancel(statusBarCtx)
	defer cancel() // 确保函数退出时取消状态栏提示的上下文
	go tip.StatusBarScrollingTip(statusBarCtx, "Processing....", pretty.TColorLightMagenta, global.App_p, global.StatusBar_p)

	eventChan, err := (*global.AgentRunner_p).Runner.Run(
		Ctx,
		(*global.Config_p).User.UserID,
		global.SessionId,
		model.Message{
			Role:    model.RoleUser,
			Content: userPrompt,
		},
		agent.WithRequestID(global.RequestId),
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
				global.Print2AgentMessageView(pretty.TErrorF("%v", err))
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
			global.Print2AgentMessageView(pretty.TCancelled())
			return nil

		default:
		}
		if event.Response != nil && len((*(*event).Response).Choices) > 0 {

			Choice := (*(*event).Response).Choices[0]
			printMessage(Choice, &startReasoning, (*global.AgentRunner_p).Stream)
			gatherContentMessage(&OutputPart, Choice, (*global.AgentRunner_p).Stream)

		}
		// event.IsRunnerCompletion()判断是否完成输出
		if event.IsRunnerCompletion() {
			break
		}

	}

	return nil

}
