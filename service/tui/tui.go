package tui

// ── 文件布局 ─────────────────────────────────────────────
//
// 本文件只放主 model：TUI 结构体与生命周期（NewTui/Init/Update/View/Run）。
// 其余按职责分文件：
//   model.go    wire JSON 形状 + 事件循环消息（frameMsg/pullSkipMsg）
//   engine.go   引擎交互单一出入口（EngineView 接口 + 全部 t.engine 调用）
//   pull.go     拉取订阅（tea.Tick 链：pullState/pullCmd/pullOnce）
//   frame.go    帧落地操作（applyFrame/applyViewText/componentWidth/wrapViewText）
//   handler.go  按键语义（keyMsgHandler）
//   component.go 组件构建与整屏拼版（NewPrettyXxx/recalcComponentSize/draw/浮层）
//   render.go   渲染类（composeView 记录重放、glamour、通知/终态文本翻译）
//   styles.go   上色（lipgloss 颜色与文本样式 helpers）
//   helps.go / bottom.go / spinner.go / banner.go  各组件自持结构体与方法

import (
	"time"

	"charm.land/bubbles/v2/cursor"
	sp "charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// TUI 是 bubbletea Model。事件循环（Update）只消费 frameMsg 与终端事件，
// 重活（全量重组、glamour）都在 pullCmd 的 goroutine 上。
type TUI struct {
	engine EngineView

	vp viewport.Model
	ta textarea.Model

	width  int
	height int

	leftPadding  int
	rightPadding int

	skills *skills
	bottom *bottom

	// pull 是拉取循环的私有状态（见 pull.go 的 pullState 注释）。
	pull *pullState

	// 以下全部是事件循环的私有状态（只在 Update 里写；v2 的 Update 与
	// View 在 eventLoop 上串行执行，所以 draw() 读它们是安全的）。
	viewText     string // 最近一次上屏的视图全文
	todoText     string
	running      bool
	fatalHandled bool // 终态已处理（/exit 直接退、init 错误等任意键）
	waitingKey   bool // 终态 waitKey=true：等任意键退出
}

// NewTui 组装界面与子组件。view 是引擎侧状态源（*Engine，经 EngineView 接口注入）。
func NewTui(view EngineView) *TUI {
	return &TUI{
		leftPadding:  2,
		rightPadding: 2,
		vp:           NewPrettyViewport(0, 1),
		skills:       NewHelps(),
		bottom: &bottom{
			spinner: NewSpinner(defaultDynamicSpinner, defaultStaticSpinner,
				60*time.Millisecond),
		},
		engine: view,
		pull:   newPullState(),
	}
}

// Init 组装组件并起 pull 链。pullCmd 是订阅的自续形式：tea.Tick 到点在
// cmd goroutine 上拉一轮引擎状态、产出 frameMsg 交给框架送进 Update。
func (t *TUI) Init() tea.Cmd {
	ta, tacmd := NewPrettyTextArea(0, 1, 0)
	t.ta = ta
	return tea.Batch(tacmd, t.pullCmd())
}

// Update 事件分发。分支里只要产生了 cmd 就透传出去（多个用 tea.Batch，
// 避免赋值覆盖丢失）；cmd 产出的 Msg 会有对应的 case 接住，链条才闭环。
func (t *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m := msg.(type) {
	case frameMsg: // pull 链的一帧
		return t.applyFrame(m)

	case cursor.BlinkMsg:
		t.ta, cmd = t.ta.Update(m)
		return t, cmd

	case tea.KeyPressMsg: //按键（只处理按下；kitty 协议终端还有 release 事件，裸匹配 String() 会把 enter 的 release 也当提交）
		return t.keyMsgHandler(m)

	case tea.PasteMsg: // 粘贴：v2 默认开启 bracketed paste，终端粘贴的整段文本（含换行）以 PasteMsg 到达。
		t.ta, cmd = t.ta.Update(m)
		t.recalcComponentSize()
		return t, cmd

	case tea.WindowSizeMsg: //窗口变化
		t.width = m.Width
		t.height = m.Height
		t.applyViewText(t.viewText) // 按新宽度整体重排
		t.recalcComponentSize()
		return t, nil

	case tea.MouseWheelMsg: //鼠标滚动
		t.vp, cmd = t.vp.Update(m)
		t.recalcComponentSize()
		return t, cmd

	case sp.TickMsg: //spinner 帧链（运行态时由 applyFrame 的 Start() 起链）
		cmd := t.bottom.spinner.Update(m)
		t.recalcComponentSize()
		return t, cmd

	case pullSkipMsg: // 坏帧保活（见 pullCmd 注释）：什么都不应用，只续链
		return t, t.pullCmd()
	}
	return t, nil
}

// View 组合整屏画面（viewport + todo + textarea + bottom，帮助浮层叠加其上）。
func (t *TUI) View() tea.View {
	v := tea.NewView(t.draw())
	v.MouseMode = tea.MouseModeCellMotion //启用主要鼠标事件，不含鼠标悬停
	v.AltScreen = true                    //进入备用屏幕：全屏接管，退出时自动还原
	return v
}

// Run 启动 bubbletea 事件循环（阻塞到 tea.Quit）。
func (t *TUI) Run() {
	if _, err := tea.NewProgram(t).Run(); err != nil {
		panic("Error running application: " + err.Error())
	}
}
