package tui

// 斜杠命令浮层：输入框里打出 "/" 后弹出的候选清单。条目由
// handler.refilterSlashItems 按前缀过滤后传入，显隐由输入框内容派生；
// 盒样式、标题行高与文本可用宽高推导是浮层通用件，定义在 component.go
// （resume 弹框共用）。

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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
	boxW := min(slashBoxWidth, termWidth)
	boxH := min(slashBoxHeight, termHeight)
	textW, textH := overlayTextSize(boxW, boxH)
	textW = max(textW, slashMinTextWidth)

	contentH := len(s.items) + titleHeight
	vpH := min(contentH, textH)

	//vp 高度必须 ≤ 盒子文本区：lipgloss 的 Height 是最小高度，不截断
	s.vp.SetWidth(textW)
	s.vp.SetHeight(vpH)
	s.vp.SetContent(slashTitle + strings.Repeat("\n", titleHeight) + s.renderItems())
	//选中项跟随滚动：掉出视口就平移窗口，选中行贴顶
	s.vp.EnsureVisible(s.index+titleHeight, 0, 0)
	return s.vp.View()
}

// renderItems 左对齐两列表格：描述固定从「最长指令名 + 2」列开始。描述
// 超宽不截断，viewport SoftWrap 默认 false 按宽度横向裁切，一行永远不变两行。
func (s *slashPopup) renderItems() string {
	//描述列起点 = 最长指令名 + 2；指令超长者退化为紧跟 +1 空格
	descCol := 0
	for _, v := range s.items {
		descCol = max(descCol, lipgloss.Width(v.cmd)+2)
	}

	var b strings.Builder
	for i, v := range s.items {
		prefix := "  " //未选中行用空占位符对齐
		if i == s.index {
			prefix = "▶ "
		}
		gap := max(1, descCol-lipgloss.Width(v.cmd))
		line := v.cmd + strings.Repeat(" ", gap) + v.desc
		if i == s.index {
			//换行不要进 render，render 后再拼
			line = prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(line)
		} else {
			line = prefix + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (s *slashPopup) isVisible() bool { return s.visible }

// slashPopupKeyHandler 斜杠浮层的按键语义：esc/ctrl+c 关浮层，up/down 选条目，
// enter 补全指令，其余键继续进输入框（打字要重新过滤候选）。
func (t *TUI) slashPopupKeyHandler(m tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	input := m.String()

	switch input {
	case "esc", "ctrl+c":
		t.slashPopup.switchVisibility(false)
		return t, nil
	case "down":
		t.slashPopup.moveDown()
		return t, nil
	case "up":
		t.slashPopup.moveUp()
		return t, nil
	case "enter":
		//清单为空时无指令可补全，浮层保持打开（esc 仍能关）
		sel, ok := t.slashPopup.selectedCmd()
		if !ok {
			return t, nil
		}
		//整段替换而非 InsertString：输入框里留着半截 "/re"，插入会拼成
		//"/re/resume "。补全后带空格，光标停在命令末尾等用户接参数。
		t.ta.SetValue(sel + " ")
		t.slashPopup.switchVisibility(false)
		t.recalcComponentSize()
		return t, nil
	default:
		var cmd tea.Cmd
		t.ta, cmd = t.ta.Update(m)
		t.refilterSlashItems() //浮层开着时继续打字也要重新过滤，否则清单永远停在第一次的结果
		t.recalcComponentSize()
		return t, cmd
	}
}

// switchVisibility 只有"设为某值"、没有"翻转"：可见性是输入框内容的派生结果。
func (s *slashPopup) switchVisibility(visible bool) { s.visible = visible }

// setItems 整体替换浮层条目并把选中项归零；每次按键都会被调用，
// 所以复用底层数组（append 的是值，与 slashRegistry 无别名）。
func (s *slashPopup) setItems(items []slashCmd) {
	s.items = s.items[:0]
	s.items = append(s.items, items...)
	s.index = 0
}

// moveUp/moveDown 清单为空时必须直接返回：取模除数是 len(s.items)，不判会 panic。
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
