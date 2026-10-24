package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// composeView 全量渲染：从消息日志整体重组视图块（无缓存、纯函数式重放，
// 正确性不依赖记账），运行在 pull 链的 goroutine 上。
// 视觉语言对齐 crush：用户消息 primary 左线、助手块左缩进、思考块弱化竖线、
// 工具行折叠三角 + 工具名；块与块之间以空行分隔。
// 思考块与工具块默认折叠（collapse.go），展开状态经 t.expand 查询；
// 返回值为视图全文与可折叠块的行号索引（点击命中用）。
func (t *TUI) composeView(st *pullState) (string, []blockSpan) {
	recs := t.fetchRecords()
	var blocks []viewBlock
	if st.bannerDone {
		blocks = append(blocks, viewBlock{text: strings.Trim(st.banner, "\n")})
	}
	w := textWidth(st.width)

	tailReasoning := &strings.Builder{} // 流式尾段：末尾连续 delta 累积的思考/正文
	tailContent := &strings.Builder{}
	toolBuf := map[string]*wireToolCall{} // 工具调用缓冲（随扫描重建）
	for i, line := range recs {
		var rec wireRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // 坏行直接跳过（记录是自描述 JSON，正常流程不会出现）
		}

		if rec.Type == "delta" {
			var m model.Message
			if rec.Msg == nil || json.Unmarshal(rec.Msg, &m) != nil {
				continue
			}
			if m.Role == "tool" {
				// 结果以 delta 先到时由它消费缓冲：ID 用该记录序号（重放确定性
				// 保证后续定稿记录不会再命中缓冲，块不重复）。
				if entry, result := takeToolResult(&m, toolBuf); entry != nil {
					id := fmt.Sprintf("tool:%d", i)
					blocks = append(blocks, viewBlock{id: id, text: t.toolBlock(entry, result, id, w)})
				}
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
				if entry, result := takeToolResult(&m, toolBuf); entry != nil {
					id := fmt.Sprintf("tool:%d", i)
					blocks = append(blocks, viewBlock{id: id, text: t.toolBlock(entry, result, id, w)})
				}
			} else {
				bufferToolCalls(&m, toolBuf)
				// 思考块并入本条：定稿自带则用它，否则用流式尾累积（两者本应
				// 同文），只渲染一次不重复；ID 用定稿记录序号（跨重组稳定）。
				// 尾段的流式展开在此消费，但清理在 applyFrame 的迁移检测做
				// （这里绝不能清——重放每帧扫历史记录，见 clearTail 注释）。
				thinkID := fmt.Sprintf("think:%d", i)
				if m.ReasoningContent != "" {
					blocks = append(blocks, viewBlock{id: thinkID,
						text: t.thinkingBlock(m.ReasoningContent, thinkID, w)})
				} else if tailReasoning.Len() > 0 {
					blocks = append(blocks, viewBlock{id: thinkID,
						text: t.thinkingBlock(tailReasoning.String(), thinkID, w)})
				}
				tailReasoning.Reset()
				if strings.TrimSpace(m.Content) != "" {
					blocks = append(blocks, viewBlock{text: renderBody(m.Content, w)})
				}
				tailContent.Reset()
			}

		} else if view := boundaryViews[rec.Type]; view != nil {
			// 边界记录：流式尾段先于它上屏并清空累积，再拼本块渲染。
			// 尾段定下唯一块身份（本记录序号），不是共享伪 ID。
			blocks = t.flushTailBlocks(blocks, tailReasoning, tailContent, w, fmt.Sprintf("think:%d", i))
			blocks = append(blocks, viewBlock{text: view(rec.Text, w)})
		}
	}
	// 流式尾段 live：按累积内容整体出渲染版（流式期间看到的就是最终形态）。
	// 尚无记录序号可依附，用伪 ID（冲刷时清理，见 flushTailBlocks）。
	blocks = t.flushTailBlocks(blocks, tailReasoning, tailContent, w, tailThinkID)
	// 终态消息拼在最后（引擎置 fatal 后 park，不再有新记录）
	if st.lastFatal != nil && st.lastFatal.Text != "" {
		blocks = append(blocks, viewBlock{text: renderFatal(st.lastFatal.Text, st.lastFatal.Style, w)})
	}
	return composeBlocks(blocks)
}

// ── 边界记录渲染（签名统一：文本 + 可用宽 w，一个块一个函数）──

// slashView 斜杠回显：单行弱化文本，无宽度概念（参数仅为对齐签名）。
func slashView(text string, _ int) string { return slashEcho(text) }

// warnView 警告块：清洗折行上色后统一 msgIndent 左缩进。
func warnView(text string, w int) string { return msgIndent.Render(warnText(text, w-msgIndentW)) }

// errView 错误块：ERROR 徽章 + 折行正文，统一 msgIndent 左缩进。
func errView(text string, w int) string { return msgIndent.Render(errText(text, w-msgIndentW)) }

// summaryView 摘要块：summary generated 头 + 折行正文，统一 msgIndent 左缩进。
func summaryView(text string, w int) string { return msgIndent.Render(summaryText(text, w-msgIndentW)) }

// boundaryViews 边界记录（自描述文本型：user/slash/warn/error/summary）的
// 渲染分发表。共同形态：流式尾段先于边界记录上屏（flushTailBlocks），再拼本块。
// 宽度预算在各 view 内自理（user 扣左线、状态类扣 msgIndent、slash 不用宽）。
var boundaryViews = map[string]func(text string, w int) string{
	"user":    userBlock,
	"slash":   slashView,
	"warn":    warnView,
	"error":   errView,
	"summary": summaryView,
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

// takeToolResult 渲染工具块的第一步：结果（Role=tool 的框架 message）到达时，
// 按 ToolID 匹配此前缓冲的工具调用并从缓冲消费掉（delta 与定稿谁先到谁消费，
// 重放确定性保证同一结果只渲染一次）。
// 无匹配时返回 nil——宁可不出工具行，也不凭空渲染。
func takeToolResult(m *model.Message, toolBuf map[string]*wireToolCall) (*wireToolCall, string) {
	entry := toolBuf[m.ToolID]
	if entry == nil {
		return nil, ""
	}
	delete(toolBuf, m.ToolID)
	return entry, m.Content
}

// renderThinking 思考块：弱化左竖线 + 弱化文本（quote 风格）。
// reasoning 是思考散文，不走 glamour（结构化 markdown 无增益，纯文本渲染
// 成本也低一个量级）；样式的 Width 负责软换行。
func renderThinking(r string, w int) string {
	gw := w - msgIndentW - thinkingFrameW
	if gw < 10 {
		gw = 10
	}
	return msgIndent.Render(thinkingQuote(gw).Render(r))
}

// userBlock 用户消息：primary 左线 + markdown 渲染。保留换行，
// 避免渲染后与输入时所见不一致。
func userBlock(text string, w int) string {
	gw := w - userBarFrameW
	if gw < 10 {
		gw = 10
	}
	out, err := renderMarkdownUser(text, gw)
	if err != nil {
		out = text // 渲染失败退回原文，别把输入吞掉
	}
	out = stripGlamourPad(out)
	return userBar.Render(strings.TrimRight(out, "\n"))
}

// ── markdown 渲染（glamour + crush 风格 StyleConfig）────────

// msgIndentW msgIndent 的左缩进列数（思考盒等需要扣掉它再对宽）。
const msgIndentW = 2

// thinkingFrameW 思考块边框占宽：左竖线(1) + 内边距(1)。
const thinkingFrameW = 2

// userBarFrameW 用户消息左线占宽：边框(1) + 内边距(1)。
const userBarFrameW = 2

// renderBody 用 glamour 渲染助手正文，ANSI 输出直接进 viewport。
func renderBody(content string, w int) string {
	gw := w - msgIndentW
	if gw < 10 {
		gw = 10
	}
	out, err := renderMarkdownMain(content, gw)
	if err != nil {
		out = content // 渲染失败退回原文，别把整条回复吞掉
	}
	out = stripGlamourPad(out)
	return msgIndent.Render(strings.TrimRight(out, "\n"))
}

// stripGlamourPad 剥掉 glamour 输出首尾的空行。不剥行尾填充：正则剥填充
// 会连带剥掉闭合 SGR（曾导致规则线错位）；填充是不可见空格，直接放行。
func stripGlamourPad(out string) string {
	return strings.Trim(out, "\n\r")
}

// renderNotice 见 engine.go/notice 相关：把通知槽位翻译成带色文本（文案配色
// 在 TUI 侧，引擎只存 kind 与原文；无文本的固定文案也由本侧拼装）。
func renderNotice(kind, text string) string {
	if kind == "new_conversation" {
		return noticeNewConversation()
	} else if kind == "cancelled" {
		return noticeCancelled()
	} else if kind == "cleared" {
		return noticeCleared()
	} else if kind == "success" {
		return noticeSuccess(text)
	} else if kind == "warning" {
		return noticeWarning(text)
	} else {
		return noticeSub(text)
	}
}

// hintSpinnerW 底部 spinner 固定占宽（圆点轨道 12 列）；hint 必须给它让位。
const hintSpinnerW = 12

// composeHint 常驻兜底提示。按宽度逐档降级：底部是单行栏，超宽会折行并
// 改变视口高度（recalcComponentSize 每帧按它算），窄终端宁可少说一句。
// 每档都是完整可读的提示，不做词中截断。
func composeHint(running bool, w int) string {
	avail := w - hintSpinnerW
	full := "type / for commands · click ▸ to expand"
	if running {
		full = "esc to interrupt · " + full
	}
	short := "/ commands · click ▸"
	if running {
		short = "esc interrupt · / · click ▸"
	}
	tiny := "/ · ▸"
	if running {
		tiny = "esc · / · ▸"
	}
	for _, h := range []string{full, short, tiny} {
		if xansi.StringWidth(h) <= avail {
			return noticeSub(h)
		}
	}
	return noticeSub("▸")
}

// renderFatal 把终态消息按样式上色并折行（引擎只存语义原文与 style 字符串）。
// 终态无 msgIndent，宽度预算为整行 w。
// "exit"（/exit 终态）无文案、不再走这里渲染。
func renderFatal(text, style string, w int) string {
	if style == "success" {
		return successText(text, w)
	} else if style == "error" {
		return errText(text, w)
	} else {
		return wrapStatusText(text, w)
	}
}

// ── glamour 渲染（每次直建）──
//
// 不缓存渲染器：单次构建实测 ~28µs，每帧合计 <1ms，不值得缓存。

// newGlamourRenderer 构建终端 markdown 渲染器（宽度小于 40 时 glamour 样式
// 会严重劣化，兜底 80）。
func newGlamourRenderer(w int, preserveNL bool) (*glamour.TermRenderer, error) {
	if w < 40 {
		w = 80
	}
	opts := []glamour.TermRendererOption{
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(w),
	}
	if preserveNL {
		opts = append(opts, glamour.WithPreservedNewLines())
	}
	return glamour.NewTermRenderer(opts...)
}

// renderMarkdownMain 助手正文 → glamour 渲染。
func renderMarkdownMain(content string, w int) (string, error) {
	r, err := newGlamourRenderer(w, false)
	if err != nil {
		return "", err
	}
	return r.Render(content)
}

// renderMarkdownUser 用户输入 → glamour 渲染（保留换行）。
func renderMarkdownUser(text string, w int) (string, error) {
	r, err := newGlamourRenderer(w, true)
	if err != nil {
		return "", err
	}
	return r.Render(text)
}

// ── crush 风格 markdown StyleConfig ──────────────────────
//
// 取值对齐 crush 的 quickStyle（charmtone 调色板），替换 glamour "dark" 预设。

const (
	crushMargin     = 2
	crushListIndent = 2
)

func hexOf(c string) *string { return &c }

func markdownStyle() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			// Sash 最亮档：暗底终端上正文要够亮才醒目（原 Smoke 衬得发灰）
			StylePrimitive: ansi.StylePrimitive{Color: hexOf("#ECEBF0")},
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
