package tui

// /resume 弹框：会话恢复候选列表（斜杠命令浮层同款）。数据由 openResume
// 同步拉取，选择经 checkoutSession 提交；按键语义在 handler.go，
// 本文件只管渲染与选中态。

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

const (
	resumeBoxWidth  = 90 //盒子总宽（含边框、padding）
	resumeBoxHeight = 18 //盒子总高（含边框、padding）

	resumeMinTextW = 20 //终端极窄时的兜底文本宽，防止负宽 panic
	resumeTitle    = "Resume Session  (↑/↓ select · enter restore · esc cancel)"

	currentSuffix = "  · current" //当前会话标记（缀在时间列后）
	timeColMinGap = 2             //标题列与时间列的最小间隔
)

// resumeList 会话恢复弹框（结构同 slashPopup：viewport + 选中索引）。
type resumeList struct {
	items   []wireResumeItem
	index   int
	visible bool
	vp      viewport.Model
}

func newResumeList() *resumeList { return &resumeList{vp: NewPrettyViewport(0, 1)} }

func (r *resumeList) isVisible() bool { return r.visible }

// setItems 重建候选，选中归零。
func (r *resumeList) setItems(items []wireResumeItem) {
	r.items = append([]wireResumeItem(nil), items...)
	r.index = 0
}

// moveUp/moveDown 空列表直接返回：取模除数是 len(r.items)，不判会 panic。
func (r *resumeList) moveUp() {
	if len(r.items) == 0 {
		return
	}
	r.index = (r.index - 1 + len(r.items)) % len(r.items)
}

func (r *resumeList) moveDown() {
	if len(r.items) == 0 {
		return
	}
	r.index = (r.index + 1) % len(r.items)
}

// selectedID 当前选中会话；空列表或索引越界时 ok=false。
func (r *resumeList) selectedID() (string, bool) {
	if r.index < 0 || r.index >= len(r.items) {
		return "", false
	}
	return r.items[r.index].ID, true
}

// View 渲染弹框内容（标题进 viewport 一起滚动，选中行跟随滚动）。
func (r *resumeList) View(termWidth, termHeight int) string {
	boxW := min(resumeBoxWidth, termWidth)
	boxH := min(resumeBoxHeight, termHeight)
	textW, textH := overlayTextSize(boxW, boxH)
	textW = max(textW, resumeMinTextW)

	//高度动态：跟内容走（标题+空行 2 + 条目数），上限为盒子文本区高度
	contentH := len(r.items) + titleHeight
	vpH := min(contentH, textH)
	r.vp.SetWidth(textW)
	r.vp.SetHeight(vpH)
	r.vp.SetContent(resumeTitle + strings.Repeat("\n", titleHeight) + r.renderItems(textW))
	//选中项跟随滚动：掉出视口就平移窗口，选中行贴顶
	r.vp.EnsureVisible(r.index+titleHeight, 0, 0)
	return r.vp.View()
}

// renderItems 标题列（按显示宽截断）+ 时间列；当前会话缀 current 标记。
// 选中行加 "▶ " 前缀并高亮，未选中的前缀占位对齐。
func (r *resumeList) renderItems(textW int) string {
	var b strings.Builder
	for i, it := range r.items {
		meta := it.UpdatedAt
		if it.Current {
			meta += currentSuffix
		}
		prefix := "  "
		if i == r.index {
			prefix = "▶ "
		}
		//标题可用宽 = 文本宽 - 前缀 - 时间块 - 间隔
		maxTitle := textW - lipgloss.Width(prefix) - lipgloss.Width(meta) - timeColMinGap
		maxTitle = max(maxTitle, 10) //极窄兜底：宁溢出交 viewport 裁切，不出负宽
		title, usedW := truncateByWidth(it.Title, maxTitle)
		gap := max(1, maxTitle-usedW)
		line := title + strings.Repeat(" ", gap) + meta
		if i == r.index {
			//换行不要进 render，render 后再拼（与 slashPopup.renderItems 一致）
			line = prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render(line)
		} else {
			line = prefix + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// truncateByWidth 按显示宽截断（中文按 2 列计），截断时补省略号；
// 返回文本与实际显示宽（含省略号）。逐 rune 累计而非按 rune 数截断，
// 否则中英混排行宽参差，时间列对不齐。
func truncateByWidth(s string, maxW int) (string, int) {
	w := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > maxW-1 { //留 1 列给省略号
			return s[:i] + "…", w + 1
		}
		w += rw
	}
	return s, w
}
