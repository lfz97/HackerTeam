package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// 启动横幅：圆角盒双栏布局（参考 Claude Code 欢迎区）——
// 左栏 Welcome + H 字标 + 模型/目录，右栏 Tips + What's new。
// 拼版方法不读 model、宽度作参数传入，可以直接对 banner 单测。

const (
	// 渐变与品牌色：青 → 紫，与整体 UI 色系一致
	bannerGradientFrom = "#4FC3F7"
	bannerGradientTo   = "#A78BFA"

	bannerTitle     = "HackerTeam"
	bannerVersion   = "v3" //大版本号，发版时更新
	bannerLogoGap   = 3    // logo 与欢迎语的间隔列（仅窄终端降级版使用）
	bannerLeftW     = 34   // 左栏列宽（含内边距），信息行超出即截断
	bannerPadding   = 1    // 盒内左右内边距
	bannerRightMinW = 24   // 右栏压缩下限，低于它放弃双栏改走紧凑堆叠
)

// bannerLogo 7 行机器人头像。刻意只用块元素（█▄▀）：制表符（╭─╮ 等）是
// East Asian Ambiguous 字符，CJK 环境的终端会渲染成 2 列而代码按 1 列
// 计宽，必然错位；块元素无条件 1 列，任何终端/字体都不会歪。
var bannerLogo = []string{
	" ▄█▄           ▄█▄ ",
	"███████████████████",
	"█████  █████  █████",
	"▀█████████████████▀",
}

// banner 描述一条启动横幅：H 字标 + Engine 的信息行。
type banner struct {
	logo []string // 字标行
	info []string // Engine 拼好的 "label<pad>value" 行
}

// newBanner 构造横幅。
func newBanner(infoLines []string) *banner {
	return &banner{
		logo: bannerLogo,
		info: infoLines,
	}
}

// compose 按可用宽度拼出横幅文本（不含首尾换行）。终端放不下双栏盒
// 就降级为紧凑堆叠版。
func (b *banner) compose(width int) string {
	if box := b.composeBox(width); box != "" {
		return box
	}
	return b.composeCompact(width)
}

// composeBox 双栏圆角盒，盒宽=终端宽（左右边距对称）。宽度不足（返回
// 空串）时由 compose 降级。
func (b *banner) composeBox(width int) string {
	// 信息按标签索引
	m := make(map[string]string, len(b.info))
	for _, line := range b.info {
		if label, value := infoParts(line); label != "" {
			m[label] = value
		}
	}

	inner := width - 2                                  //去掉左右边框后的内容宽
	rightW := inner - bannerLeftW - 2 - bannerPadding*2 //右栏列宽
	if rightW < bannerRightMinW {
		return "" //放不下右栏，降级
	}

	center := lipgloss.NewStyle().Width(bannerLeftW).Align(lipgloss.Center)

	// 左右两栏内容块在盒内垂直居中：上下各留 1 行呼吸，块高差由居中摊平。
	// 左栏 = 欢迎语 + 空行 + 猫头像；右栏 = system 标题 + 全部启动信息
	// （快捷键说明底部提示条已常驻，不再重复）。
	logo := b.coloredLogo()
	leftH := 2 + len(logo)
	stats := make([]string, 0, len(b.info))
	for _, line := range b.info {
		if label, _ := infoParts(line); label != "" {
			stats = append(stats, subText(line))
		}
	}
	rightH := 1 + len(stats)
	contentRows := max(leftH, rightH)
	rows := contentRows + 2
	leftTop := 1 + (contentRows-leftH)/2
	rightTop := 1 + (contentRows-rightH)/2

	left := make([]string, rows)
	left[leftTop] = center.Render("Welcome back!")
	for i, row := range logo {
		left[leftTop+2+i] = center.Render(row)
	}
	right := make([]string, rows)
	right[rightTop] = brandText("system")
	for i, s := range stats {
		right[rightTop+1+i] = s
	}

	var s strings.Builder
	//顶边框嵌品牌色标题（同 codebuddy：cliName 放 border text，offset 3）
	title := "─ " + brandText(bannerTitle+" "+bannerVersion) + " "
	s.WriteString("╭" + title + strings.Repeat("─", inner-ansi.StringWidth(title)) + "╮\n")
	for i := 0; i < rows; i++ {
		s.WriteString("│ " + fitWidth(left[i], bannerLeftW) + "  " + fitWidth(right[i], rightW) + " │\n")
	}
	s.WriteString("╰" + strings.Repeat("─", inner) + "╯")
	return s.String()
}

// composeCompact 窄终端降级版：logo 左、信息行右，无边框。
func (b *banner) composeCompact(width int) string {
	clamp := func(s string) string {
		if width <= 0 {
			return s
		}
		return clampWidth(s, width)
	}

	headlines := b.headlines()
	logo := b.coloredLogo()

	var s strings.Builder
	for i, row := range logo {
		line := row + strings.Repeat(" ", bannerLogoGap)
		if i < len(headlines) && headlines[i] != "" {
			line += headlines[i]
		}
		s.WriteString(clamp(line))
		if i < len(logo)-1 {
			s.WriteString("\n")
		}
	}
	return s.String()
}

// headlines 信息行（与 logo 行数对齐）：产品名 / 模型 · 工具统计 / 工作目录。
func (b *banner) headlines() []string {
	m := make(map[string]string, len(b.info))
	for _, line := range b.info {
		if label, value := infoParts(line); label != "" {
			m[label] = value
		}
	}
	line2 := m["model"]
	if v := m["tools"]; v != "" {
		//value 以数字开头时补回标签词（tools），否则摘要丢失主语
		if v[0] >= '0' && v[0] <= '9' {
			v = "tools " + v
		}
		if line2 != "" {
			line2 += " · "
		}
		line2 += v
	}
	return []string{bannerTitle, line2, m["cwd"]}
}

// coloredLogo 逐行纵向渐变（3 行：青 → 紫）。
func (b *banner) coloredLogo() []string {
	out := make([]string, len(b.logo))
	for i, row := range b.logo {
		t := 0.0
		if len(b.logo) > 1 {
			t = float64(i) / float64(len(b.logo)-1)
		}
		out[i] = colorText(lipgloss.Color(hexLerp(bannerGradientFrom, bannerGradientTo, t)), row)
	}
	return out
}

// brandText 品牌色文本（右栏标题）。
func brandText(text string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(bannerGradientFrom)).
		Render(text)
}

// infoParts 拆 "label<pad到13列>value" 行：首个连续 2 空格之前是标签
// （如 model/endpoint），之后是值。
func infoParts(line string) (label, value string) {
	i := strings.Index(line, "  ")
	if i < 0 {
		return "", strings.TrimSpace(line)
	}
	return strings.TrimSpace(line[:i]), strings.TrimLeft(line[i:], " ")
}

// ---------- 渐变辅助 ----------

// hexLerp 两个 hex 颜色按 t∈[0,1] 线性插值，返回 "#rrggbb"。
func hexLerp(from, to string, t float64) string {
	parse := func(c string) (r, g, b float64) {
		c = strings.TrimPrefix(c, "#")
		if len(c) != 6 {
			return 0, 0, 0
		}
		rr, _ := strconv.ParseUint(c[0:2], 16, 8)
		gg, _ := strconv.ParseUint(c[2:4], 16, 8)
		bb, _ := strconv.ParseUint(c[4:6], 16, 8)
		return float64(rr), float64(gg), float64(bb)
	}
	r1, g1, b1 := parse(from)
	r2, g2, b2 := parse(to)
	r := uint8(r1 + (r2-r1)*t)
	g := uint8(g1 + (g2-g1)*t)
	b := uint8(b1 + (b2-b1)*t)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// gradientText 逐字符横向渐变：第 x 个字符取 from→to 插值色（按显示列计 x）。
// 字形全部为单列宽块元素，无需处理宽字符。
func gradientText(s, from, to string) string {
	w := ansi.StringWidth(s)
	if w <= 1 {
		return s
	}
	var b strings.Builder
	x := 0
	for _, r := range s {
		b.WriteString(colorText(lipgloss.Color(hexLerp(from, to, float64(x)/float64(w-1))), string(r)))
		x += lipgloss.Width(string(r))
	}
	return b.String()
}

// ---------- 字符串拼版辅助（通用，不绑定 banner） ----------
//
// ANSI 输出下没有 tview 标签的转义问题（方括号就是字面量），宽度度量
// 直接用 ansi.StringWidth（显示宽度、宽字符与 ANSI 序列都正确处理）。

// subText 次文本色（信息行）。
func subText(text string) string {
	return lipgloss.NewStyle().Foreground(cSub).Render(text)
}

// fitWidth 截断 + 补齐到恰好 w 列。
func fitWidth(s string, w int) string {
	s = clampWidth(s, w)
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// clampWidth 截断到不超过 w 列（超出时补省略号），w <= 0 返回空串。
func clampWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w-1, "…")
	}
	// ansi.Truncate 保证宽度不超 w，这里只做防御性兜底
	for ansi.StringWidth(s) > w {
		runes := []rune(s)
		if len(runes) == 0 {
			break
		}
		s = string(runes[:len(runes)-1])
	}
	return s
}

// composeBanner 组装启动横幅文本（pull 循环在 StartupInfo 就绪后的第一帧调用一次）。
// 横幅是"死文本"：写入后随对话滚动、resize 由 viewport 软换行兜底——
// 这是刻意的语义（启动横幅本来就是一次性历史记录）。
func composeBanner(infoLines []string, width int) string {
	b := newBanner(infoLines)
	//banner 与输入框之间只留 1 行空行（对齐 Claude Code 的紧凑观感）
	return "\n" + b.compose(width) + "\n\n"
}
