package tui

import (
	"strings"

	"HackerTeam/utils/pretty"
	"charm.land/glamour/v2"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sync"
	"sync/atomic"
	"time"
)

// 定义颜色，配色统一来源于 pretty.TuiXxx 常量，确保界面风格统一且美观
var (
	bg          tcell.Color = tcell.GetColor(pretty.TuiBg)          // 整体背景色
	borderColor tcell.Color = tcell.GetColor(pretty.TuiBorderColor) // 边框颜色
	inputAreaBg tcell.Color = tcell.GetColor(pretty.TuiInputAreaBg) // 输入区背景色
)

const (
	// drawInterval 重绘节流间隔。tview 只在 QueueUpdateDraw、按键/鼠标/resize 事件时重绘，
	// 纯流式输出期间这些都不发生，所以由 drawLoop 按固定帧率驱动刷新。
	drawInterval = 30 * time.Millisecond
	// spinnerTicks 指示器每推进一帧占用多少个 drawInterval tick。3 × 30ms = 90ms/帧，
	// 10 帧约 0.9s 转一圈。
	spinnerTicks = 3
)

type Tui struct {
	app       *tview.Application
	appLayout *layout
	inputChan chan string

	// dirty 标记"内容已写入 widget 但尚未上屏"，由 drawLoop 消费。
	// 写入走 QueueUpdate（只改 widget、不重绘）而不是 QueueUpdateDraw：后者每次调用都
	// 阻塞等待一次完整全屏重绘，而 TextView.Draw 内部的 parseAhead 会把整个文本缓冲区
	// 拷贝一遍（t.text.String()），流式输出下就是每 token O(n)、整体 O(n²)。
	dirty    atomic.Bool
	drawOnce sync.Once

	// running 由引擎 goroutine 通过 SetAgentRunning 写、drawLoop 读。
	// 指示器的实际外观切换全在 drawLoop 单线程里做，所以只有这一个字段需要跨 goroutine。
	running atomic.Bool

	// glamour renderer 构建时要解析一遍 style JSON，按宽度缓存复用
	renderMu sync.Mutex
	renderer *glamour.TermRenderer
	renderW  int
}

type layout struct {
	pages        *tview.Pages
	mainFlex     *tview.Flex // ResizeItem 要在父 Flex 上调，所以得存着
	agentMessage *tview.TextView
	todoBar      *todoBar
	noticeBar    *noticeBar
	indicator    *tview.TextView
	inputArea    *tview.TextArea
	helpTable    *helptable
}
type helptable struct {
	h               *tview.Table
	helpPageVisible bool
	// helpItems 由引擎 goroutine 写（AddHelpItems / ResetHelpItems），
	// 由 UI goroutine 读（toggleHelpPage → refreshhelpTable），必须加锁
	mu        sync.Mutex
	helpItems []helpItem
}
type helpItem struct {
	cmd  string
	desc string
}

// ── 输入区左侧的运行指示器 ─────────────────────────────
//
// 做成独立 widget 而不是 TextArea 的 label：TextArea.SetLabel 是无锁写字段
// （textarea.go:792），由 UI 线程在 Draw 中读取，跨 goroutine 调用是数据竞争，
// 每帧都得包一层 QueueUpdate；而 TextView.SetText 自带锁，drawLoop 可以直接调。

const indicatorWidth = 2

var (
	// 空闲态。indicator 开了 SetDynamicColors，所以可以带 tview 颜色标签。
	// 用 TuiSubText 而不是 tview 默认的 label 颜色——后者是 ColorYellow。
	idleIndicator = pretty.TColoredText(pretty.TuiSubText, "> ")
	// 盲文 10 帧，每个都是 1 cell 宽（已实测），加一个空格正好 indicatorWidth 列
	spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
)

// spinnerIndicator 渲染运行态的第 frame 帧。frame 由 drawLoop 单调递增传入，此处取模回绕。
func spinnerIndicator(frame int) string {
	return pretty.TColoredText(pretty.TColorLightMagenta,
		string(spinnerFrames[frame%len(spinnerFrames)])+" ")
}

// ── 输入框上方的两个 bar ─────────────────────────────
//
// TodoBar：纵向清单栏，每个任务一行，有清单时占 N 行、无清单时塌成 0 行。
// NoticeBar：固定 1 行，承载瞬时通知与常驻键位提示，永不塌陷（hint 常驻）。

const (
	// noticeTTL 临时通知的停留时长。到期后 NoticeBar 自动回落到兜底提示。
	noticeTTL = 4 * time.Second
	// minMessageRows 消息区无论如何至少保留的行数。矮终端下用它钳制 TodoBar 高度：
	// Flex 的 distSize = height - 所有 fixedSize 之和且不做钳制，Box.SetRect 也原样
	// 存负高度，而 pos += size 用原始值 → 不钳制会让 AgentMessage 拿到负高度、
	// pos 倒退、下方三个 item 全部画错行并在屏幕底部留残影。
	minMessageRows = 3
)

// NoticeBar 的常驻兜底提示。全小写英文，沿用被删除的 InputHint 的 [gray::d] 暗灰样式。
// esc to interrupt 只在运行态出现——ESC 中断仅在 agent 运行期间有效，平时显示它是噪音。
const (
	hintIdle    = `[gray::d]ctrl+k for help[-:-:-]`
	hintRunning = `[gray::d]esc to interrupt · ctrl+k for help[-:-:-]`
)

// noticeBar 输入框上方的单行通知栏。notice 与 hint 二选一，一次只显示一种。
type noticeBar struct {
	view *tview.TextView

	// 槽位由引擎 goroutine 写（setNotice），由 drawLoop 读写（render 里清理过期
	// 通知），是真·多写入方，必须加锁。
	// 注意与 indicator 的区别：indicator 只有 drawLoop 一个写入方，所以不需要锁。
	mu          sync.Mutex
	notice      string    // 临时通知（已带颜色标签，调用方负责）
	noticeUntil time.Time // 过期时间
}

func (b *noticeBar) setNotice(msg string, ttl time.Duration) {
	b.mu.Lock()
	b.notice = msg
	b.noticeUntil = time.Now().Add(ttl)
	b.mu.Unlock()
}

// render 未过期的 notice 优先，否则回落到 hint（按 running 二选一）。
// running 由调用方（drawLoop）传入，它本来就已经读过这个原子标志。
func (b *noticeBar) render(running bool) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.notice != "" {
		if time.Now().Before(b.noticeUntil) {
			return b.notice
		}
		b.notice = "" // 过期即清，避免每帧反复比对一个死字符串
	}
	if running {
		return hintRunning
	}
	return hintIdle
}

// todoBar 输入框上方的纵向清单栏（每个任务一行）。有清单时占 N 行，无清单时塌成 0 行。
type todoBar struct {
	view *tview.TextView

	// text 是引擎 goroutine 推来的原始多行纯文本（每个任务一行，未转义）。转义、
	// 着色、SetText、ResizeItem 全在 drawLoop 里做，保持"widget 写入单线程"。
	// 引擎写、drawLoop 读，是真·多写入方，必须加锁。
	mu   sync.Mutex
	text string
}

func (b *todoBar) store(text string) {
	b.mu.Lock()
	b.text = text
	b.mu.Unlock()
}

func (b *todoBar) current() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text
}

// SetTodoText 更新 TodoBar 的清单文本，传空串则整栏塌成 0 行。
// 当前收到的是多行文本（functionTools.TodoStatusBar 的输出，每个任务一行）。
// ⚠️ text 是**来自框架/LLM 的纯文本**（以 "[TODO] " 开头，且条目正文由 LLM 撰写），
// 转义与着色由 drawLoop 负责——与 ShowNotice 的契约相反（那个收的是受信 markup，
// 转义会破坏颜色标签）。
// 由 agent 包的 BeforeModel callback 在每次 LLM hop 推送，可从任意 goroutine 调用。
func (t *Tui) SetTodoText(text string) {
	t.appLayout.todoBar.store(text)
	t.markDirty()
}

// drawState 是 drawLoop 的私有状态。只被 drawLoop 这一个 goroutine 读写，
// 不是共享状态，因此无需同步（刻意不做成 Tui 的字段，避免误导后人以为要加锁）。
type drawState struct {
	showingRunning bool   // indicator 当前显示的是否为运行态
	spinTick       int    // spinner 帧推进计数
	shownTodo      string // TodoBar 上一帧的原始文本
	shownNotice    string // NoticeBar 上一帧的渲染结果
}

// startDrawLoop 启动固定帧率的重绘循环，只启动一次。
// 它是"重绘节流 + 动画时钟"的多重身份：除了按 dirty 标志节流重绘，还独占驱动
// 运行指示器动画与两个 bar 的内容刷新。所有 widget 写入因此都是单线程的，
// 配合 TextView.SetText 自带锁，既不需要额外同步也不需要为每件事单起 ticker。
func (t *Tui) startDrawLoop() {
	t.drawOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(drawInterval)
			defer ticker.Stop()

			st := &drawState{}
			for range ticker.C {
				running := t.running.Load()
				t.tickIndicator(running, st)
				t.tickTodoBar(st)
				t.tickNoticeBar(running, st)
				// 没有新内容就不重绘，空闲时不产生任何 CPU 开销
				if t.dirty.CompareAndSwap(true, false) {
					t.app.Draw()
				}
			}
		}()
	})
}

// tickIndicator 推进运行指示器。
// 用 if/else-if 而非 switch —— 项目风格要求，不要改回 switch。
func (t *Tui) tickIndicator(running bool, st *drawState) {
	if running != st.showingRunning { // 状态翻转：立刻换外观，帧计数归零
		st.showingRunning = running
		st.spinTick = 0
		if running {
			t.setIndicator(spinnerIndicator(0))
		} else {
			t.setIndicator(idleIndicator)
		}
	} else if running { // 持续运行：每 spinnerTicks 个 tick 推进一帧
		st.spinTick++
		if st.spinTick%spinnerTicks == 0 {
			t.setIndicator(spinnerIndicator(st.spinTick / spinnerTicks))
		}
	}
}

// tickNoticeBar 刷新通知栏。TTL 到期、通知到达、running 翻转三件事全走这一条路径，
// 因此不需要任何事件驱动的机制。
func (t *Tui) tickNoticeBar(running bool, st *drawState) {
	nb := t.appLayout.noticeBar
	if s := nb.render(running); s != st.shownNotice {
		st.shownNotice = s
		nb.view.SetText(s) // TextView.SetText 自带锁，无需 QueueUpdate
		t.dirty.Store(true)
	}
}

// tickTodoBar 刷新清单栏的内容与高度（0 到 N 行，每个任务一行）。
//
// 高度钳制的原因：Flex 的 distSize = height - 所有 fixedSize 之和且不做钳制，
// Box.SetRect 也原样存负高度，而 pos += size 用原始值。终端高度不足时 distSize
// 变负 → AgentMessage 拿到负高度 → pos 倒退 → 下方三个 item 全部画错行、
// 屏幕底部留未绘制残影。行数超过 max 时压到 max（顶部行可见，其余被挡住）；
// max 不足 1 时塌成 0 行。
//
// 多行后必须 ScrollToBeginning()：SetText 只调 resetIndex()，不清
// lineOffset/trackEnd，而 NewTextView() 默认 scrollable=true，滚轮下滚会把
// trackEnd 置真、导致视图永久卡在底部。SetScrollable(false) 不是解法
// （它会顺手把 trackEnd 设成 true，直接底部锚定）。
func (t *Tui) tickTodoBar(st *drawState) {
	tb := t.appLayout.todoBar
	text := tb.current()
	if text == st.shownTodo {
		return
	}
	st.shownTodo = text

	rendered := ""
	n := 0
	if text != "" {
		rendered = renderTodoLines(text)
		n = strings.Count(text, "\n") + 1
	}

	// ResizeItem 无锁写 FixedSize/Proportion、GetInnerRect 无锁读布局字段，两者都
	// 由 UI 线程在 Draw 中使用，所以必须一起放进 QueueUpdate（那里就是 UI 线程）。
	t.app.QueueUpdate(func() {
		_, _, _, total := t.appLayout.mainFlex.GetInnerRect()
		// NoticeBar 与 InputRow 各占 1 行，消息区至少留 minMessageRows 行
		if max := total - 2 - minMessageRows; n > max {
			n = max
		}
		if n < 0 {
			n = 0
		}
		tb.view.SetText(rendered)
		tb.view.ScrollToBeginning()
		// proportion 必须是 0：给 1 会让 TodoBar 与 AgentMessage 平分剩余空间
		t.appLayout.mainFlex.ResizeItem(tb.view, n, 0)
	})
	t.dirty.Store(true)
}

// renderTodoLines 把 TodoBar 的多行纯文本转义并逐行上色：
// [TODO] 头行与 (N done) 计数行暗灰、◐ 进行中青色、☐ 待办正文色。
// 必须逐行先 tview.Escape 再包颜色标签："[TODO]" 会被 tview 当成前景色名整个吞掉
// （实测 "[TODO] x" 的 TaggedStringWidth 是 2 而非 8），条目正文由 LLM 撰写、
// 可能含 ASCII 方括号。Escape 与着色都不改变行数，高度计算不受影响。
func renderTodoLines(text string) string {
	colorOf := func(line string) string {
		switch {
		case strings.HasPrefix(line, "◐ "):
			return pretty.TColorCyan
		case strings.HasPrefix(line, "☐ "):
			return pretty.TuiMainText
		default: // [TODO] 头行、计数行
			return pretty.TuiSubText
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = pretty.TColoredText(colorOf(line), tview.Escape(line))
	}
	return strings.Join(lines, "\n")
}

// ShowNotice 在 NoticeBar 显示一条临时通知，noticeTTL 后自动回落到兜底提示。
// ⚠️ msg 必须是**已带 tview 颜色标签的受信 markup**（用 pretty.TBarXxx 系列生成），
// 本方法不做转义——转义会破坏颜色标签。与 SetTodoText 的契约相反。
// 只取 mutex 写字段、不走 QueueUpdate，因此在 app.Run() 启动前调用也不会阻塞。
// 这比被它替换掉的 ShowSuccessInMsgView 更好：后者经 PrintToMsgView → QueueUpdate，
// 会在事件循环启动前阻塞住 init 序列。
func (t *Tui) ShowNotice(msg string) {
	t.appLayout.noticeBar.setNotice(msg, noticeTTL)
	t.markDirty()
}

// setIndicator 更新指示器文本并请求重绘。只允许 drawLoop 调用。
// TextView.SetText 自带锁，可以直接从本 goroutine 调，不需要 QueueUpdate。
func (t *Tui) setIndicator(s string) {
	t.appLayout.indicator.SetText(s)
	t.dirty.Store(true)
}

// SetAgentRunning 标记 agent 是否正在运行。只写一个原子标志，可从任意 goroutine 调用、
// 不阻塞；指示器的实际切换由 drawLoop 在下一帧完成（最多 drawInterval 延迟）。
func (t *Tui) SetAgentRunning(running bool) {
	t.running.Store(running)
}

// markDirty 标记需要重绘。必须在 widget 写入完成之后调用：QueueUpdate 是阻塞的，
// 返回时内容已经落地，这样 drawLoop 的下一帧才画得到它。反过来（先标记后写入）
// 会出现"这一帧把标记消费掉了、内容却还没写进去"，导致最后一批文本永不上屏。
func (t *Tui) markDirty() {
	t.startDrawLoop()
	t.dirty.Store(true)
}

func (t *Tui) PrintToMsgView(content string, clear bool) {
	t.app.QueueUpdate(func() {
		if clear {
			t.appLayout.agentMessage.Clear()
		}
		fmt.Fprint(t.appLayout.agentMessage, content)
	})
	// 这里不调 ScrollToEnd()。它会把 TextView 的 trackEnd 强行置回 true，于是用户在
	// 流式输出期间往上翻滚查看历史时，下一个 token 就把视图弹回底部。trackEnd 本身就是
	// "是否在底部、是否该跟随"的标记，tview 自己维护（滚轮上翻置 false、翻回底部置 true），
	// 初始值在 GetTuiService 里开一次即可。
	t.markDirty()
}

func (t *Tui) ListenUserInput() chan string {
	t.app.QueueUpdateDraw(func() {
		t.app.SetFocus(t.appLayout.inputArea)

		//注册一个输入捕获器，每次用户在输入框敲击键盘时都会触发
		t.appLayout.inputArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			// Ctrl+K 切换帮助页
			if event.Key() == tcell.KeyCtrlK {
				t.toggleHelpPage()
				return nil
			}

			// Enter 提交输入
			// ModNone = 0，无任何修饰键（Ctrl/Shift/Alt 均未按下），即裸按 Enter。
			// Shift+Enter 落到函数末尾的 return event，由 TextArea 插入换行（手动多行输入）。
			// bracketed paste 保证粘贴里的 \n 走 PasteEvent 通道，不会产生 KeyEnter 事件
			if event.Key() == tcell.KeyEnter && event.Modifiers() == tcell.ModNone {

				//获取输入文本
				text := t.appLayout.inputArea.GetText()
				// 发送输入文本到 inputChan。default 分支：对端（引擎循环）未在监听时
				// （如自动 turn 期间）不投递，避免 unbuffered send 阻塞 tview 事件循环
				// 导致 UI 卡死。注意：只有投递成功才清空输入框，default 分支保留文本，
				// 用户输入不丢失。
				select {
				case t.inputChan <- text:
					t.appLayout.inputArea.SetText("", false)
				default:
				}
				return nil //Enter事件不捕获
			}

			//传递事件给 TextArea 默认处理（插入字符、换行等）
			return event
		})
	})
	return t.inputChan

}

func (t *Tui) SetAppFuncTriggerWithEsc(f func()) {
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyEscape {
				f() // 执行回调
				return nil
			}
			return event // 其他按键正常传递
		})
	})
}

func (t *Tui) ClearAppFuncTrigger() {
	(*t).app.QueueUpdateDraw(func() {
		(*t).app.SetInputCapture(nil)
	})
}

// showMsgAndExit 打印一条消息并结束程序。waitForKey=true 时等用户按任意键再退出。
//
// 末尾的 select{} 是故意的，不要改成"由回调 close(chan) 然后返回"：
//   - 调用方（引擎 init 序列）打完致命错误后并没有 return，一旦这里返回，引擎 goroutine
//     会带着未初始化的状态（如 nil memory service）继续往下跑，最后在别处 panic。
//   - 也不能在下面的回调里直接 os.Exit：screen.Fini() 是在 app.Run() 的返回路径上调的，
//     从回调硬退出会跳过它，终端会留在 alt-screen + raw mode，退出后用户的 shell 是坏的。
//
// 现在的收尾链路是：app.Stop() → tui.Run() 返回 → tview 复原终端 → main() 返回 → 进程退出。
func (t *Tui) showMsgAndExit(msg string, waitForKey bool) {
	t.PrintToMsgView(msg, false)
	// 强制刷一帧：PrintToMsgView 现在只写入不重绘，不显式 Draw 的话退出信息
	// 可能还没上屏 app 就 Stop 了。
	t.app.Draw()
	t.app.QueueUpdate(func() {
		if !waitForKey {
			t.app.Stop()
			return
		}
		//只要有按键就退出程序
		t.app.SetFocus(t.appLayout.agentMessage)
		t.appLayout.agentMessage.SetInputCapture(
			func(event *tcell.EventKey) *tcell.EventKey {
				t.app.Stop()
				return nil
			})
	})
	select {}
}

func (t *Tui) ShowErrorInMsgViewAndExit(errmsg string) {
	t.showMsgAndExit(errmsg, true)
}

func (t *Tui) ShowSuccessInMsgViewAndExit(sussessmsg string) {
	t.showMsgAndExit(pretty.TSuccess(sussessmsg), true)
}

func (t *Tui) ShowMsgAndExitNoTrigger(msg string) {
	t.showMsgAndExit(msg, false)
}

func (t *Tui) AddHelpItems(items []map[string]string) {
	ht := t.appLayout.helpTable
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for _, i := range items {
		for k, v := range i {
			ht.helpItems = append(ht.helpItems, helpItem{cmd: k, desc: v})
		}
	}
}

func (t *Tui) ResetHelpItems() {
	t.defaultHelpItems()
}

// RenderMarkdown 用 glamour 渲染 markdown。
func (t *Tui) RenderMarkdown(in string) (string, error) {
	r, err := t.glamourRenderer()
	if err != nil {
		return "", err
	}
	return r.Render(in)
}

// glamourRenderer 返回按当前消息区宽度缓存的 renderer，宽度变化时才重建。
func (t *Tui) glamourRenderer() (*glamour.TermRenderer, error) {
	// 宽度读取与竞争规避见 banner.go 里 contentWidth 的注释
	w := t.contentWidth()
	if w < 40 {
		w = 80
	}

	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if t.renderer != nil && t.renderW == w {
		return t.renderer, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w),
		glamour.WithStylesFromJSONBytes([]byte(`{
			"document": {
				"margin": 0
			}
		}`)),
	)
	if err != nil {
		return nil, err
	}
	t.renderer, t.renderW = r, w
	return r, nil
}

func (t *Tui) Run() {
	err := (*t).app.Run()
	if err != nil {
		panic("Error running application: " + err.Error())
	}
}
func GetTuiService() *Tui {
	//设置Agent消息显示区
	AgentMessage := tview.NewTextView().
		SetDynamicColors(true). // 启用颜色
		SetScrollable(true).    // 可滚动
		SetWrap(true)

	AgentMessage.SetBackgroundColor(bg) // 设置背景颜色
	// 打开"跟随底部"。SetScrollable(true) 不会设置 trackEnd（只有传 false 才会），
	// 所以必须显式开一次，否则视图会永远停在顶部、完全不跟随新输出。
	// 之后用户在底部就继续跟随、往上翻就不打扰，全部由 tview 自己维护。
	AgentMessage.ScrollToEnd()

	// 输入区左侧的运行指示器：空闲显示 ">"，agent 运行中显示盲文 spinner，由 drawLoop 驱动。
	// 背景必须是 inputAreaBg：这个位置原本在 TextArea 内部、底色就是 inputAreaBg，
	// 不设的话会继承 InputRow 的 bg，左边出现一块 2 格的色差。
	Indicator := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetText(idleIndicator)
	Indicator.SetBackgroundColor(inputAreaBg)

	//设置底部输入区（不再有 label，提示符由 Indicator 承担）
	InputArea := tview.NewTextArea().SetWrap(true)
	InputArea.SetBackgroundColor(inputAreaBg)
	InputArea.SetTextStyle(tcell.StyleDefault.
		Background(inputAreaBg).                        // 输入区背景色
		Foreground(tcell.GetColor(pretty.TuiMainText))) // 文字颜色

	// 消息区与通知栏之间的清单栏：纵向，每个任务一行，有清单时占 N 行、无清单时塌成 0 行。
	// 背景用 bg（不是 inputAreaBg）：它是参考信息，视觉上应融入消息区，
	// 与下方"控制区"（NoticeBar + InputRow）区分开。
	// 高度由 drawLoop 通过 mainFlex.ResizeItem 动态设置，初始 0 行。
	// 不要对它调 SetScrollable(false)——那会把 trackEnd 置真（见 tickTodoBar 注释）。
	TodoBar := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	TodoBar.SetBackgroundColor(bg)

	// 输入框上方的通知栏：瞬时通知 + 常驻键位提示，固定 1 行、永不塌陷。
	// 不设背景色，继承 MainFlex 的 bg——让它融入消息区而不是和 InputRow 连成一块。
	// 内容全部带颜色标签，所以不需要 SetTextColor。
	// 内容靠右：与原来 InputHint 在 InputRow 右侧的位置一致，视线落点不变。
	// 注意通知与兜底提示共用这一个 widget，所以两者都靠右。
	NoticeBar := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignRight).
		SetText(hintIdle)

	InputRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	InputRow.SetBackgroundColor(bg)
	InputRow.AddItem(Indicator, indicatorWidth, 0, false) // 左侧运行指示器
	InputRow.AddItem(InputArea, 0, 1, true)

	//设置整体布局
	MainFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	MainFlex.SetBackgroundColor(bg)
	MainFlex.AddItem(AgentMessage, 0, 1, false) // Agent消息区占剩余空间
	MainFlex.AddItem(TodoBar, 0, 0, false)      // 清单栏，初始 0 行，由 ResizeItem 动态调整
	MainFlex.AddItem(NoticeBar, 1, 0, false)    // 通知栏，固定 1 行、永不塌陷
	MainFlex.AddItem(InputRow, 1, 0, true)      // 底部的输入区

	HelpTable := tview.NewTable()
	HelpTable.SetBackgroundColor(bg)
	HelpTable.SetBorder(true)
	HelpTable.SetBorderColor(borderColor)
	HelpTable.SetTitle(" 斜杠指令 — Ctrl+K / Esc 关闭 ")
	HelpTable.SetTitleAlign(tview.AlignLeft)
	HelpTable.SetSelectable(true, false) // 行可选，列不可选

	HelpTable.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.GetColor("#2A3A5C")).
		Foreground(tcell.GetColor(pretty.TuiMainText)))

	app := tview.NewApplication()
	pages := tview.NewPages()
	pages.AddPage("AgentPage", MainFlex, true, true)
	app.SetRoot(pages, true) // true = 全屏模式
	app.EnableMouse(true)    //允许接收鼠标事件
	app.EnablePaste(true)    //启用 bracketed paste，避免长文本粘贴时逐字符处理导致 CPU 飙升和界面卡死

	tui := &Tui{
		app: app,
		appLayout: &layout{
			pages:        pages,
			mainFlex:     MainFlex,
			agentMessage: AgentMessage,
			todoBar:      &todoBar{view: TodoBar},
			noticeBar:    &noticeBar{view: NoticeBar},
			indicator:    Indicator,
			inputArea:    InputArea,
			helpTable: &helptable{
				h:               HelpTable,
				helpItems:       []helpItem{},
				helpPageVisible: false,
			},
		},
		inputChan: make(chan string),
	}

	(*(*(*tui).appLayout).helpTable).h.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlK {
			tui.toggleHelpPage()
			return nil
		}
		return event
	})
	tui.defaultHelpItems()
	tui.refreshhelpTable()
	return tui

}
