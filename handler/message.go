package handler

import (
	"HackerTeam/global"
	"HackerTeam/utils/pretty"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func printMessage(Choice model.Choice, startReasoning *bool, stream bool) {

	if stream {
		//------------------处理流式的响应---------------------------------------------------------------------------
		if Choice.Delta.ReasoningContent != "" && !(*startReasoning) {

			global.PrintToTui(global.AgentMessage, pretty.TReasoningStart(), false)
			*startReasoning = true

		} else if Choice.Delta.ReasoningContent != "" && (*startReasoning) {

			// 思考内容
			global.PrintToTui(global.AgentMessage, pretty.TReasoningContent(Choice.Delta.ReasoningContent), false)

		} else if Choice.Delta.ReasoningContent == "" && (*startReasoning) {
			*startReasoning = false

			global.PrintToTui(global.AgentMessage, pretty.TReasoningEnd(), false)

		}
		if Choice.Delta.Content != "" {
			// 正文内容
			global.PrintToTui(global.AgentMessage, Choice.Delta.Content, false)
		}

	} else {
		//------------------处理非流式的响应---------------------------------------------------------------------------
		//处理思考信息 - 使用黄色
		if Choice.Message.ReasoningContent != "" {

			global.PrintToTui(global.AgentMessage, pretty.TReasoningStart(), false)
			global.PrintToTui(global.AgentMessage, pretty.TReasoningContent(Choice.Message.ReasoningContent), false)
			global.PrintToTui(global.AgentMessage, pretty.TReasoningEnd(), false)

		}
		// 正文内容
		if Choice.Message.Content != "" {
			global.PrintToTui(global.AgentMessage, Choice.Message.Content, false)
		}
	}

	/*------------------此处统一处理工具信息---------------------------------------------------------------------------*/

	//处理工具请求------------------------------------
	//工具请求信息不一定在delta中，也可能在message中，所以两者都要处理
	if len(Choice.Delta.ToolCalls) != 0 {
		for _, toolCall := range Choice.Delta.ToolCalls {
			global.PrintToTui(global.AgentMessage, pretty.TToolCall(toolCall.Function.Name)+pretty.TToolArgs(string(toolCall.Function.Arguments)), false)
		}
	}

	if len(Choice.Message.ToolCalls) != 0 {
		for _, toolCall := range Choice.Message.ToolCalls {
			global.PrintToTui(global.AgentMessage, pretty.TToolCall(toolCall.Function.Name)+pretty.TToolArgs(string(toolCall.Function.Arguments)), false)
		}
	}
	//处理工具结果------------------------------------
	//工具结果的role是tool，但信息不一定在delta中，也可能在message中，所以两者都要处理
	{
		if Choice.Delta.Role == "tool" {
			global.PrintToTui(global.AgentMessage, pretty.TToolResult(Choice.Delta.Content), false)
		}
		if Choice.Message.Role == "tool" {
			global.PrintToTui(global.AgentMessage, pretty.TToolResult(Choice.Message.Content), false)
		}
	}
}

// 收集输出正文，如果出现错误，可以通过这段文本在下一轮对llm进行提示，帮助模型更好地理解之前发生了什么，从而调整后续输出
func gatherContentMessage(Container_p *string, Choice model.Choice, Stream bool) {
	if Stream {
		*Container_p += Choice.Delta.Content
	} else {
		*Container_p += Choice.Message.Content
	}
}
