package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// NewPrettyTextArea 美化过的输入框（demo 同款：圆角边框 + 紫色 62）。
func NewPrettyTextArea(width int, height int, charLimit int) (textarea.Model, tea.Cmd) {

	//初始化
	ta := textarea.New()
	ta.SetWidth(width)       // 设置宽度（列数）
	ta.SetHeight(height)     // 设置高度（行数）
	ta.CharLimit = charLimit // 最大容纳字符数，0 表示不限制
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true // 设置高度随内容自动增减
	ta.MinHeight = height   // 自动收缩高度下限
	ta.MaxHeight = height + 4

	// 裸 enter 是"提交"（在 keyMsgHandler 里拦截），换行改挂 shift+enter /
	// ctrl+j。textarea 默认把 enter 绑定 InsertNewline，必须显式改绑，
	// 否则拦截前被默认绑定吃掉的行为不可控。
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))

	//定义样式
	style := textarea.DefaultStyles(false)
	style.Focused.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).       //圆角框线
		BorderForeground(lipgloss.Color("62")). //框线颜色
		Padding(0, 1)                           //框线与文字边距（垂直、水平）
	style.Focused.CursorLine = lipgloss.NewStyle()

	//应用样式
	ta.SetStyles(style)

	//启用焦点，textarea只有启用焦点时才处理消息
	cmd := ta.Focus()
	return ta, cmd
}

// NewPrettyViewport 美化过的展示框（demo 同款）。
func NewPrettyViewport(width int, height int) viewport.Model {
	vp := viewport.New()
	vp.SetHeight(height)
	vp.SetWidth(width)
	return vp
}

// recalcComponentSize 重算所有组件尺寸（demo 范式 + 清单栏）。
func (t *TUI) recalcComponentSize() {
	// 窗口尺寸未就绪(启动早期,engine 的消息可能抢在第一条 WindowSizeMsg 之前到达)时,
	// t.width/t.height 还是 0,此时布局免谈,等 WindowSizeMsg 到达后再算
	if t.width <= 0 || t.height <= 0 {
		return
	}
	//计算组件宽度，需要减去两边padding的宽度，给padding留空间
	componentWidth := t.componentWidth()
	t.ta.SetWidth(componentWidth) // textarea在 v2 里 SetWidth 之后，组件会按新宽度把当前文本重新软换行一遍，数出总共占多少显示行，然后自动把高度设成这个行数。
	t.vp.SetWidth(componentWidth)
	t.bottom.UpdateGap(componentWidth - lipgloss.Width(t.bottom.spinnerView()) - lipgloss.Width(t.bottom.notice))

	bottomHeight := lipgloss.Height(t.bottom.View())
	todoHeight := lipgloss.Height(t.todoText)
	// ta.View() 含边框，实际占 ta.Height()+2 行
	viewportHeight := t.height - lipgloss.Height(t.ta.View()) - bottomHeight - todoHeight //viewport需要占满剩余高度
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	t.vp.SetHeight(viewportHeight)
}

// draw 组合组件并绘制（demo 范式：viewport + todo + textarea + bottom，
// 两侧 padding，帮助浮层 Compositor 叠加）。
func (t *TUI) draw() string {
	messageView := t.vp.View()
	textAreaView := t.ta.View()
	bottomView := t.bottom.View()
	leftPaddingView := lipgloss.NewStyle().Width(t.leftPadding).Height(t.height).Render("")
	rightPaddingView := lipgloss.NewStyle().Width(t.rightPadding).Height(t.height).Render("")

	//垂直方向组合消息栏、清单栏、文本栏、底部栏。清单为空时不占行。
	blocks := []string{messageView}
	if t.todoText != "" {
		blocks = append(blocks, t.todoText)
	}
	blocks = append(blocks, textAreaView, bottomView)
	content := lipgloss.JoinVertical(lipgloss.Left, blocks...)

	//水平方向组合padding栏
	content = lipgloss.JoinHorizontal(lipgloss.Left, leftPaddingView, content, rightPaddingView)
	//帮助弹窗：官方 Compositor —— 主界面是底层 Layer，帮助盒 X/Y 定位、Z=1 悬浮其上
	if t.skills.isVisible() {
		content = t.overlayCentered(content, t.skills.View(t.width, t.height))
	}
	return content
}

// overlayCentered 在原内容上增加居中悬浮框（demo 同款：圆角 + 紫色 62）。
// 盒子样式与尺寸常量统一在 skills.go 定义（skillBoxStyle / skillBoxWidth /
// skillBoxHeight），此处只夹取、渲染、定位。
func (t *TUI) overlayCentered(back string, front string) string {
	boxW := min(skillBoxWidth, t.width) //终端过小就夹到屏幕内
	//盒高跟内容动态走：vp 实际渲染行数 + 边框/内边距，上限已由 skills.View 夹过
	boxH := min(lipgloss.Height(front)+skillBoxStyle.GetVerticalFrameSize(), t.height)

	box := skillBoxStyle.
		Width(boxW).
		Height(boxH).
		Render(front)
	x := max(0, (t.width-lipgloss.Width(box))/2) //居中
	y := max(0, (t.height-lipgloss.Height(box))/2)

	content := lipgloss.NewCompositor(
		lipgloss.NewLayer(back),               //z=0 底层：主界面
		lipgloss.NewLayer(box).X(x).Y(y).Z(1), //z=1 悬浮层：帮助盒
	).Render()

	return content
}
