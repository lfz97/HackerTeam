package messagerender

import (
	"HackerTeam/service/engine/tools"
	"HackerTeam/utils/pretty"
	"github.com/rivo/tview"
	"github.com/tidwall/gjson"
	"regexp"
	"strings"
	"sync"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type ResponseMessageStatus struct {
	startReasoning bool
	Stream         bool
	ShowReasoning  bool
}

type MessageRender struct {
	tui    tuiService
	Status *ResponseMessageStatus
	Buffer *toolMsgBuffer
}

type tuiService interface {
	PrintToMsgView(content string, clear bool)
	ReplaceTailInMsgView(raw string, replacement string) bool
	RenderMarkdown(in string) (string, error)
}

// glamourTailPad 匹配行尾的"空白 + ANSI 序列"混合填充。glamour 会把标题、表格、
// 代码块的每一行都补满整行宽度，每个空格还裹一层 SGR：实测 153B 的 markdown 渲染后
// 是 12.7KB，其中 92% 是这种填充。消息区 TextView 在进程生命周期内从不清空，且每帧
// 全量重绘、每次替换都要扫一遍整个 buffer，所以必须剥掉。
var glamourTailPad = regexp.MustCompile(`(?:\x1b\[[0-9;]*m|[ \t])+$`)

func NewMessageRender(tui tuiService, ShowReasoning bool, Stream bool) *MessageRender {

	return &MessageRender{
		tui: tui,
		Status: &ResponseMessageStatus{
			Stream:        Stream,
			ShowReasoning: ShowReasoning,
		},
		Buffer: &toolMsgBuffer{
			mu:           sync.Mutex{},
			toolMessages: []*toolMessage{},
		},
	}
}

// startReasoning 用于标记是否正在输出思考内容。只在stream为true的情况下使用，对于思考内容的渲染方式和正文不同
func (r *MessageRender) RenderResponse(Choice model.Choice, isPartial bool) {

	if (*(*r).Status).Stream {
		r.renderStreamEvent(Choice, isPartial)

	} else {
		r.renderNonStreamEvent(Choice)
	}

	r.gatherToolMessage(Choice)
	r.renderToolMessages()
}

func (r *MessageRender) renderStreamEvent(Choice model.Choice, isPartial bool) {
	if Choice.Delta.ReasoningContent != "" {
		if !(*(*r).Status).startReasoning {
			if (*(*r).Status).ShowReasoning {
				(*r).tui.PrintToMsgView("\n", false)
			}
			(*(*r).Status).startReasoning = true
		}
		if (*(*r).Status).ShowReasoning {
			// 思考内容
			(*r).tui.PrintToMsgView(pretty.TReasoningContent(Choice.Delta.ReasoningContent), false)
		}
	} else if (*(*r).Status).startReasoning {
		(*(*r).Status).startReasoning = false
		if (*(*r).Status).ShowReasoning {
			(*r).tui.PrintToMsgView("\n", false)
		}
	}
	if Choice.Delta.Content != "" && Choice.Delta.Role != "tool" {
		// 正文内容（工具响应片段不作为正文渲染，由下方统一处理工具信息部分处理）
		(*r).tui.PrintToMsgView(Choice.Delta.Content, false)
	}

	// 合并响应：本 hop 的正文已完整，把刚流出去的 raw 替换成渲染版。两个 provider 实测
	// 都是每 hop 恰好一个这样的响应，且 Message.Content 与所有 Delta.Content 的拼接字节相同。
	//
	// 判据只能用 IsPartial：同样在带工具调用的 hop 上，openai 给 Done=false 而 anthropic 给
	// Done=true（Done 的框架语义是"flow 是否该停"），IsFinalResponse() 则因带工具调用恒为 false。
	// Role=="tool" 也要挡：工具结果事件的 Message.Content 非空且 Role 是 tool。
	if !isPartial && strings.TrimSpace(Choice.Message.Content) != "" && Choice.Message.Role != "tool" {
		// 替换失败说明 raw 已不在 buffer 末尾（中途被别的写入者插过），保持 raw 即可。
		// 绝不能退化成追加，否则正文会重复显示一遍。
		(*r).tui.ReplaceTailInMsgView(Choice.Message.Content, r.renderBody(Choice.Message.Content))
	}
}

func (r *MessageRender) renderNonStreamEvent(Choice model.Choice) {
	// 思考信息 - 根据配置决定是否显示
	if Choice.Message.ReasoningContent != "" && (*(*r).Status).ShowReasoning {
		(*r).tui.PrintToMsgView("\n", false)
		(*r).tui.PrintToMsgView(pretty.TReasoningContent(Choice.Message.ReasoningContent), false)
		(*r).tui.PrintToMsgView("\n", false)
	}
	// 正文内容
	if strings.TrimSpace(Choice.Message.Content) != "" && Choice.Message.Role != "tool" {
		(*r).tui.PrintToMsgView(r.renderBody(Choice.Message.Content), false)
	}
}

// renderBody 用 glamour 渲染 markdown，TranslateANSI 转为 tview 颜色标签。
// 流式与非流式两条路径共用，保证两种模式的最终观感一致。
//
// 正文标记必须加在渲染结果上，不能加在 markdown 源码前面：`● ` 会让首行的块级结构失效
// ——实测代码围栏和表格会整个塌成一行、列表首项不再被识别、标题降级成普通段落。
func (r *MessageRender) renderBody(content string) string {
	out, err := (*r).tui.RenderMarkdown(content)
	if err != nil {
		out = content // 渲染失败退回原文，别把整条回复吞掉
	}

	// glamour 的输出恒以一个换行开头，不剥掉的话标记会独占一行、与正文脱开
	out = strings.TrimRight(strings.TrimLeft(out, "\n\r"), "\n\r ")
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		l = glamourTailPad.ReplaceAllString(l, "")
		// 剥填充会连带剥掉行尾闭合样式的 reset，不补回则颜色泄漏到后续行
		// （实测不补会留下 2~5 个未闭合标签）
		if l != "" && !strings.HasSuffix(l, "\x1b[m") {
			l += "\x1b[m"
		}
		lines[i] = l
	}
	// 标记加在第一个有可见内容的行上：正文以代码块或表格开头时，
	// 剥完填充后首行是空的，直接前置会让 ● 独占一行
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = pretty.TContentNoneStreamTag(l)
			break
		}
	}
	return tview.TranslateANSI(strings.Join(lines, "\n")) + "[-:-:-]"
}

func (r *MessageRender) gatherToolMessage(Choice model.Choice) {

	//处理工具请求------------------------------------
	//工具请求信息不一定在delta中，也可能在message中，所以两者都要处理
	if len(Choice.Delta.ToolCalls) != 0 {
		for _, toolCall := range Choice.Delta.ToolCalls {
			r.addToolCallMsg(toolCall)
		}
	}

	if len(Choice.Message.ToolCalls) != 0 {
		for _, toolCall := range Choice.Message.ToolCalls {
			r.addToolCallMsg(toolCall)
		}
	}

	//处理工具结果------------------------------------
	//工具结果的role是tool，但信息不一定在delta中，也可能在message中，所以两者都要处理

	if Choice.Delta.Role == "tool" {

		r.addToolResultMsg(Choice.Delta.ToolID, Choice.Delta.Content)
	}
	if Choice.Message.Role == "tool" {
		r.addToolResultMsg(Choice.Message.ToolID, Choice.Message.Content)
	}
}

// 将toolcall消息放进临时buffer中
func (r *MessageRender) addToolCallMsg(toolcall model.ToolCall) {

	(*(*r).Buffer).mu.Lock()
	defer (*(*r).Buffer).mu.Unlock()

	(*(*r).Buffer).toolMessages = append((*(*r).Buffer).toolMessages, &toolMessage{
		Id:                toolcall.ID,
		FunctionName:      toolcall.Function.Name,
		FunctionArguments: toolcall.Function.Arguments,
	})
}

// 将toolresult消息，按照id一一对应放进buffer中
func (r *MessageRender) addToolResultMsg(ToolID string, content string) {

	(*(*r).Buffer).mu.Lock()
	defer (*(*r).Buffer).mu.Unlock()
	for _, msg_p := range (*(*r).Buffer).toolMessages {
		if (*msg_p).Id == ToolID {
			(*msg_p).Result = content
			(*msg_p).hasResult = true
		}
	}

}

func (r *MessageRender) renderToolMessages() {
	mappers := tools.GetParamMapper()
	for _, msg_p := range (*(*r).Buffer).toolMessages {
		if !(*msg_p).hasResult || (*msg_p).printed {
			continue
		}
		(*msg_p).printed = true
		in, out := renderToolMessageByMapper(mappers, msg_p)
		(*r).tui.PrintToMsgView(pretty.TToolCompact(
			(*msg_p).FunctionName,
			[]byte(in),
			out,
		), false)
	}

}

func renderToolMessageByMapper(mappers *tools.Mappers, msg *toolMessage) (string, string) {
	for _, mapper := range *mappers {
		if (*mapper).Name == (*msg).FunctionName {
			in := ""
			for _, in_arg := range (*mapper).In {
				in += gjson.GetBytes((*msg).FunctionArguments, in_arg).String() + "  "
			}
			out := ""
			for _, out_arg := range (*mapper).Out {
				out += gjson.GetBytes([]byte((*msg).Result), out_arg).String() + "  "
			}
			in = "(" + strings.TrimSuffix(in, "  ") + ")"
			out = "(" + strings.TrimSuffix(out, "  ") + ")"
			return in, out
		}
	}
	return string((*msg).FunctionArguments), (*msg).Result
}
