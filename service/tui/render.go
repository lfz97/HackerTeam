package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// composeView 全量渲染：从消息日志整体重组视图文本（无缓存、纯函数式重放，
// 正确性不依赖任何记账）。每次重组都对全部记录重放状态机（流式尾段、工具缓冲）
// 并重新渲染，运行在 pull 链的 goroutine 上。
//
// 视觉语言对齐 crush：用户消息带 primary 左线、助手块统一左缩进、思考块底色
// 盒、工具行 "✓ name args"、错误带徽章；块与块之间以空行分隔。
func (t *TUI) composeView(st *pullState) string {
	recs := t.fetchRecords()
	var parts []string
	if st.bannerDone {
		parts = append(parts, strings.Trim(st.banner, "\n"))
	}
	w := textWidth(st.width)

	tailReasoning := &strings.Builder{} // 流式尾段：末尾连续 delta 累积的思考/正文
	tailContent := &strings.Builder{}
	toolBuf := map[string]*wireToolCall{} // 工具调用缓冲（随扫描重建）
	for _, line := range recs {
		var rec wireRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // 坏行直接跳过（记录是自描述 JSON，正常流程不会出现）
		}

		var out string
		if rec.Type == "delta" {
			var m model.Message
			if rec.Msg == nil || json.Unmarshal(rec.Msg, &m) != nil {
				continue
			}
			if m.Role == "tool" {
				out = renderToolResult(&m, toolBuf)
			} else {
				tailReasoning.WriteString(m.ReasoningContent)
				tailContent.WriteString(m.Content)
				bufferToolCalls(&m, toolBuf)
			}

		} else if rec.Type == "message" {
			var m model.Message
			if rec.Msg == nil || json.Unmarshal(rec.Msg, &m) != nil {
				continue
			}
			if m.Role == "tool" {
				out = renderToolResult(&m, toolBuf)
			} else {
				bufferToolCalls(&m, toolBuf)
				// 思考块并入本条输出（定稿自带则用它，否则用流式尾累积的，两者本应同文），
				// 流式期间与定稿后的观感一致，没有切换跳变。
				var body []string
				if m.ReasoningContent != "" {
					body = append(body, renderThinking(m.ReasoningContent, w))
				} else if tailReasoning.Len() > 0 {
					body = append(body, renderThinking(tailReasoning.String(), w))
				}
				tailReasoning.Reset()
				if strings.TrimSpace(m.Content) != "" {
					body = append(body, renderBody(st, m.Content, w))
				}
				tailContent.Reset()
				out = strings.Join(body, "\n\n")
			}

		} else if rec.Type == "user" {
			// 流式尾段先于边界记录上屏，并清空累积。
			out = flushTail(st, tailReasoning, tailContent, w) + userBlock(st, rec.Text, w)

		} else if rec.Type == "slash" {
			out = flushTail(st, tailReasoning, tailContent, w) + slashEcho(rec.Text)

		} else if rec.Type == "warn" {
			out = flushTail(st, tailReasoning, tailContent, w) + msgIndent.Render(warnText(rec.Text))

		} else if rec.Type == "error" {
			out = flushTail(st, tailReasoning, tailContent, w) + msgIndent.Render(errText(rec.Text))

		} else if rec.Type == "summary" {
			out = flushTail(st, tailReasoning, tailContent, w) + msgIndent.Render(summaryText(rec.Text))
		}
		if out != "" {
			parts = append(parts, out)
		}
	}
	// 流式尾段 live：按累积内容整体出渲染版（流式期间看到的就是最终形态）
	if tailReasoning.Len() > 0 {
		parts = append(parts, renderThinking(tailReasoning.String(), w))
	}
	if tailContent.Len() > 0 {
		parts = append(parts, renderBody(st, tailContent.String(), w))
	}
	// 终态消息拼在最后（引擎置 fatal 后 park，不再有新记录）
	if st.lastFatal != nil && st.lastFatal.Text != "" {
		parts = append(parts, renderFatal(st.lastFatal.Text, st.lastFatal.Style))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// bufferToolCalls 缓冲工具调用（等结果到达后拼工具行）。
// provider 可能把一次调用拆成多个增量片段：同 ID 合并、无 ID 按 index 兜底、
// 名字非空才覆盖、参数按字节续接——完整调用与分片调用两种形态都稳。
func bufferToolCalls(m *model.Message, toolBuf map[string]*wireToolCall) {
	for i, tc := range m.ToolCalls {
		key := tc.ID
		if key == "" {
			key = fmt.Sprintf("#%d", i)
		}
		entry := toolBuf[key]
		if entry == nil {
			entry = &wireToolCall{}
			toolBuf[key] = entry
		}
		if tc.Function.Name != "" {
			entry.name = tc.Function.Name
		}
		entry.args += string(tc.Function.Arguments)
	}
}

// renderToolResult 渲染工具行：结果（Role=tool 的框架 message）到达时，按 ToolID
// 匹配此前缓冲的工具调用（toolCompact 内建参数/结果压缩与截断，原始数据直喂即可）。
// 无缓冲匹配时与旧版一致：宁可不出工具行，也不凭空渲染。
func renderToolResult(m *model.Message, toolBuf map[string]*wireToolCall) string {
	entry := toolBuf[m.ToolID]
	if entry == nil {
		return ""
	}
	delete(toolBuf, m.ToolID)
	return msgIndent.Render(toolCompact(entry.name, []byte(entry.args), m.Content))
}

// renderThinking 思考块：弱化左竖线 + 弱化文本（quote 风格，无底色）。
// 纯函数，流式尾段与定稿同型，观感一致。reasoning 是模型的思考散文，
// 不走 glamour（结构化 markdown 对它没有增益，纯文本渲染在长思考流式
// 重放下的成本也低一个量级）；样式的 Width 负责软换行。
func renderThinking(r string, w int) string {
	return msgIndent.Render(thinkingQuote(w - msgIndentW).Render(r))
}

// flushTail 渲染流式尾段并清空累积，返回值作为边界记录（user/slash/warn/
// error/summary）输出的一部分——边界记录到达时，未定稿的流式内容先于它上屏。
func flushTail(st *pullState, tailReasoning, tailContent *strings.Builder, w int) string {
	var parts []string
	if tailReasoning.Len() > 0 {
		parts = append(parts, renderThinking(tailReasoning.String(), w))
	}
	if tailContent.Len() > 0 {
		parts = append(parts, renderBody(st, tailContent.String(), w))
	}
	tailReasoning.Reset()
	tailContent.Reset()
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n\n"
}

// userBlock 用户消息：primary 左线 + markdown 渲染（crush UserBlurred +
// UserMarkdownRenderer——用户输入在 textarea 里手打，保留换行，避免渲染后
// 和输入时看到的不一样）。
func userBlock(st *pullState, text string, w int) string {
	out, err := st.glamRenderer.user(text, w)
	if err != nil {
		out = text // 渲染失败退回原文，别把输入吞掉
	}
	out = stripGlamourPad(out)
	return userBar.Render(strings.TrimRight(out, "\n"))
}

// ── markdown 渲染（glamour + crush 风格 StyleConfig）────────

// msgIndentW msgIndent 的左缩进列数（思考盒等需要扣掉它再对宽）。
const msgIndentW = 2

// glamourTailPad 匹配行尾的"空白 + ANSI 序列"混合填充。glamour 会把标题、表格、
// 代码块的每一行都补满整行宽度，每个空格还裹一层 SGR：实测 153B 的 markdown
// 渲染后是 12.7KB，其中 92% 是这种填充。消息区每帧全量重绘、每次重组都要扫
// 一遍整个 buffer，所以必须剥掉。
var glamourTailPad = regexp.MustCompile(`(?:\x1b\[[0-9;]*m|[ \t])+$`)

// renderBody 用 glamour 渲染助手正文（crush Markdown StyleConfig）。ANSI 输出
// 直接进 viewport——bubbletea 渲染器本身就是 ANSI 消费者。
func renderBody(st *pullState, content string, w int) string {
	out, err := st.glamRenderer.main(content, w)
	if err != nil {
		out = content // 渲染失败退回原文，别把整条回复吞掉
	}
	out = stripGlamourPad(out)
	return msgIndent.Render(strings.TrimRight(out, "\n"))
}

// stripGlamourPad 剥掉 glamour 输出的首尾换行与行尾填充，并补回行尾 reset
// （剥填充会连带剥掉闭合样式的 SGR，不补则颜色泄漏到后续行）。
func stripGlamourPad(out string) string {
	out = strings.TrimRight(strings.TrimLeft(out, "\n\r"), "\n\r ")
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		l = glamourTailPad.ReplaceAllString(l, "")
		if l != "" && !strings.HasSuffix(l, "\x1b[m") {
			l += "\x1b[m"
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// renderNotice 见 engine.go/notice 相关：把通知槽位翻译成带色文本（文案配色
// 在 TUI 侧，引擎只存 kind 与原文；无文本的固定文案也由本侧拼装）。
func renderNotice(kind, text string) string {
	if kind == "new_conversation" {
		return noticeNewConversation()
	} else if kind == "cancelled" {
		return noticeCancelled()
	} else if kind == "success" {
		return noticeSuccess(text)
	} else if kind == "warning" {
		return noticeWarning(text)
	} else {
		return noticeSub(text)
	}
}

// composeHint 常驻兜底提示。esc to interrupt 只在运行态出现——
// ESC 中断仅在 agent 运行期间有效，平时显示它是噪音。
func composeHint(running bool) string {
	if running {
		return noticeSub("esc to interrupt · ctrl+k for help")
	}
	return noticeSub("ctrl+k for help")
}

// renderFatal 把终态消息按样式上色（引擎只存语义原文与 style 字符串）。
// "exit"（/exit 终态）无文案、不再走这里渲染。
func renderFatal(text, style string) string {
	if style == "success" {
		return successText(text)
	} else if style == "error" {
		return errText(text)
	} else {
		return text
	}
}

// ── glamour renderer 缓存（按宽度，pull 链单 goroutine 串行使用）──

// glamourCache 按宽度缓存 main（助手正文）/user（用户输入，保留换行）两个
// renderer。重建要解析一遍 style 配置，不能每条消息重建。
type glamourCache struct {
	mainR *glamour.TermRenderer
	userR *glamour.TermRenderer
	width int
}

// renderOut 渲染器落缓存（宽度变化时重建）。
func (c *glamourCache) renderOut(w int) error {
	if c.width == w && c.mainR != nil && c.userR != nil {
		return nil
	}
	// 宽度小于 40 时 glamour 样式会严重劣化，兜底 80
	if w < 40 {
		w = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		return err
	}
	ur, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(w),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return err
	}
	c.mainR, c.userR, c.width = r, ur, w
	return nil
}

func (c *glamourCache) main(in string, w int) (string, error) {
	if err := c.renderOut(w); err != nil {
		return "", err
	}
	return c.mainR.Render(in)
}

func (c *glamourCache) user(in string, w int) (string, error) {
	if err := c.renderOut(w); err != nil {
		return "", err
	}
	return c.userR.Render(in)
}

// ── crush 风格 markdown StyleConfig ──────────────────────
//
// 取值对齐 crush 的 quickStyle（charmtone 调色板）：正文 Smoke、标题 Malibu、
// H1 是 primary 底的徽章、代码块 Char 底 + Chroma 语法色。替换 glamour 的
// "dark" 预设——预设的配色与本项目的消息视觉语言不搭。

const (
	crushMargin     = 2
	crushListIndent = 2
)

func hexOf(c string) *string { return &c }

func markdownStyle() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: hexOf("#BFBCC8")},
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{},
			Indent:         new(uint(crushMargin)),
			IndentToken:    new(string("│ ")),
		},
		List: ansi.StyleList{LevelIndent: crushListIndent},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockSuffix: "\n",
				Color:       hexOf("#00A4FF"),
				Bold:        new(bool(true)),
			},
		},
		H1: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "# ",
				Color:  hexOf("#6B50FF"),
				Bold:   new(bool(true)),
			},
		},
		H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
		H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
		H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
		H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
		H6: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "###### ",
				Color:  hexOf("#12C78F"),
				Bold:   new(bool(false)),
			},
		},
		Strikethrough: ansi.StylePrimitive{CrossedOut: new(bool(true))},
		Emph:          ansi.StylePrimitive{Italic: new(bool(true))},
		Strong:        ansi.StylePrimitive{Bold: new(bool(true))},
		HorizontalRule: ansi.StylePrimitive{
			Color:  hexOf("#3A3943"),
			Format: "\n--------\n",
		},
		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task: ansi.StyleTask{
			StylePrimitive: ansi.StylePrimitive{},
			Ticked:         "[✓] ",
			Unticked:       "[ ] ",
		},
		Link:     ansi.StylePrimitive{Color: hexOf("#00A4FF"), Underline: new(bool(true))},
		LinkText: ansi.StylePrimitive{Color: hexOf("#12C78F"), Bold: new(bool(true))},
		Image:    ansi.StylePrimitive{Color: hexOf("#FF60FF"), Underline: new(bool(true))},
		ImageText: ansi.StylePrimitive{
			Color:  hexOf("#858392"),
			Format: "Image: {{.text}} →",
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix:          " ",
				Suffix:          " ",
				Color:           hexOf("#EB4268"),
				BackgroundColor: hexOf("#333333"), // 纯中性灰——蓝紫调底色在黑底终端上会被读成蓝色块
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Color: hexOf("#3A3943")},
				Margin:         new(uint(crushMargin)),
			},
			Chroma: &ansi.Chroma{
				Text:            ansi.StylePrimitive{Color: hexOf("#BFBCC8")},
				Error:           ansi.StylePrimitive{Color: hexOf("#FFFAF1"), BackgroundColor: hexOf("#EB4268")},
				Comment:         ansi.StylePrimitive{Color: hexOf("#605F6B")},
				CommentPreproc:  ansi.StylePrimitive{Color: hexOf("#FF60FF")},
				Keyword:         ansi.StylePrimitive{Color: hexOf("#00A4FF")},
				KeywordReserved: ansi.StylePrimitive{Color: hexOf("#FF84FF")},
				KeywordType:     ansi.StylePrimitive{Color: hexOf("#4FBEFE")},
				Operator:        ansi.StylePrimitive{Color: hexOf("#BFBCC8")},
				Punctuation:     ansi.StylePrimitive{Color: hexOf("#E8FE96")},
				Name:            ansi.StylePrimitive{Color: hexOf("#BFBCC8")},
				NameBuiltin:     ansi.StylePrimitive{Color: hexOf("#FF60FF")},
				NameTag:         ansi.StylePrimitive{Color: hexOf("#FF60FF")},
				NameAttribute:   ansi.StylePrimitive{Color: hexOf("#4FBEFE")},
				NameClass:       ansi.StylePrimitive{Color: hexOf("#6B50FF"), Underline: new(bool(true)), Bold: new(bool(true))},
				NameDecorator:   ansi.StylePrimitive{Color: hexOf("#F5EF34")},
				NameFunction:    ansi.StylePrimitive{Color: hexOf("#12C78F")},
				LiteralNumber:   ansi.StylePrimitive{Color: hexOf("#00FFB2")},
				LiteralString:   ansi.StylePrimitive{Color: hexOf("#E8FE96")},
				Background:      ansi.StylePrimitive{BackgroundColor: hexOf("#262626")}, // 纯中性灰
			},
		},
		Table: ansi.StyleTable{
			StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{}},
		},
		DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n "},
	}
}
