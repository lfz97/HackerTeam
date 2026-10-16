package tui

// 斜杠命令浮层：输入框里打出 "/" 后弹出的候选清单（demo 同款：居中圆角盒、
// 上下选择、enter 补全）。条目由 handler.refilterSlashItems 每次按键按前缀从
// slashRegistry 过滤后传入；显隐同样是派生的——由输入框内容决定，无独立开关、
// 无打开快捷键。盒样式、标题行高与文本可用宽高推导都是浮层通用件，定义在
// component.go（overlayBoxStyle/titleHeight/overlayTextSize，resume 弹框共用）。

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// 浮层盒尺寸。lipgloss v2 的 Width/Height 连边框一起算，文本可用宽高由
// overlayTextSize 从盒样式反推，不写魔法数字。
const (
	slashBoxWidth  = 100 //盒子总宽（含边框、padding）
	slashBoxHeight = 20  //盒子总高（含边框、padding）

	slashMinTextWidth = 10 //终端极窄时的兜底文本宽，防止负宽 panic
	slashTitle        = "Commands  (↑/↓ select · enter complete · esc dismiss)"
)

// slashPopup 斜杠命令浮层（结构同 resumeList：viewport + 选中索引）。
type slashPopup struct {
	items   []slashCmd
	index   int
	visible bool
	vp      viewport.Model
}

func newSlashPopup() *slashPopup { return &slashPopup{vp: NewPrettyViewport(0, 1)} }

func (s *slashPopup) View(termWidth, termHeight int) string {
	//终端过小时盒子整体夹到屏幕内，再从盒子尺寸反推文本可用宽高
	boxW := min(slashBoxWidth, termWidth)
	boxH := min(slashBoxHeight, termHeight)
	textW, textH := overlayTextSize(boxW, boxH)
	textW = max(textW, slashMinTextWidth)

	//高度动态：跟内容走（标题+空行 2 + 条目数），上限为盒子文本区高度
	contentH := len(s.items) + titleHeight
	vpH := min(contentH, textH)

	//viewport 高度必须 ≤ 盒子文本区，否则 View() 渲染的行数会撑破盒子
	//（lipgloss 的 Height 是最小高度，不截断）
	s.vp.SetWidth(textW)
	s.vp.SetHeight(vpH)
	s.vp.SetContent(slashTitle + strings.Repeat("\n", titleHeight) + s.renderItems()) //标题也进 viewport，一起滚动
	//选中项跟随滚动：掉出视口就平移窗口，选中行贴顶（官方 EnsureVisible 语义）
	s.vp.EnsureVisible(s.index+titleHeight, 0, 0)
	return s.vp.View()
}

// renderItems 把条目渲染成左对齐两列表格：描述固定从「最长指令名 + 2」
// 列开始，全表统一，呈现整齐的两列。描述超宽不截断，viewport SoftWrap
// 默认 false 按宽度横向裁切，一行永远不会变两行。选中行加 "▶ " 前缀并高亮。
func (s *slashPopup) renderItems() string {
	//描述列起点 = 最长指令名 + 2（对齐留白）；指令超长者退化为紧跟 +1 空格
	descCol := 0
	for _, v := range s.items {
		descCol = max(descCol, lipgloss.Width(v.cmd)+2)
	}

	var b strings.Builder
	for i, v := range s.items {
		prefix := "  "    //未选中的，前面加个空占位符，防止排版错乱
		if i == s.index { //如果index指向了这个索引值，说明是被用户选中了，加一个箭头标识
			prefix = "▶ "
		}
		gap := max(1, descCol-lipgloss.Width(v.cmd))
		line := v.cmd + strings.Repeat(" ", gap) + v.desc
		if i == s.index {
			line = prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(line) //换行不要进render，render后再加
		} else {
			line = prefix + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (s *slashPopup) isVisible() bool { return s.visible }

// switchVisibility 只有"设为某值"、没有"翻转"：可见性是输入框内容的派生结果，
// 翻转语义会让调用点隐式依赖当前状态。
func (s *slashPopup) switchVisibility(visible bool) { s.visible = visible }

// setItems 整体替换浮层条目并把选中项归零（新清单从首项开始选中）。
// 每次按键都会被调用，所以复用底层数组（append 的是值，与 slashRegistry 无别名）。
func (s *slashPopup) setItems(items []slashCmd) {
	s.items = s.items[:0]
	s.items = append(s.items, items...)
	s.index = 0
}

// moveUp/moveDown 清单为空时必须直接返回：取模的除数是 len(s.items)，不判会
// panic。（refilterSlashItems 不会用空清单开框，但 setItems 是包内公开入口。）
func (s *slashPopup) moveUp() {
	if len(s.items) == 0 {
		return
	}
	s.index = (s.index - 1 + len(s.items)) % len(s.items)
}

func (s *slashPopup) moveDown() {
	if len(s.items) == 0 {
		return
	}
	s.index = (s.index + 1) % len(s.items)
}

// selectedCmd 返回当前选中条目的指令；清单为空或索引越界时 ok=false。
func (s *slashPopup) selectedCmd() (string, bool) {
	if s.index < 0 || s.index >= len(s.items) {
		return "", false
	}
	return s.items[s.index].cmd, true
}
