package handler

import (
	"HackerTeam/global"
	"HackerTeam/utils/pretty"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/rivo/tview"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// glamourRenderer 基于 dark 主题的自定义渲染器，补上了表格边框（dark 主题默认 table 样式为空，无边框）。
var glamourRenderer *glamour.TermRenderer

func init() {
	glamourRenderer, _ = glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithStylesFromJSONBytes([]byte(`{
			"document": {
				"margin": 0
			},
			"table": {
				"center_separator": "┼",
				"column_separator": "│",
				"row_separator": "─"
			}
		}`)),
	)
}

func printMessage(Choice model.Choice, startReasoning *bool, stream bool) {

	if stream {
		renderStreamEvent(Choice, startReasoning)

	} else {
		renderNonStreamEvent(Choice)
	}

	renderToolCall(Choice)
	renderToolResult(Choice)
}

// 收集输出正文，如果出现错误，可以通过这段文本在下一轮对llm进行提示，帮助模型更好地理解之前发生了什么，从而调整后续输出
func gatherContentMessage(Container_p *string, Choice model.Choice, Stream bool) {
	if Stream {
		*Container_p += Choice.Delta.Content
	} else {
		*Container_p += Choice.Message.Content
	}
}

func renderStreamEvent(Choice model.Choice, startReasoning *bool) {
	if Choice.Delta.ReasoningContent != "" {
		if !(*startReasoning) {
			if (*global.Config_p).Model.ShowReasoning {
				global.PrintToTui(global.AgentMessage, "\n", false)
			}
			*startReasoning = true
		}
		if (*global.Config_p).Model.ShowReasoning {
			// 思考内容
			global.PrintToTui(global.AgentMessage, pretty.TReasoningContent(Choice.Delta.ReasoningContent), false)
		}
	} else if *startReasoning {
		*startReasoning = false
		if (*global.Config_p).Model.ShowReasoning {
			global.PrintToTui(global.AgentMessage, "\n", false)
		}
	}
	if Choice.Delta.Content != "" && Choice.Delta.Role != "tool" {
		// 正文内容（工具响应片段不作为正文渲染，由下方统一处理工具信息部分处理）
		global.PrintToTui(global.AgentMessage, Choice.Delta.Content, false)
	}
}

func renderNonStreamEvent(Choice model.Choice) {
	// 思考信息 - 根据配置决定是否显示
	if Choice.Message.ReasoningContent != "" && (*global.Config_p).Model.ShowReasoning {
		global.PrintToTui(global.AgentMessage, "\n", false)
		global.PrintToTui(global.AgentMessage, pretty.TReasoningContent(Choice.Message.ReasoningContent), false)
		global.PrintToTui(global.AgentMessage, "\n", false)
	}
	// 正文内容 - 使用 glamour 渲染 markdown，TranslateANSI 转为 tview 颜色标签
	if strings.TrimSpace(Choice.Message.Content) != "" && Choice.Message.Role != "tool" {
		out, _ := glamourRenderer.Render(pretty.TContentNoneStreamTag(Choice.Message.Content))
		out = strings.TrimRight(out, "\n\r ")
		global.PrintToTui(global.AgentMessage, tview.TranslateANSI(out)+"[-:-:-]", false)
	}
}

func renderToolCall(Choice model.Choice) {
	/*------------------此处统一处理工具信息---------------------------------------------------------------------------*/

	//处理工具请求------------------------------------
	//工具请求信息不一定在delta中，也可能在message中，所以两者都要处理
	if len(Choice.Delta.ToolCalls) != 0 {
		for _, toolCall := range Choice.Delta.ToolCalls {
			addToolCallMsg(toolCall)
		}
	}

	if len(Choice.Message.ToolCalls) != 0 {
		for _, toolCall := range Choice.Message.ToolCalls {
			addToolCallMsg(toolCall)
		}
	}
}
func renderToolResult(Choice model.Choice) {
	//处理工具结果------------------------------------
	//工具结果的role是tool，但信息不一定在delta中，也可能在message中，所以两者都要处理

	if Choice.Delta.Role == "tool" {

		addToolResultMsg(Choice.Delta.ToolID, Choice.Delta.Content)
	}
	if Choice.Message.Role == "tool" {
		addToolResultMsg(Choice.Message.ToolID, Choice.Message.Content)
	}

}
