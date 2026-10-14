package tui

// ── 调色板与消息样式（对齐 crush 的 Charmtone 主题）──────────
//
// 消息区视觉语言参考 crush：
//   用户消息   primary 色左竖线 + 1 列缩进（crush Messages.UserBlurred）
//   助手消息   纯左缩进 2 列，无边框无前缀（crush Messages.AssistantBlurred）
//   思考块     极淡底色通栏盒（crush Messages.ThinkingBox，bgLeastVisible）
//   工具行     状态图标 + info 色工具名 + muted 参数（crush toolHeader）
//   错误       红底 tag 徽章 + 弱化正文（crush Messages.ErrorTag/ErrorTitle）
// 颜色取值来自 charmbracelet/x/exp/charmtone（与 crush 默认主题一致）。

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	cPrimary       = lipgloss.Color("#6B50FF") // Charple：用户左线、品牌色
	cInfo          = lipgloss.Color("#00A4FF") // Malibu：工具名、链接、标题
	cSuccess       = lipgloss.Color("#00FFB2") // Julep：工具 ✓、成功
	cSuccessSubtle = lipgloss.Color("#12C78F") // Guac：链接文本、成功正文
	cError         = lipgloss.Color("#EB4268") // Sriracha：错误 tag
	cWarning       = lipgloss.Color("#F5EF34") // Mustard：警告
	cFgBase        = lipgloss.Color("#ECEBF0") // Sash：正文主色
	cFgSubtle      = lipgloss.Color("#BFBCC8") // Smoke：markdown 正文、错误标题
	cFgMuted       = lipgloss.Color("#858392") // Squid：弱化文本、参数、提示
	cFgMostSubtle  = lipgloss.Color("#605F6B") // Oyster：思考块左线
	cCodeBg        = lipgloss.Color("#333333") // 行内代码底色（纯中性灰）
	cCodeBlockBg   = lipgloss.Color("#262626") // 代码块底色（纯中性灰）
	cOnPrimary     = lipgloss.Color("#FFFAF1") // Butter：tag 徽章前景

	// 兼容旧引用：边框沿用 demo 的 "62"（输入框/帮助浮层），次文本对齐 Squid。
	cBorder = lipgloss.Color("62")
	cSub    = cFgMuted

	// 工具块体区竖线：与用户消息（primary 线）、思考块（faint 线）同款视觉
	// 语言。参数区暗灰（输入=既定上下文，Oyster）、结果区淡绿（输出=产出，
	// Guac；饱和度低于 success 主色，错误结果的红色内容仍可辨识）。
	cToolLineIn  = cFgMostSubtle
	cToolLineOut = cSuccessSubtle

	// diff 行条纹底色（crush DefaultDarkStyle Insert/DeleteLine 同款取值）：
	// add/del 行整行成条纹，行号列比内容列深一档；ctx 行保持透明。
	cDiffAddBg    = lipgloss.Color("#303a30")
	cDiffAddGutBg = lipgloss.Color("#293229")
	cDiffDelBg    = lipgloss.Color("#3a3030")
	cDiffDelGutBg = lipgloss.Color("#332929")
)

// maxTextWidth 消息文本的最大列宽（crush 同款 cap：超宽终端下正文不至于拉满，
// 保持可读的行长）。
const maxTextWidth = 120

// textWidth 块内容的可用列宽。
func textWidth(w int) int {
	if w > maxTextWidth {
		return maxTextWidth
	}
	return w
}

// ── 消息块样式 ─────────────────────────────────────────

// msgIndent 助手侧消息块的统一左缩进（crush AssistantBlurred：PaddingLeft(2)，
// 无边框无前缀符号）。
var msgIndent = lipgloss.NewStyle().PaddingLeft(2)

// userBar 用户消息：primary 色左竖线 + 1 列缩进（crush UserBlurred）。
var userBar = lipgloss.NewStyle().
	PaddingLeft(1).
	BorderLeft(true).
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(cPrimary)

// thinkingQuote 思考块：弱化左竖线 + 缩进（crush ThinkingBox 的无底色变体——
// 大面积底色洗在黑底终端上会被读成色块，蓝紫调的尤其难看，弃用）。
// Width 由调用方按文本宽度传入，负责软换行。
func thinkingQuote(w int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(cFgMuted).
		PaddingLeft(1).
		Width(w - 1). // 减去 border 一列
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(cFgMostSubtle)
}

// errorTag 错误徽章：红底浅字（crush Messages.ErrorTag）。
var errorTag = lipgloss.NewStyle().Padding(0, 1).Background(cError).Foreground(cOnPrimary)

// ── 状态/生命周期消息（无装饰符号，语义由颜色承载——沿用项目规则）──

// errText 错误消息：ERROR 徽章 + 弱化正文。
func errText(text string) string {
	return errorTag.Render("ERROR") + " " + colorText(cFgSubtle, text)
}

// successText 成功消息（淡绿正文）。
func successText(text string) string {
	return colorText(cSuccessSubtle, text)
}

// warnText 警告消息（黄）。
func warnText(text string) string {
	return colorText(cWarning, text)
}

// slashEcho 斜杠指令回显（弱化，弱到不抢对话内容的视觉权重）。
func slashEcho(text string) string {
	return colorText(cFgMuted, text)
}

// summaryText 摘要提示（淡绿）。
func summaryText(text string) string {
	return colorText(cSuccessSubtle, "已生成摘要：\n"+text)
}

// ── 工具块头部行（crush toolHeader：状态图标 + info 工具名）──
// 参数行/结果行/内容块由 toolview.go 的 toolView 负责。

// toolHeader 头部行：✓ 工具名（params 预留扩展，空串不出尾随空格）。
func toolHeader(name, params string) string {
	icon := colorText(cSuccess, "✓")
	if params == "" {
		return fmt.Sprintf("%s %s", icon, colorText(cInfo, name))
	}
	return fmt.Sprintf("%s %s %s", icon, colorText(cInfo, name), params)
}

// compactLine 把多行文本压成单行：去掉 ANSI 转义与回车（命令输出常带 PTY 的
// \r\n 和程序自身的颜色码，直接进视图会串色），换行转空格并合并多余空格，
// tab 展开为 4 空格（测量计 0 列而终端展开 8 列，见 cleanBlock 注释）。
// toolview 的参数/标量兜底在用；多行内容块用 toolview.cleanBlock（保留换行）。
func compactLine(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", tabSpaces)
	s = strings.ReplaceAll(s, "  ", " ")
	return strings.TrimSpace(s)
}

// ── bar 通知（单行、无首尾换行）──────────────────────────

// noticeNewConversation 新对话通知（info）。
func noticeNewConversation() string {
	return colorText(cInfo, "new conversation started")
}

// noticeCancelled 取消通知（黄）。
func noticeCancelled() string {
	return colorText(cWarning, "session cancelled")
}

// noticeCleared 清屏通知（info，同 new_conversation 的中性信息色）。
func noticeCleared() string {
	return colorText(cInfo, "history cleared")
}

// noticeSuccess 成功通知（淡绿）。
func noticeSuccess(text string) string {
	return colorText(cSuccessSubtle, text)
}

// noticeWarning 警告通知（黄）。
func noticeWarning(text string) string {
	return colorText(cWarning, text)
}

// noticeSub 兜底：弱化文本色。
func noticeSub(text string) string {
	return colorText(cFgMuted, text)
}

// ── todo 清单栏 ─────────────────────────────────────────

// todoLines 把 TodoBar 的多行纯文本逐行上色：
// [TODO] 头行与 (N done) 计数行弱化、◐ 进行中 info 色、☐ 待办正文色。
// ANSI 输出下方括号是字面量，无需转义。
func todoLines(text string) string {
	colorOf := func(line string) color.Color {
		if strings.HasPrefix(line, "◐ ") {
			return cInfo
		} else if strings.HasPrefix(line, "☐ ") {
			return cFgBase
		} else { // [TODO] 头行、计数行
			return cFgMuted
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = colorText(colorOf(line), line)
	}
	return strings.Join(lines, "\n")
}

// colorText 前景色包装。
func colorText(c color.Color, text string) string {
	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// ── diff 条纹（add/del 行底色）────────────────────────────

// diffKindBg add/del 行的条纹底色（行号列、内容列两档深度）；
// ctx/gap 返回 nil（无底色，保持透明）。
func diffKindBg(kind string) (gut, con color.Color) {
	switch kind {
	case "add":
		return cDiffAddGutBg, cDiffAddBg
	case "del":
		return cDiffDelGutBg, cDiffDelBg
	}
	return nil, nil
}

// diffCell 前景+底色单格；bg 为 nil 退化为纯前景（等价 colorText）。
// 嵌套 colorText 会被内层 reset 打断底色，条纹内的格子必须走这里。
func diffCell(fg, bg color.Color, text string) string {
	s := lipgloss.NewStyle().Foreground(fg)
	if bg != nil {
		s = s.Background(bg)
	}
	return s.Render(text)
}

// bgCell 纯底色格（条纹 gutter 与行尾补齐的空格）。
func bgCell(bg color.Color, text string) string {
	return lipgloss.NewStyle().Background(bg).Render(text)
}

// bgSGRSeq 底色起始序列：ansi.Hardwrap 折行不跨行重发活跃 SGR，
// 续行内容需手动前置。取 lipgloss 渲染 "X" 的前缀段（SGR 只含
// 数字/分号/字母 m，不会撞上 X）。
func bgSGRSeq(c color.Color) string {
	pre, _, _ := strings.Cut(lipgloss.NewStyle().Background(c).Render("X"), "X")
	return pre
}
