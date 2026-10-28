package tui

// ── pull 链（tea.Tick 订阅）──────────────────────────────
//
// 固定帧率拉引擎五类状态 + 终态观察。tea.Tick 到点产出 frameMsg 送进
// Update，Update 处理完返回下一个 pullCmd 续链，不经过 program.Send。
// 拉取与解析走 engine.go 的封装，本文件只做状态记账与帧组装。

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

// pullState pull 链的私有状态。cmd 链是串行的（同一时刻至多一个 pullCmd
// 在飞，绝不并发两个，否则这里就是数据竞争），只被 pullCmd 的 goroutine 读写。
type pullState struct {
	seenVersion   uint64    // 已消费的消息日志版本
	pending       bool      // 有待重组的视图（节流窗口内推迟，不会丢）
	lastCompose   time.Time // 上次视图重组时间（节流）
	composedWidth int       // 当前 st.view 排版时的宽度（0=尚未排版，变化即强制重组）
	banner        string    // 启动横幅（就绪后进视图头部，只渲染一次）
	bannerDone    bool
	bootDone      bool // 首次启动状态已决（离开 booting 即停拉，帧不再携带）
	lastFatal     *wireFatal
	view          string      // 最近一次重组的视图全文（帧恒携带，重复帧交给框架去重）
	spans         []blockSpan // 视图的可折叠块行号索引（与 view 同源同帧）
	expandVer     uint64      // 产出 view/spans 那次重组起点的展开状态版本
	width         int         // 本轮重组用的组件宽度（glamour 按此换行）
}

func newPullState() *pullState {
	return &pullState{}
}

// pullCmd 是 pull 链的 Elm 载体：tea.Tick 到点后在 cmd goroutine 上执行一轮
// 拉取、产出 frameMsg 送进 Update，Update 返回下一个 pullCmd 续链。重活
// （重组、glamour）都在 cmd 的 goroutine 上，事件循环只收轻量帧，按键永不被
// 渲染阻塞；重复帧的渲染去重交给 renderer。
//
// 宽度在续链时（事件循环上）读取并捕获进闭包——cmd goroutine 拿到的是不可变
// 快照，不碰 model 字段，无需同步。resize 的即时重排由 applyViewText 负责。
//
// ⚠️ fn 绝不能返回 nil：nil 不会触发 Update 的续链分支，链就静默断掉。
// 坏帧用 pullSkipMsg 保活。
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
// RunStateJSON 解析失败时出现（下一轮重拉）。
//
// 刻意不做帧级去重——重复帧是常态，去重脏活交给 renderer（viewEquals
// 整帧短路 + cell diff，不产生终端 I/O），自己再去重只引入状态与 bug 面。
//
// w 是 pullCmd 续链时捕获的宽度快照，供 glamour 换行与横幅拼版使用。
func (t *TUI) pullOnce(w int) (frameMsg, bool) {
	st := t.pull
	st.width = w

	// 点击折叠切换：绕过 composeInterval 节流（点击反馈要即时）。
	if t.forceCompose.CompareAndSwap(true, false) {
		st.pending = true
		st.lastCompose = time.Time{}
	}

	rs, ok := t.fetchRunState()
	if !ok {
		return frameMsg{}, false // 坏帧不致盲：下一轮重拉
	}

	// 启动横幅：宽度就绪后组装一次（0 宽度会把横幅永久降级成堆叠版）。
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
	// 宽度变化强制重组（绕过节流）：applyViewText 不改写内容，resize 后
	// 旧排版尽快被新宽度重组替换。
	widthChanged := st.composedWidth != w && w > 0
	if (st.pending && time.Since(st.lastCompose) >= composeInterval) || widthChanged {
		st.pending = false
		st.lastCompose = time.Now()
		st.composedWidth = w
		// 版本在 compose 起点读：此后到帧落地之间若有新点击（版本前进），
		// applyFrame 会把它认成旧帧，锚点留给下一轮强制重组消费。
		st.expandVer = t.expand.version()
		st.view, st.spans = t.composeView(st)
	}

	// 通知栏槽位：TTL 内显示通知，否则回落常驻 hint（运行态多一条 esc 提示）。
	notice := t.fetchNotice(rs.Running, w)

	// 首次启动状态：拉到终态（config/ready）即停；config 态的帧携带预填值
	// 作为一次性开页信号，applyFrame 据此开页（引擎装配与 TUI 并发起跑，
	// booting 阶段必须重拉，TUI Init 单次查询有竞态）。
	var bootField map[string]string
	if !st.bootDone {
		if fs, ok := t.fetchFirstStartup(); ok && fs.State != "booting" {
			st.bootDone = true
			if fs.State == "config" {
				bootField = fs.Fields
			}
		}
	}

	return frameMsg{
		view:      st.view,
		spans:     st.spans,
		expandVer: st.expandVer,
		todo:      todoLines(t.fetchTodoText()),
		notice:    notice,
		running:   rs.Running,
		fatal:     rs.Fatal,
		bootField: bootField,
	}, true
}
