package pretty

import (
	"fmt"
	"os"
	"strings"
)

// ========== 颜色定义 ==========
const (
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorGray    = "\033[90m"
	ColorReset   = "\033[0m"

	// 背景色
	ColorBgRed    = "\033[41m"
	ColorBgGreen  = "\033[42m"
	ColorBgYellow = "\033[43m"
	ColorBgBlue   = "\033[44m"

	// 样式
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"
)

// ========== TView 颜色定义 (tview 格式) ==========
const (
	TColorRed     = "red"
	TColorGreen   = "green"
	TColorYellow  = "yellow"
	TColorBlue    = "blue"
	TColorMagenta = "magenta"
	TColorCyan    = "cyan"
	TColorWhite   = "white"
	TColorGray    = "gray"
	TColorBlack   = "black"
	TColorOrange  = "orange"
	TColorSkyBlue = "#4FC3F7"

	// 浅色版本
	TColorLightRed     = "lightred"
	TColorLightGreen   = "lightgreen"
	TColorLightYellow  = "lightyellow"
	TColorLightBlue    = "lightblue"
	TColorLightMagenta = "#B39DDB"
	TColorLightCyan    = "lightcyan"
	TColorLightWhite   = "lightwhite"
	TColorLightGray    = "lightgray"

	//特殊版本
	TColorClaudeCodeOrange = "#f7b786" // Claude Code 橙色

)

// ── TView 背景色预设（适合 tview 动态颜色标签 [foreground:background:attr]）──
const (
	TBgSilver      = "#C0C0C0" // 银灰 — 最常用浅灰底
	TBgLightGray   = "#D3D3D3" // 浅灰
	TBgDarkGray    = "#696969" // 暗灰
	TBgGainsboro   = "#DCDCDC" // 极浅灰
	TBgMistyRose   = "#FFE4E1" // 浅粉
	TBgLavender    = "#E6E6FA" // 淡紫
	TBgLightCyan   = "#E0FFFF" // 浅青
	TBgLightYellow = "#FFFFE0" // 浅黄（便利贴感）
	TBgHoneydew    = "#F0FFF0" // 蜜瓜绿
	TBgAliceBlue   = "#F0F8FF" // 爱丽丝蓝
	TBgSeashell    = "#FFF5EE" // 贝壳白
	TBgLinen       = "#FAF0E6" // 亚麻色
)

// ========== TUI 界面配色（GitHub 深色模式 + 深空蓝调）==========
const (
	TuiBg          = "#0F1115" // 整体背景色
	TuiPanelBg     = "#151821" // 面板/侧边栏背景色
	TuiBorderColor = "#2A2F3A" // 边框颜色
	TuiStatusBarBg = "#151821" // 状态栏背景色
	TuiInputAreaBg = "#151821" // 输入区背景色
	TuiSplitLine   = "#4A5060" // 分割线颜色
	TuiMainText    = "#C9D1D9" // 主文本颜色
	TuiSubText     = "#8B949E" // 次文本颜色
	TuiStatusHint  = "#6FC3DF" // 状态提示颜色
)

// ========== 分隔线样式 ==========
const (
	SeparatorLine  = "─"
	SeparatorThick = "═"
)

// ========== 符号定义 ==========
var (
	// 状态符号
	SymbolSuccess     = "✓"
	SymbolError       = "✗"
	SymbolWarning     = "⚠"
	SymbolInfo        = "ℹ"
	SymbolQuestion    = "❓"
	SymbolThinking    = "💭"
	SymbolRobot       = "🤖"
	SymbolUser        = "👤"
	SymbolExit        = "👋"
	SymbolWelcome     = "🌟"
	SymbolLoading     = "⟳"
	SymbolBullet      = "•"
	SymbolArrow       = "→"
	SymbolDoubleArrow = "⇒"
)

// ========== 标题与分隔线 ==========

// SectionTitle 输出 section 标题
func SectionTitle(title string) {
	fmt.Printf("%s%s%s %s %s\n", ColorCyan, Bold, strings.Repeat(SeparatorLine, 25), title, ColorReset)
}

// SectionEnd 输出 section 结束
func SectionEnd() {
	fmt.Println(ColorGray + strings.Repeat(SeparatorLine, 60) + ColorReset)
}

// Header 输出标题
func Header(text string) {
	padding := (60 - len(text)) / 2
	fmt.Printf("%s%s%s%s%s\n", ColorBlue, Bold, strings.Repeat(" ", padding), text, ColorReset)
}

// SubHeader 输出副标题
func SubHeader(text string) {
	fmt.Printf("%s%s%s\n", ColorCyan, text, ColorReset)
}

// Divider 输出分隔线
func Divider() {
	fmt.Println(ColorGray + strings.Repeat(SeparatorLine, 60) + ColorReset)
}

// DividerThick 输出粗分隔线
func DividerThick() {
	fmt.Println(ColorWhite + strings.Repeat(SeparatorThick, 60) + ColorReset)
}

// ========== 提示信息 ==========

// Welcome 输出欢迎信息
func Welcome(text string) {
	fmt.Printf("%s%s %s %s\n", ColorGreen, SymbolWelcome, text, ColorReset)
}

// Success 输出成功信息
func Success(text string) {
	fmt.Printf("%s%s %s%s\n", ColorGreen, SymbolSuccess, ColorReset, text)
}

// SuccessF 输出格式化成功信息
func SuccessF(format string, args ...interface{}) {
	fmt.Printf("%s%s %s%s\n", ColorGreen, SymbolSuccess, ColorReset, fmt.Sprintf(format, args...))
}

// Error 输出错误信息
func Error(text string) {
	fmt.Printf("%s%s %s%s\n", ColorRed, SymbolError, ColorReset, text)
}

// ErrorF 输出格式化错误信息
func ErrorF(format string, args ...interface{}) {
	fmt.Printf("%s%s %s%s\n", ColorRed, SymbolError, ColorReset, fmt.Sprintf(format, args...))
}

// Warning 输出警告信息
func Warning(text string) {
	fmt.Printf("%s%s %s%s\n", ColorYellow, SymbolWarning, ColorReset, text)
}

// WarningF 输出格式化警告信息
func WarningF(format string, args ...interface{}) {
	fmt.Printf("%s%s %s%s\n", ColorYellow, SymbolWarning, ColorReset, fmt.Sprintf(format, args...))
}

// Info 输出提示信息
func Info(text string) {
	fmt.Printf("%s%s %s%s\n", ColorBlue, SymbolInfo, ColorReset, text)
}

// InfoF 输出格式化提示信息
func InfoF(format string, args ...interface{}) {
	fmt.Printf("%s%s %s%s\n", ColorBlue, SymbolInfo, ColorReset, fmt.Sprintf(format, args...))
}

// Question 输出问题信息
func Question(text string) {
	fmt.Printf("%s%s %s%s\n", ColorCyan, SymbolQuestion, ColorReset, text)
}

// ========== 对话相关 ==========

// Greet 输出问候语（带换行）
func Greet(text string) {
	fmt.Printf("%s%s%s %s %s\n\n", ColorBlue, Bold, SymbolRobot, text, ColorReset)
}

// Thinking 输出思考中
func Thinking(text string) {
	fmt.Printf("%s%s %s%s", ColorYellow, SymbolThinking, ColorReset, text)
}

// ThinkingEnd 输出思考结束
func ThinkingEnd() {
	fmt.Printf("\n%s%s %s\n", ColorGreen, SymbolThinking, ColorReset)
}

// UserInput 输出用户输入提示（带颜色和换行）
func UserInput(text string) {
	fmt.Printf("\n%s%s%s %s%s\n", ColorGreen, Bold, SymbolUser, ColorReset, text)
}

// PromptInput 通用输入提示符（简洁版本）
func PromptInput() {
	fmt.Printf("\n%s%s%s ", ColorGreen, SymbolUser, ColorReset)
}

// ========== 工具调用 ==========

// ToolCall 输出工具调用信息
func ToolCall(name string) {
	fmt.Printf("%s%s %s调用工具: %s%s\n", ColorMagenta, SymbolArrow, ColorCyan, name, ColorReset)
}

// ToolCallArgs 输出工具参数
func ToolCallArgs(args string) {
	fmt.Printf("%s%s %s参数: %s%s\n", ColorMagenta, strings.Repeat(" ", 2), ColorGray, args, ColorReset)
}

// ========== 流式输出 ==========

// StreamStart 流式输出开始
func StreamStart() {
	fmt.Printf("%s%s %s", ColorCyan, SymbolLoading, ColorReset)
}

// StreamEnd 流式输出结束
func StreamEnd() {
	fmt.Println()
}

// ========== 内容输出（不同类型不同颜色）==========
var (
	ContentColor   = ColorWhite   // 正文内容 - 白色
	ReasoningColor = ColorYellow  // 思考推理 - 黄色
	ToolColor      = ColorMagenta // 工具调用 - 洋红色
	CodeColor      = ColorCyan    // 代码块 - 青色
	URLColor       = ColorBlue    // 链接 - 蓝色
)

// Content 输出正文内容（白色）
func Content(text string) {
	fmt.Printf("%s%s%s", ContentColor, text, ColorReset)
}

// Reasoning 输出思考推理（黄色）
func Reasoning(text string) {
	fmt.Printf("%s%s%s", ReasoningColor, text, ColorReset)
}

// ToolCallOutput 输出工具调用（洋红色）
func ToolCallOutput(name string) {
	fmt.Printf("%s%s %s[工具] %s%s\n", ColorMagenta, SymbolArrow, ColorCyan, name, ColorReset)
}

// ToolCallArgsOutput 输出工具调用参数
func ToolCallArgsOutput(args string) {
	// 截断过长的参数显示
	displayArgs := args
	if len(displayArgs) > 200 {
		displayArgs = displayArgs[:200] + "..."
	}
	fmt.Printf("%s    └── 参数: %s%s%s\n", ColorMagenta, ColorGray, displayArgs, ColorReset)
}

// ToolResult 输出工具执行结果
func ToolResult(text string) {
	// 截断过长的结果显示
	displayText := text
	if len(displayText) > 300 {
		displayText = displayText[:300] + "..."
	}
	fmt.Printf("%s    └── 结果: %s%s%s\n", ColorGreen, ColorWhite, displayText, ColorReset)
}

// MarkdownHeading 输出 Markdown 标题
func MarkdownHeading(level int, text string) {
	prefix := strings.Repeat("#", level)
	color := []string{ColorBlue, ColorGreen, ColorCyan, ColorYellow}[level-1]
	fmt.Printf("%s%s %s%s%s\n", color, prefix, ColorReset, text, ColorReset)
}

// ========== 程序流程 ==========

// Exit 退出信息
func Exit(text string) {
	fmt.Printf("%s%s %s %s\n", ColorMagenta, SymbolExit, text, ColorReset)
}

// Loading 加载信息
func Loading(text string) {
	fmt.Printf("%s%s %s%s\n", ColorCyan, SymbolLoading, ColorReset, text)
}

// LoadingF 格式化加载信息
func LoadingF(format string, args ...interface{}) {
	fmt.Printf("%s%s %s%s\n", ColorCyan, SymbolLoading, ColorReset, fmt.Sprintf(format, args...))
}

// Step 完成步骤
func Step(step int, total int, text string) {
	fmt.Printf("%s[%d/%d]%s %s%s\n", ColorCyan, step, total, ColorReset, text, ColorReset)
}

// ========== 表格与列表 ==========

// ListItem 输出列表项
func ListItem(text string) {
	fmt.Printf("%s%s%s %s\n", ColorWhite, SymbolBullet, ColorReset, text)
}

// KeyValue 输出键值对
func KeyValue(key string, value string) {
	fmt.Printf("%s%s:%s %s\n", ColorCyan, key, ColorReset, value)
}

// KeyValueF 输出格式化键值对
func KeyValueF(key string, format string, args ...interface{}) {
	value := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s:%s %s\n", ColorCyan, key, ColorReset, value)
}

// ========== 进度与状态 ==========

// Progress 输出进度条
func Progress(current int, total int, barWidth int) {
	percent := float64(current) / float64(total) * 100
	filled := int(float64(barWidth) * float64(current) / float64(total))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Printf("\r%s[%s] %.1f%%", ColorCyan, bar, percent)
	if current == total {
		fmt.Println(ColorReset)
	}
}

// Status 输出状态
func Status(label string, status string, isOK bool) {
	if isOK {
		fmt.Printf("%s%s: %s%s %s%s\n", ColorCyan, label, ColorGreen, status, SymbolSuccess, ColorReset)
	} else {
		fmt.Printf("%s%s: %s%s %s%s\n", ColorCyan, label, ColorRed, status, SymbolError, ColorReset)
	}
}

// ========== 调试信息 ==========

// Debug 输出调试信息（仅调试模式使用）
func Debug(text string) {
	fmt.Printf("%s[DEBUG]%s %s%s\n", ColorGray, ColorReset, text, ColorReset)
}

// DebugF 格式化调试信息
func DebugF(format string, args ...interface{}) {
	fmt.Printf("%s[DEBUG]%s %s%s\n", ColorGray, ColorReset, fmt.Sprintf(format, args...), ColorReset)
}

// ========== 等待用户输入 ==========

// WaitForEnter 等待用户按回车
func WaitForEnter(prompt string) {
	fmt.Printf("%s%s %s", ColorYellow, SymbolArrow, ColorReset)
	fmt.Print(" ")
	fmt.Print(prompt)
	fmt.Scanln()
}

// WaitForEnterDefault 默认等待提示
func WaitForEnterDefault() {
	fmt.Printf("%s%s 按回车键继续...%s", ColorYellow, SymbolArrow, ColorReset)
	fmt.Scanln()
}

// ========== 彩色文本 ==========

// ColoredText 输出彩色文本
func ColoredText(color string, text string) {
	fmt.Printf("%s%s%s", color, text, ColorReset)
}

// BoldText 输出粗体文本
func BoldText(text string) {
	fmt.Printf("%s%s%s", Bold, text, ColorReset)
}

// ========== 格式化输出 ==========

// Println 普通换行输出
func Println(text string) {
	fmt.Println(text)
}

// Print 普通输出
func Print(text string) {
	fmt.Print(text)
}

// Printf 格式化输出
func Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Printfln 格式化输出并换行
func Printfln(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	fmt.Println()
}

// NewLine 输出空行
func NewLine() {
	fmt.Println()
}

// ========== 便捷函数 ==========

// Prompt 通用提示符
func Prompt() {
	fmt.Print(ColorCyan + SymbolArrow + ColorReset + " ")
}

// ErrorWithExit 错误并退出
func ErrorWithExit(text string) {
	Error(text)
	WaitForEnterDefault()
	os.Exit(1)
}

// ErrorFWithExit 格式化错误并退出
func ErrorFWithExit(format string, args ...interface{}) {
	ErrorF(format, args...)
	WaitForEnterDefault()
	os.Exit(1)
}

// ========== TUI 美化输出 (返回 tview 颜色标签字符串) ==========
//
// 设计规范:
//   颜色语义: green=成功/正向  red=错误  yellow=警告/推理  cyan=用户/信息  magenta=工具
//   符号规范: ✓成功 ✗错误 ⚠警告 ▶用户 ⚙工具 ◈状态
//   布局规范: 状态消息前空行分隔，推理/工具用边框包裹

const thinLine = "────────────────────────────────────────"

// ── 状态消息 ─────────────────────────────────

// TError TUI 错误信息
func TError(text string) string {
	return fmt.Sprintf("\n[red::b]✗ [-:-:-]%s\n", text)
}

// TErrorF TUI 格式化错误信息
func TErrorF(format string, args ...interface{}) string {
	return TError(fmt.Sprintf(format, args...))
}

// TSuccess TUI 成功信息
func TSuccess(text string) string {
	return fmt.Sprintf("\n[green::b]✓ [-:-:-]%s\n", text)
}

// TSuccessF TUI 格式化成功信息
func TSuccessF(format string, args ...interface{}) string {
	return TSuccess(fmt.Sprintf(format, args...))
}

// TWarning TUI 警告信息
func TWarning(text string) string {
	return fmt.Sprintf("\n[yellow::b]⚠ [-:-:-]%s\n", text)
}

// TWarningF TUI 格式化警告信息
func TWarningF(format string, args ...interface{}) string {
	return TWarning(fmt.Sprintf(format, args...))
}

// ── 生命周期 ─────────────────────────────────

// TWelcome TUI 欢迎/提示信息
func TWelcome(text string) string {
	return fmt.Sprintf("\n[green::b]◈ [-:-:-]%s\n", text)
}

// TReady TUI 启动完成信息
func TReady(name string) string {
	return fmt.Sprintf("\n[green::b]✓ [-:-:-]%s 就绪\n", name)
}

// TExit TUI 退出/结束信息
func TExit(text string) string {
	return fmt.Sprintf("\n[green::b]✓ [-:-:-]%s\n", text)
}

// TNewConversation TUI 新对话提示
func TNewConversation() string {
	return "\n[cyan::b]◈ [-:-:-]新对话已开始\n"
}

// TInterrupted TUI 中断提示
func TInterrupted() string {
	return "\n[yellow::b]⚠ [-:-:-]输入已打断\n"
}

// TCancelled TUI 取消提示
func TCancelled() string {
	return "\n[yellow::b]⚠ [-:-:-]会话已取消\n"
}

// ── 对话内容 ─────────────────────────────────

// TUserInput TUI 用户输入回显
func TUserInput(text string) string {
	return fmt.Sprintf("\n[white:%s:b]▶ %s[-:-:-]\n", TBgDarkGray, text)
}

// ── 推理区块 ─────────────────────────────────

// TReasoningStart TUI 推理开始
func TReasoningStart() string {
	return "\n[yellow::b]»[-:-:-]\n"
}

// TReasoningEnd TUI 推理结束
func TReasoningEnd() string {
	return "\n[yellow::b]«[-:-:-]\n"
}

// TReasoningContent 推理正文（暗黄色）
func TReasoningContent(text string) string {
	return fmt.Sprintf("[yellow::d]%s[-:-:-]", text)
}

// ── 正文区块 ─────────────────────────────────
func TContentNoneStreamTag(text string) string {
	return fmt.Sprintf("● %s", text)
}

// ── 工具区块 ─────────────────────────────────

// TToolCall TUI 工具调用（单行：工具名）
func TToolCall(name string) string {
	return fmt.Sprintf("\n[#C9966A]⮡ %s[-]", name)
}

// TToolArgs TUI 工具参数（跟在工具名后）
func TToolArgs(args string) string {
	displayArgs := args
	if len(displayArgs) > 200 {
		displayArgs = displayArgs[:200] + "..."
	}
	return fmt.Sprintf(" [#8B7355]%s[-]", displayArgs)
}

// TToolResult TUI 工具结果
func TToolResult(text string) string {
	displayText := text
	if len(displayText) > 300 {
		displayText = displayText[:300] + "..."
	}
	return fmt.Sprintf("\n[#8B7355]  %s[-]", displayText)
}

// TToolCompact 紧凑单行工具渲染：绿点 + 橙色工具名 + 灰色参数/结果概要
// 格式: ● name  args → resultSummary
func TToolCompact(name string, args []byte, result string) string {
	// ── 参数压缩：去换行、合并空格、截断到 60 字符 ──
	var compactArgs string
	if len(args) > 0 {
		s := strings.ReplaceAll(string(args), "\n", " ")
		s = strings.ReplaceAll(s, "  ", " ")
		s = strings.TrimSpace(s)
		if len(s) > 60 {
			s = s[:60] + "..."
		}
		if s != "" {
			compactArgs = " " + s
		}
	}

	// ── 结果概要：短文本直接显示，长文本只显示统计 ──
	var resultSummary string
	if result == "" {
		resultSummary = "∅"
	} else {
		lines := strings.Count(result, "\n") + 1
		chars := len(result)
		if chars <= 60 && lines <= 1 {
			resultSummary = strings.TrimSpace(result)
		} else if lines > 1 {
			resultSummary = fmt.Sprintf("%d lines, %s", lines, formatSize(chars))
		} else {
			resultSummary = formatSize(chars)
		}
	}

	return fmt.Sprintf(
		"\n[-:-:-]  [green]●[-] [%s]%s[-] [gray::d]%s → %s[-]",
		TColorClaudeCodeOrange, name, compactArgs, resultSummary,
	)
}

// formatSize 将字节数格式化为人类可读的大小
func formatSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

// ── 通用 ─────────────────────────────────────

// TDivider TUI 分隔线
func TDivider() string {
	return fmt.Sprintf("[gray::d]%s[-:-:-]\n", thinLine+"───────────")
}

// TColoredText TUI 彩色文本
func TColoredText(color string, text string) string {
	return fmt.Sprintf("[%s]%s[-:-:-]", color, text)
}

// ── 背景色工具 ────────────────────────────────

// TBg 通用背景色包装：前景白色，指定底色，文字粗体
//   - bgColor: 背景色 hex，如 TBgSilver
//   - text: 内容（需自行转义 tview 特殊字符）
func TBg(bgColor string, text string) string {
	return fmt.Sprintf("[white:%s:b]%s[-:-:-]", bgColor, text)
}

// TBgDim 通用背景色包装：前景灰色(dim)，指定底色
//
//	用于次要信息块，视觉权重更低
func TBgDim(bgColor string, text string) string {
	return fmt.Sprintf("[gray:%s]%s[-:-:-]", bgColor, text)
}
