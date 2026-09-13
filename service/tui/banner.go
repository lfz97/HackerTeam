package tui

import (
	"strings"

	"HackerTeam/utils/pretty"
	"github.com/rivo/tview"
)

// 启动横幅：banner 结构体描述内容（logo / 信息 / 面板三段），方法负责拼版。
// 拼版方法不读 widget、宽度作参数传入，需要时可以直接对 banner 单测。

const (
	bannerLogoWidth = 12 // logo 点阵宽度，固定不可伸缩
	bannerGap       = 1  // 段之间的间隔列

	// bannerConfigMinWidth 信息列压缩下限，低于它就放弃三列改走堆叠
	bannerConfigMinWidth = 24
)

// bannerLogo 5 行 H 字标，每行恰好 bannerLogoWidth 个 rune（█ 为 1 cell）。
var bannerLogo = []string{
	"  ██    ██  ",
	" ██      ██ ",
	" ██████████ ",
	" ██      ██ ",
	"  ██    ██  ",
}

// bannerLogoColors 与 bannerLogo 逐行对应，两端取 pretty 调色板常量，中间线性插值。
var bannerLogoColors = []string{
	"#4FC3F7", // = pretty.TColorSkyBlue
	"#57C3F1",
	"#5FC3EB",
	"#67C3E5",
	"#6FC3DF", // = pretty.TuiStatusHint
}

// banner 描述一条启动横幅：左 logo、中信息、右面板。
// 信息行与面板行数相等（各 5 行），三块上下齐平，不需要垂直对齐逻辑。
type banner struct {
	logo      []string // 点阵行
	logoColor []string // 逐行渐变色
	info      []string // Engine 拼好的 "label value" 行
	panel     bannerPanel
}

// bannerPanel 右栏键位提示面板，方框宽度由最长行测出（naturalWidth），加长提示不用改数字。
type bannerPanel struct {
	title string
	lines []string
}

// defaultBannerPanel 默认面板。刻意保持 3 条：加边框正好 5 行，与 logo、信息列等高。
var defaultBannerPanel = bannerPanel{
	title: "getting started",
	lines: []string{
		"ctrl+k       slash commands",
		"esc          interrupt this run",
		"shift+enter  insert a newline",
	},
}

// newBanner 用默认 logo 渐变与面板构造横幅。
func newBanner(infoLines []string) *banner {
	return &banner{
		logo:      bannerLogo,
		logoColor: bannerLogoColors,
		info:      infoLines,
		panel:     defaultBannerPanel,
	}
}

// naturalWidth 面板自然宽度：最长行 + 1 前导空格 + 2 边框，不小于标题行所需。
func (p bannerPanel) naturalWidth() int {
	w := blockWidth(p.lines) + 3
	if min := tview.TaggedStringWidth("─ "+p.title+" ") + 3; w < min {
		w = min
	}
	return w
}

// render 画面板方框，每行恰好 panelW 列。
func (p bannerPanel) render(panelW int) []string {
	inner := panelW - 2

	title := "─ " + p.title + " "
	// 顶边框 = 1(╭) + title + dashes + 1(╮) = panelW，反推 dashes；clamp 仅防御
	dashes := inner - tview.TaggedStringWidth(title)
	if dashes < 0 {
		dashes = 0
	}

	lines := make([]string, 0, len(p.lines)+2)
	lines = append(lines, "╭"+title+strings.Repeat("─", dashes)+"╮")
	for _, s := range p.lines {
		lines = append(lines, "│"+fitWidth(" "+s, inner)+"│")
	}
	lines = append(lines, "╰"+strings.Repeat("─", inner)+"╯")
	return lines
}

// totalWidth 三列完整版的自然总宽，降级判断与列宽计算共用。
func (b *banner) totalWidth() int {
	return bannerLogoWidth + bannerGap + blockWidth(b.info) + bannerGap + b.panel.naturalWidth()
}

// configWidth 信息列自然宽度 = 总宽 - logo - 面板 - 两个间隔列。
func (b *banner) configWidth() int {
	return b.totalWidth() - b.panel.naturalWidth() - bannerLogoWidth - 2*bannerGap
}

// compose 按可用宽度拼出横幅文本（不含首尾换行）。
// 三段里只有信息列是弹性的（logo 是点阵艺术、面板是固定文案），降级顺序：
// 放得下 → 三列零截断；放不下 → 压缩信息列保三列；压到下限以下 → 纵向堆叠。
func (b *banner) compose(width int) string {
	panelW := b.panel.naturalWidth()
	if width >= b.totalWidth() {
		return b.composeWide(b.configWidth(), panelW)
	}
	if avail := width - bannerLogoWidth - 2*bannerGap - panelW; avail >= bannerConfigMinWidth {
		return b.composeWide(avail, panelW)
	}
	return b.composeStacked(width)
}

// composeWide 三列完整版：logo | 信息 | 面板，每行恰好三段之和列。
func (b *banner) composeWide(configW, panelW int) string {
	logo := b.coloredLogo()

	info := make([]string, 0, len(b.info))
	for _, line := range b.info {
		// 整行一个颜色：Engine 传来的是已拼好的 "label value"，拆开上色不值得
		info = append(info, pretty.TColoredText(pretty.TuiSubText, fitWidth(line, configW)))
	}

	panel := b.panel.render(panelW)

	rows := len(logo)
	if len(info) > rows {
		rows = len(info)
	}
	if len(panel) > rows {
		rows = len(panel)
	}

	var s strings.Builder
	for i := 0; i < rows; i++ {
		s.WriteString(lineAt(logo, i, bannerLogoWidth))
		s.WriteString(strings.Repeat(" ", bannerGap))
		s.WriteString(lineAt(info, i, configW))
		s.WriteString(strings.Repeat(" ", bannerGap))
		s.WriteString(lineAt(panel, i, panelW))
		if i < rows-1 {
			s.WriteString("\n")
		}
	}
	return s.String()
}

// composeStacked 窄终端降级版：纵向堆叠、无方框，只截断不补齐。
func (b *banner) composeStacked(width int) string {
	// width <= 0 时不截断，交给 AgentMessage 的 SetWrap(true) 折行
	clamp := func(s string) string {
		if width <= 0 {
			return tview.Escape(s)
		}
		return clampWidth(s, width)
	}

	var s strings.Builder
	for i, row := range b.logo {
		s.WriteString(pretty.TColoredText(b.logoColor[i], row))
		s.WriteString("\n")
	}
	s.WriteString("\n")
	for _, line := range b.info {
		s.WriteString(pretty.TColoredText(pretty.TuiSubText, clamp(line)))
		s.WriteString("\n")
	}
	s.WriteString("\n")
	for _, line := range b.panel.lines {
		s.WriteString(pretty.TColoredText(pretty.TuiSubText, clamp(line)))
		s.WriteString("\n")
	}
	return strings.TrimRight(s.String(), "\n")
}

// coloredLogo 逐行套渐变色。
func (b *banner) coloredLogo() []string {
	logo := make([]string, len(b.logo))
	for i, row := range b.logo {
		logo[i] = pretty.TColoredText(b.logoColor[i], row)
	}
	return logo
}

// ---------- 字符串拼版辅助（通用，不绑定 banner） ----------

// blockWidth 返回一组文本里最宽那行的显示宽度。必须量转义后的文本：
// 字面 "[CN]" 的 TaggedStringWidth 是 0（被当颜色标签吞掉），转义后才是 4。
func blockWidth(lines []string) int {
	w := 0
	for _, s := range lines {
		if n := tview.TaggedStringWidth(tview.Escape(s)); n > w {
			w = n
		}
	}
	return w
}

// lineAt 取第 i 行，越界返回 w 个空格。
func lineAt(lines []string, i, w int) string {
	if i < len(lines) {
		return lines[i]
	}
	return strings.Repeat(" ", w)
}

// fitWidth 转义 + 截断 + 补齐到恰好 w 列。
func fitWidth(s string, w int) string {
	s = clampWidth(s, w)
	if pad := w - tview.TaggedStringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// clampWidth 转义并截断到不超过 w 列。必须先 Escape 再度量（原因见 blockWidth）；
// 末尾按 rune 硬切兜底，保证宽度不变量无条件成立。
func clampWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = tview.Escape(s)
	if tview.TaggedStringWidth(s) > w {
		s = truncateToWidth(s, w-1) + "…"
	}
	for tview.TaggedStringWidth(s) > w {
		runes := []rune(s)
		if len(runes) == 0 {
			break
		}
		s = string(runes[:len(runes)-1])
	}
	return s
}

// truncateToWidth 从尾部逐 rune 回退到显示宽度不超过 w，入参必须已转义。
func truncateToWidth(s string, w int) string {
	runes := []rune(s)
	for len(runes) > 0 && tview.TaggedStringWidth(string(runes)) > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

// ---------- TUI 接线 ----------

// contentWidth 返回消息区当前内容宽度。GetInnerRect 无锁读布局字段，
// 必须在 QueueUpdate 内调；用 QueueUpdate 而非 QueueUpdateDraw——只读值，不触发重绘。
func (t *Tui) contentWidth() int {
	var w int
	t.app.QueueUpdate(func() {
		_, _, w, _ = t.appLayout.agentMessage.GetInnerRect()
	})
	return w
}

// ShowStartupBanner 把启动横幅写进消息区，只在 Startup 那一轮调用一次。
// 横幅是"死文本"：随对话滚动、resize 不重排，这是刻意的语义。
func (t *Tui) ShowStartupBanner(infoLines []string) {
	b := newBanner(infoLines)
	t.PrintToMsgView("\n"+b.compose(t.contentWidth())+"\n\n\n", false)
}
