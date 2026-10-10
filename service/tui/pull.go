package tui

// ── pull 链（tea.Tick 订阅）──────────────────────────────
//
// 心跳与旧 drawLoop 相同：固定帧率拉引擎五类状态（横幅一次、消息日志按版本、
// 运行态、清单、通知）+ 终态观察。交付方式是 Elm 订阅的自续形式：tea.Tick
// 到点产出 frameMsg 送进 Update，Update 处理完返回下一个 pullCmd 续链
// （与 spinner 的 Tick 续链同构），不经过 program.Send。
//
// 引擎数据的拉取与解析走 engine.go 的封装，本文件只做状态记账与帧组装。

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	// pullInterval 拉取帧率。既是状态上屏延迟上界，也是通知 TTL 的时钟。
	pullInterval = 30 * time.Millisecond
	// composeInterval 视图重组的最小间隔：把 delta 高频到达时的无效重组合并掉。
	composeInterval = 100 * time.Millisecond
)

// pullState pull 链的私有状态。只被 pullCmd 的 goroutine 读写——cmd 链是
// 串行的（同一时刻至多一个 pullCmd 在飞：Init 发一个、applyFrame 每帧续一个，
// 绝不并发两个，否则这里就是数据竞争），Update 从不触碰。
type pullState struct {
	seenVersion  uint64    // 已消费的消息日志版本
	pending      bool      // 有待重组的视图（节流窗口内推迟，不会丢）
	lastCompose  time.Time // 上次视图重组时间（节流）
	banner       string    // 启动横幅（就绪后进视图头部，只渲染一次）
	bannerDone   bool
	lastFatal    *wireFatal
	view         string // 最近一次重组的视图全文（帧恒携带，重复帧交给框架去重）
	width        int    // 本轮重组用的组件宽度（glamour 按此换行）
	glamRenderer *glamourCache
}

func newPullState() *pullState {
	return &pullState{glamRenderer: &glamourCache{}}
}

// pullCmd 是 pull 链的 Elm 载体：tea.Tick 到点后在 cmd goroutine 上执行一轮
// 拉取、产出 frameMsg 送进 Update；Update 处理完返回下一个 pullCmd 续链
// （与 spinner 的 Tick 续链同构）。重活（重组、glamour）都在 cmd 的 goroutine
// 上，事件循环只收轻量帧，按键永不被渲染阻塞；重复帧的渲染去重交给 renderer
// （viewEquals + cell diff）。
//
// 宽度在续链时（事件循环上）读取并捕获进闭包——cmd goroutine 拿到的是不可变
// 快照，不碰 model 字段，无需任何同步。快照最多滞后一个 pullInterval，且只
// 影响"新重组的帧"；resize 的即时重排由 Update 的 applyViewText 负责。
//
// ⚠️ fn 绝不能返回 nil：nil 不会触发 Update 的续链分支，链就静默断掉。
// 坏帧（RunStateJSON 解析失败，理论死路的防御性兜底）用 pullSkipMsg 保活。
func (t *TUI) pullCmd() tea.Cmd {
	w := max(0, t.componentWidth())
	return tea.Tick(pullInterval, func(time.Time) tea.Msg {
		if frame, ok := t.pullOnce(w); ok {
			return frameMsg(frame)
		}
		return pullSkipMsg{}
	})
}

// pullOnce 执行一轮拉取：按需重组视图、组装帧并返回。false 仅在
// RunStateJSON 解析失败时出现（坏帧不致盲：下一轮重拉）。
//
// 刻意不做帧级去重——重复帧（30ms 一帧、重组 100ms 节流）是常态，脏活
// 框架做：renderer 的 flush 有 viewEquals 整帧相等短路，内部是 cell 级
// 双缓冲 diff，重复帧不会产生任何终端 I/O。自己再去重只会引入状态与
// bug 面（上一版 lastSent 的先赋值后比较曾让流式输出一个字都刷不出来）。
//
// w 是 pullCmd 续链时捕获的宽度快照（事件循环上读取），供 glamour 换行
// 与横幅拼版使用。
func (t *TUI) pullOnce(w int) (frameMsg, bool) {
	st := t.pull
	st.width = w

	rs, ok := t.fetchRunState()
	if !ok {
		return frameMsg{}, false // 坏帧不致盲：下一轮重拉
	}

	// 启动横幅：引擎就绪且宽度已知后组装一次（宽度未就绪时等下一轮，
	// 否则 0 宽度会把横幅永久降级成堆叠版）。
	if !st.bannerDone && w > 0 {
		if lines, ok := t.fetchStartupInfo(); ok {
			st.bannerDone = true
			st.banner = composeBanner(lines, w)
			st.pending = true
		}
	}

	// 消息日志：版本有变化就标记待重组；重组按 composeInterval 节流。
	if ver := t.fetchVersion(); ver != st.seenVersion {
		st.seenVersion = ver
		st.pending = true
	}
	// 终态出现也要重组一次（fatal 不 bump 版本，必须显式触发）。
	if rs.Fatal != nil && rs.Fatal != st.lastFatal {
		st.lastFatal = rs.Fatal
		st.pending = true
	}
	if st.pending && time.Since(st.lastCompose) >= composeInterval {
		st.pending = false
		st.lastCompose = time.Now()
		st.view = t.composeView(st)
	}

	// 通知栏槽位：TTL 内显示通知，否则回落常驻 hint（运行态多一条 esc 提示）。
	notice := t.fetchNotice(rs.Running)

	return frameMsg{
		view:    st.view,
		todo:    todoLines(t.fetchTodoText()),
		notice:  notice,
		running: rs.Running,
		fatal:   rs.Fatal,
	}, true
}
