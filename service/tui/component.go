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
	ta := textarea.New()
	ta.SetWidth(width)
	ta.SetHeight(height)
	ta.CharLimit = charLimit // 0 表示不限制
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = height
	ta.MaxHeight = height + 4

	// 裸 enter 被 keyMsgHandler 拦截为"提交"，换行必须改绑到这里，
	// 否则 textarea 默认的 InsertNewline 会先吃掉 enter。
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))

	style := textarea.DefaultStyles(false)
	// 不设前景色会落到终端默认灰，输入区显得暗
	style.Focused.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Foreground(cFgBase)
	style.Focused.Text = lipgloss.NewStyle().Foreground(cFgBase)
	style.Focused.CursorLine = lipgloss.NewStyle()

	ta.SetStyles(style)

	// textarea 只有 Focus 后才处理消息
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

// ── 浮层通用件（斜杠命令浮层与 resume 弹框共用）───────────────

// titleHeight 浮层标题占的行数（标题本身 + 空行）。
const titleHeight = 2

// overlayBoxStyle 浮层盒样式唯一定义处：overlayCentered 算尺寸与各浮层排版
// 共用，保证改 padding/边框时自动一致。
var overlayBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(1, 2)

// overlayTextSize 计算给定盒子总宽高下，文本真正可用的宽高。lipgloss v2 的
// Width/Height 连边框一起算，所以要减掉样式的 frame size；全部从
// overlayBoxStyle 推导，改 padding 或边框时自动跟上，不写魔法数字。
func overlayTextSize(boxW, boxH int) (textW, textH int) {
	st := overlayBoxStyle.Width(boxW).Height(boxH)
	return boxW - st.GetHorizontalFrameSize(), boxH - st.GetVerticalFrameSize()
}

// recalcComponentSize 重算所有组件尺寸（demo 范式 + 清单栏）。
func (t *TUI) recalcComponentSize() {
	// 启动早期 WindowSizeMsg 未到，t.width/t.height 还是 0，布局免谈
	if t.width <= 0 || t.height <= 0 {
		return
	}
	componentWidth := t.componentWidth()
	// v2 里 SetWidth 后 textarea 会按新宽度重新软换行并自动调高度
	t.ta.SetWidth(componentWidth)
	t.vp.SetWidth(componentWidth)
	t.bottom.UpdateGap(componentWidth - lipgloss.Width(t.bottom.spinnerView()) - lipgloss.Width(t.bottom.notice))

	bottomHeight := lipgloss.Height(t.bottom.View())
	todoHeight := lipgloss.Height(t.todoText)
	// ta.View() 含边框，实际占 ta.Height()+2 行
	viewportHeight := t.height - lipgloss.Height(t.ta.View()) - bottomHeight - todoHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	// 改高度前先记是否贴底：todo 栏出现/变高会把视口压矮，压矮后同一 YOffset
	// 已经不在底部，applyViewText 下一帧读到的就是"用户滚上去了"，从此不再跟随，
	// 本轮后续消息与最终回答全部停在上屏位置之外。
	wasBottom := t.vp.AtBottom()
	t.vp.SetHeight(viewportHeight)
	if wasBottom {
		t.vp.GotoBottom()
	}
}

// draw 组合组件并绘制整屏，浮层经 Compositor 叠加其上。
func (t *TUI) draw() string {
	messageView := t.vp.View()
	textAreaView := t.ta.View()
	bottomView := t.bottom.View()
	leftPaddingView := lipgloss.NewStyle().Width(t.leftPadding).Height(t.height).Render("")
	rightPaddingView := lipgloss.NewStyle().Width(t.rightPadding).Height(t.height).Render("")

	// 清单为空时不占行
	blocks := []string{messageView}
	if t.todoText != "" {
		blocks = append(blocks, t.todoText)
	}
	blocks = append(blocks, textAreaView, bottomView)
	content := lipgloss.JoinVertical(lipgloss.Left, blocks...)

	content = lipgloss.JoinHorizontal(lipgloss.Left, leftPaddingView, content, rightPaddingView)
	// 浮层：主界面是底层 Layer，浮层盒居中悬浮其上（z=1）
	if t.slashPopup.isVisible() {
		content = t.overlayCentered(slashBoxWidth, slashBoxHeight, content, t.slashPopup.View(t.width, t.height))
	}
	if t.resume.isVisible() {
		content = t.overlayCentered(resumeBoxWidth, resumeBoxHeight, content, t.resume.View(t.width, t.height))
	}
	return content
}

// overlayCentered 在原内容上居中叠加浮层盒；盒样式统一用 overlayBoxStyle，
// 尺寸由调用方传入（各浮层自持常量），此处只夹取、渲染、定位。
func (t *TUI) overlayCentered(boxW, boxH int, back string, front string) string {
	boxW = min(boxW, t.width) //终端过小就夹到屏幕内
	//盒高跟内容走：vp 实际渲染行数 + 边框/内边距
	boxH = min(lipgloss.Height(front)+overlayBoxStyle.GetVerticalFrameSize(), boxH, t.height)

	box := overlayBoxStyle.
		Width(boxW).
		Height(boxH).
		Render(front)
	x := max(0, (t.width-lipgloss.Width(box))/2) //居中
	y := max(0, (t.height-lipgloss.Height(box))/2)

	content := lipgloss.NewCompositor(
		lipgloss.NewLayer(back),               //z=0 底层：主界面
		lipgloss.NewLayer(box).X(x).Y(y).Z(1), //z=1 悬浮层：浮层盒
	).Render()

	return content
}
