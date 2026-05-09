package handler

import (
	"HyperBot/tui/global_object"
	"HyperBot/utils/pretty"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func printMessage(Choice model.Choice, startReasoning *bool, stream bool) {

	if stream {
		//------------------处理流式的响应---------------------------------------------------------------------------
		if Choice.Delta.ReasoningContent != "" && !(*startReasoning) {

			global_object.Print2AgentMessageView(pretty.TReasoningStart())
			*startReasoning = true

		} else if Choice.Delta.ReasoningContent != "" && (*startReasoning) {

			// 思考内容
			global_object.Print2AgentMessageView(pretty.TReasoningContent(Choice.Delta.ReasoningContent))

		} else if Choice.Delta.ReasoningContent == "" && (*startReasoning) {
			*startReasoning = false

			global_object.Print2AgentMessageView(pretty.TReasoningEnd())

		}
		if Choice.Delta.Content != "" {
			// 正文内容
			global_object.Print2AgentMessageView(Choice.Delta.Content)
		}

	} else {
		//------------------处理非流式的响应---------------------------------------------------------------------------
		//处理思考信息 - 使用黄色
		if Choice.Message.ReasoningContent != "" {

			global_object.Print2AgentMessageView(pretty.TReasoningStart())
			global_object.Print2AgentMessageView(pretty.TReasoningContent(Choice.Message.ReasoningContent))
			global_object.Print2AgentMessageView(pretty.TReasoningEnd())

		}
		// 正文内容
		if Choice.Message.Content != "" {
			global_object.Print2AgentMessageView(Choice.Message.Content)
		}
	}

	/*------------------此处统一处理工具信息---------------------------------------------------------------------------*/

	//处理工具请求------------------------------------
	//工具请求信息不一定在delta中，也可能在message中，所以两者都要处理
	if len(Choice.Delta.ToolCalls) != 0 {
		for _, toolCall := range Choice.Delta.ToolCalls {
			global_object.Print2AgentMessageView(pretty.TToolCall(toolCall.Function.Name) + pretty.TToolArgs(string(toolCall.Function.Arguments)))
		}
	}

	if len(Choice.Message.ToolCalls) != 0 {
		for _, toolCall := range Choice.Message.ToolCalls {
			global_object.Print2AgentMessageView(pretty.TToolCall(toolCall.Function.Name) + pretty.TToolArgs(string(toolCall.Function.Arguments)))
		}
	}
	//处理工具结果------------------------------------
	//工具结果的role是tool，但信息不一定在delta中，也可能在message中，所以两者都要处理
	{
		if Choice.Delta.Role == "tool" {
			global_object.Print2AgentMessageView(pretty.TToolResult(Choice.Delta.Content))
		}
		if Choice.Message.Role == "tool" {
			global_object.Print2AgentMessageView(pretty.TToolResult(Choice.Message.Content))
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
