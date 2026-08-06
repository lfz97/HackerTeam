package engine

import (
	"HackerTeam/utils/pretty"
	"strings"

	"github.com/rivo/tview"
	"sync"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

func (e *Engine) printMessage(Choice model.Choice, startReasoning *bool, stream bool) {

	if stream {
		e.renderStreamEvent(Choice, startReasoning)

	} else {
		e.renderNonStreamEvent(Choice)
	}

	e.renderToolCall(Choice)
	e.renderToolResult(Choice)
}

// 收集输出正文，如果出现错误，可以通过这段文本在下一轮对llm进行提示，帮助模型更好地理解之前发生了什么，从而调整后续输出
func gatherContentMessage(Container_p *string, Choice model.Choice, Stream bool) {
	if Stream {
		*Container_p += Choice.Delta.Content
	} else {
		*Container_p += Choice.Message.Content
	}
}

func (e *Engine) renderStreamEvent(Choice model.Choice, startReasoning *bool) {
	if Choice.Delta.ReasoningContent != "" {
		if !(*startReasoning) {
			if (*(*e).Config_p).Model.ShowReasoning {
				(*e).tui.PrintToMsgView("\n", false)
			}
			*startReasoning = true
		}
		if (*(*e).Config_p).Model.ShowReasoning {
			// 思考内容
			(*e).tui.PrintToMsgView(pretty.TReasoningContent(Choice.Delta.ReasoningContent), false)
		}
	} else if *startReasoning {
		*startReasoning = false
		if (*(*e).Config_p).Model.ShowReasoning {
			(*e).tui.PrintToMsgView("\n", false)
		}
	}
	if Choice.Delta.Content != "" && Choice.Delta.Role != "tool" {
		// 正文内容（工具响应片段不作为正文渲染，由下方统一处理工具信息部分处理）
		(*e).tui.PrintToMsgView(Choice.Delta.Content, false)
	}
}

func (e *Engine) renderNonStreamEvent(Choice model.Choice) {
	// 思考信息 - 根据配置决定是否显示
	if Choice.Message.ReasoningContent != "" && (*(*e).Config_p).Model.ShowReasoning {
		(*e).tui.PrintToMsgView("\n", false)
		(*e).tui.PrintToMsgView(pretty.TReasoningContent(Choice.Message.ReasoningContent), false)
		(*e).tui.PrintToMsgView("\n", false)
	}
	// 正文内容 - 使用 glamour 渲染 markdown，TranslateANSI 转为 tview 颜色标签
	if strings.TrimSpace(Choice.Message.Content) != "" && Choice.Message.Role != "tool" {
		out, _ := (*e).tui.NewGlamourRenderer().Render(pretty.TContentNoneStreamTag(Choice.Message.Content))
		out = strings.TrimRight(out, "\n\r ")
		(*e).tui.PrintToMsgView(tview.TranslateANSI(out)+"[-:-:-]", false)
	}
}

func (e *Engine) renderToolCall(Choice model.Choice) {
	/*------------------此处统一处理工具信息---------------------------------------------------------------------------*/

	//处理工具请求------------------------------------
	//工具请求信息不一定在delta中，也可能在message中，所以两者都要处理
	if len(Choice.Delta.ToolCalls) != 0 {
		for _, toolCall := range Choice.Delta.ToolCalls {
			e.addToolCallMsg(toolCall)
		}
	}

	if len(Choice.Message.ToolCalls) != 0 {
		for _, toolCall := range Choice.Message.ToolCalls {
			e.addToolCallMsg(toolCall)
		}
	}
}
func (e *Engine) renderToolResult(Choice model.Choice) {
	//处理工具结果------------------------------------
	//工具结果的role是tool，但信息不一定在delta中，也可能在message中，所以两者都要处理

	if Choice.Delta.Role == "tool" {

		e.addToolResultMsg(Choice.Delta.ToolID, Choice.Delta.Content)
	}
	if Choice.Message.Role == "tool" {
		e.addToolResultMsg(Choice.Message.ToolID, Choice.Message.Content)
	}

}

type toolmsg struct {
	FunctionName      string
	FunctionArguments []byte
	Result            string
}
type toolMsgBufferStruct struct {
	mu         sync.Mutex
	toolMsgMap map[string]*toolmsg
}

var toolMsgBuffer toolMsgBufferStruct = toolMsgBufferStruct{
	mu:         sync.Mutex{},
	toolMsgMap: map[string]*toolmsg{},
}

func (e *Engine) addToolCallMsg(toolcall model.ToolCall) {
	id := toolcall.ID

	toolMsgBuffer.mu.Lock()
	defer toolMsgBuffer.mu.Unlock()

	toolMsgBuffer.toolMsgMap[id] = &toolmsg{
		FunctionName:      toolcall.Function.Name,
		FunctionArguments: toolcall.Function.Arguments,
	}
}

func (e *Engine) addToolResultMsg(toolcallid string, content string) {

	toolMsgBuffer.mu.Lock()
	defer toolMsgBuffer.mu.Unlock()

	msg_p := toolMsgBuffer.toolMsgMap[toolcallid]
	if msg_p != nil {
		(*msg_p).Result = content

		(*e).tui.PrintToMsgView(pretty.TToolCompact(
			(*msg_p).FunctionName,
			(*msg_p).FunctionArguments,
			(*msg_p).Result,
		), false)

	}
}
