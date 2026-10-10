package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// 帮助浮层尺寸。盒子样式（overlayCentered 的 Border+Padding(1,2)）决定
// 「边框+内边距」占多少：lipgloss v2 的 Width/Height 连边框一起算，
// 文本可用宽/高 = 总宽/高 - 样式的水平/垂直 frame size。全部从样式推导，
// 改 padding 或边框时这里自动跟上，不再手写魔法数字。
const (
	skillBoxWidth  = 100 //盒子总宽（含边框、padding）
	skillBoxHeight = 20  //盒子总高（含边框、padding）

	skillMinTextWidth = 10 //终端极窄时的兜底文本宽，防止负宽 panic
	titleHeight       = 2
	skillsTitle       = "Skills"
)

// skillBoxStyle 帮助浮层盒样式，唯一定义处。skills 和 overlayCentered 共用，
// 保证内容排版与盒子 frame 尺寸永远一致。
var skillBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(1, 2)

// skillTextSize 计算给定盒子总宽高下，文本真正可用的宽高。
func skillTextSize(boxW, boxH int) (textW, textH int) {
	st := skillBoxStyle.Width(boxW).Height(boxH)
	return boxW - st.GetHorizontalFrameSize(), boxH - st.GetVerticalFrameSize()
}

// helps 帮助浮层（demo 同款：居中圆角盒、上下选择、enter 插入指令）。
// 默认项自持；技能项由调用方传入（每次打开时从引擎拉取——loadSkills 在
// init/refresh 序列里随时可能重建列表，拉取式天然拿到最新版）。
type skills struct {
	items   [][2]string
	index   int
	visible bool
	vp      viewport.Model
}

func (s *skills) View(termWidth, termHeight int) string {
	//终端过小时盒子整体夹到屏幕内，再从盒子尺寸反推文本可用宽高
	boxW := min(skillBoxWidth, termWidth)
	boxH := min(skillBoxHeight, termHeight)
	textW, textH := skillTextSize(boxW, boxH)
	textW = max(textW, skillMinTextWidth)

	//高度动态：跟内容走（标题+空行 2 + 条目数），上限为盒子文本区高度
	contentH := len(s.items) + titleHeight
	vpH := min(contentH, textH)

	//viewport 高度必须 ≤ 盒子文本区，否则 View() 渲染的行数会撑破盒子
	//（lipgloss 的 Height 是最小高度，不截断）
	s.vp.SetWidth(textW)
	s.vp.SetHeight(vpH)
	s.vp.SetContent(skillsTitle + strings.Repeat("\n", titleHeight) + s.renderItems()) //标题也进 viewport，一起滚动
	//选中项跟随滚动：掉出视口就平移窗口，选中行贴顶（官方 EnsureVisible 语义）
	s.vp.EnsureVisible(s.index+titleHeight, 0, 0)
	return s.vp.View()
}

// renderItems 把条目渲染成左对齐两列表格：描述固定从「最长指令名 + 2」
// 列开始，全表统一，呈现整齐的两列。描述超宽不截断，viewport SoftWrap
// 默认 false 按宽度横向裁切，一行永远不会变两行。选中行加 "▶ " 前缀并高亮。
func (s *skills) renderItems() string {
	//描述列起点 = 最长指令名 + 2（对齐留白）；指令超长者退化为紧跟 +1 空格
	descCol := 0
	for _, v := range s.items {
		descCol = max(descCol, lipgloss.Width(v[0])+2)
	}

	var b strings.Builder
	for i, v := range s.items {
		prefix := "  "    //未选中的，前面加个空占位符，防止排版错乱
		if i == s.index { //如果index指向了这个索引值，说明是被用户选中了，加一个箭头标识
			prefix = "▶ "
		}
		gap := max(1, descCol-lipgloss.Width(v[0]))
		line := v[0] + strings.Repeat(" ", gap) + v[1]
		if i == s.index {
			line = prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(line) //换行不要进render，render后再加
		} else {
			line = prefix + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (s *skills) isVisible() bool {
	return s.visible
}

func (s *skills) toggleVisibility() {
	s.visible = !s.visible
}

func (s *skills) moveUp() {
	s.index = (s.index - 1 + len(s.items)) % len(s.items)
}

func (s *skills) moveDown() {
	s.index = (s.index + 1) % len(s.items)
}

// refreshItems 重建帮助条目：默认项 + 引擎拉取的技能项（每次打开时调用，
// 技能项由 engine.go 的 fetchHelpItems 解析好传入）。
func (s *skills) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case []wireSkillItem:
		s.items = [][2]string{}
		for _, it := range m {
			s.items = append(s.items, [2]string{it.Cmd, it.Desc})
		}
		s.index = 0
		return nil
	}
	return nil
}

func NewHelps() *skills {

	return &skills{
		vp: NewPrettyViewport(0, 1),
	}
}
