package tui

// 块折叠与点击命中的单元测试：行号映射的数学、命中规则、展开状态机、
// 流式尾段伪 ID 的生命周期（不泄漏到下一步）。

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// mm 构造 assistant model.Message（与 engine.wireMsg 序列化后同形）。
func mm(role, reasoning, content string) model.Message {
	return model.Message{Role: model.Role(role), ReasoningContent: reasoning, Content: content}
}

func TestComposeBlocksJoinEquivalence(t *testing.T) {
	// 与原先 strings.Join(parts, "\n\n") 等价：块间恰一个空行
	blocks := []viewBlock{{text: "a"}, {text: "b\nc"}, {text: "d"}}
	got, spans := composeBlocks(blocks)
	if got != "a\n\nb\nc\n\nd" {
		t.Fatalf("join broken: %q", got)
	}
	if len(spans) != 0 {
		t.Fatalf("no id → no spans, got %#v", spans)
	}
}

func TestComposeBlocksSpanLines(t *testing.T) {
	blocks := []viewBlock{
		{text: "banner"},
		{id: "think:1", text: "l1\nl2\nl3"},
		{text: "body"},
		{id: "tool:2", text: "▸ x"},
	}
	_, spans := composeBlocks(blocks)
	// 行布局：0 banner / 1 空 / 2-4 think / 5 空 / 6 body / 7 空 / 8 tool
	if len(spans) != 2 || spans[0].start != 2 || spans[0].id != "think:1" ||
		spans[1].start != 8 || spans[1].id != "tool:2" {
		t.Fatalf("spans broken: %#v", spans)
	}
}

func TestHeaderIDAtOnlyFirstLine(t *testing.T) {
	spans := []blockSpan{{start: 2, id: "think:1"}, {start: 7, id: "tool:2"}}
	if id, ok := headerIDAt(spans, 2); !ok || id != "think:1" {
		t.Fatalf("header line miss: %q %v", id, ok)
	}
	// 块内非首行不是热区：正文点击不折叠（与文本选择不冲突）
	if _, ok := headerIDAt(spans, 3); ok {
		t.Fatalf("body line must not be a hit zone")
	}
	if _, ok := headerIDAt(spans, 5); ok {
		t.Fatalf("non-block line must not be a hit zone")
	}
}

func TestSpanByID(t *testing.T) {
	spans := []blockSpan{{start: 2, id: "a"}, {start: 9, id: "b"}}
	if s, ok := spanByID(spans, "b"); !ok || s.start != 9 {
		t.Fatalf("spanByID broken: %#v %v", s, ok)
	}
	if _, ok := spanByID(spans, "c"); ok {
		t.Fatalf("missing id must not match")
	}
}

func TestExpandStateToggleAndClearTail(t *testing.T) {
	e := newExpandState()
	if e.expanded("x") {
		t.Fatalf("default must be collapsed")
	}
	if !e.toggle("x") {
		t.Fatalf("first toggle → expanded")
	}
	if e.expanded("tail:think") {
		t.Fatalf("clean state has no tail entry")
	}
	e.m[tailThinkID] = true // 模拟流式期间点开
	e.clearTail()
	if e.expanded(tailThinkID) {
		t.Fatalf("clearTail must purge tail entry")
	}
	if !e.expanded("x") {
		t.Fatalf("clearTail must not touch other blocks")
	}
	e.reset()
	if e.expanded("x") {
		t.Fatalf("reset must clear all")
	}
}

func TestMouseClickTogglesAndAnchors(t *testing.T) {
	tu := NewTui(&fakeView{})
	tu.spans = []blockSpan{{start: 5, id: "tool:3"}}
	tu.vp.SetHeight(10)
	tu.vp.SetWidth(80)
	tu.vp.SetContent(strings.Repeat("pad\n", 20))
	tu.vp.SetYOffset(3) // 内容行 5 在屏幕行 2

	// 非左键不响应
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 2, Button: tea.MouseRight})
	if tu.expand.expanded("tool:3") {
		t.Fatalf("right click must not toggle")
	}
	// 左键点中头部行：展开 + 锚 + 绕过节流
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 2, Button: tea.MouseLeft})
	if !tu.expand.expanded("tool:3") {
		t.Fatalf("left click on header must toggle")
	}
	if tu.anchor == nil || tu.anchor.id != "tool:3" || tu.anchor.row != 2 {
		t.Fatalf("anchor broken: %+v", tu.anchor)
	}
	if !tu.forceCompose.Load() {
		t.Fatalf("click must bypass compose throttle")
	}
	// 再次点击收起
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 2, Button: tea.MouseLeft})
	if tu.expand.expanded("tool:3") {
		t.Fatalf("second click must collapse")
	}
	// 点空白行：无状态变化
	tu.anchor = nil
	tu.forceCompose.Store(false)
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if tu.anchor != nil || tu.forceCompose.Load() {
		t.Fatalf("miss click must be a no-op")
	}
}

func TestMouseClickBelowViewportIsNoop(t *testing.T) {
	// 视口下方（todo/输入框/底栏）不是消息区：line = m.Y + YOffset 落在
	// 视口外但仍属全文的行号上，若无守卫，行号撞上块头就会误切换并设锚。
	// （历史 bug：无显式守卫，上滚回看时点输入框可误命中屏幕外的块。）
	tu := NewTui(&fakeView{})
	tu.vp.SetHeight(10)
	tu.vp.SetWidth(80)
	tu.vp.SetContent(strings.Repeat("pad\n", 30))

	// 屏幕行 13 在视口外（height=10）：行号 13 撞块头也必须扑空
	tu.spans = []blockSpan{{start: 13, id: "tool:99"}}
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 13, Button: tea.MouseLeft})
	if tu.expand.expanded("tool:99") || tu.anchor != nil {
		t.Fatalf("click below viewport must be a no-op")
	}

	// 边界：视口最后一行仍有效
	tu.spans = []blockSpan{{start: 9, id: "tool:98"}}
	tu.mouseClickHandler(tea.MouseClickMsg{X: 0, Y: 9, Button: tea.MouseLeft})
	if !tu.expand.expanded("tool:98") {
		t.Fatalf("click on last viewport row must still hit")
	}
}

func TestThinkingCollapsedAndExpand(t *testing.T) {
	fake := &fakeView{records: []string{
		engineWireToolRecord(t, "message", mm("assistant", "deep thought\n"+strings.Repeat("more ", 50), "")),
	}}
	tu := NewTui(fake)
	tu.pull.width = 120
	view, spans := tu.composeView(tu.pull)
	flat := ansi.Strip(view)
	if !strings.Contains(flat, "▸") || !strings.Contains(flat, "Thinking · ") {
		t.Fatalf("want collapsed thinking line:\n%s", flat)
	}
	if strings.Contains(flat, "deep thought") {
		t.Fatalf("collapsed thinking leaked content:\n%s", flat)
	}
	if len(spans) != 1 || spans[0].id != "think:0" || spans[0].start != 0 {
		t.Fatalf("spans broken: %#v", spans)
	}
	// 点开后全文可见，头部行仍在块首（热区不丢）
	tu.expand.toggle("think:0")
	view, _ = tu.composeView(tu.pull)
	flat = ansi.Strip(view)
	if !strings.Contains(flat, "▾") || !strings.Contains(flat, "deep thought") {
		t.Fatalf("expanded thinking broken:\n%s", flat)
	}
	if line0 := strings.Split(flat, "\n")[0]; !strings.Contains(line0, "Thinking") {
		t.Fatalf("expanded header must be line 0: %q", line0)
	}
}

func TestThinkingDeltaAndFinalizeNoDuplicate(t *testing.T) {
	// 流式 delta 与定稿 message 携带同文思考：只产出一个思考块
	// （尾段伪 ID 被定稿块消费，不出现第二个 span），且展开后全文恰一次。
	thinking := "same reasoning text"
	fake := &fakeView{records: []string{
		engineWireToolRecord(t, "delta", mm("assistant", thinking, "")),
		engineWireToolRecord(t, "message", mm("assistant", thinking, "answer")),
	}}
	tu := NewTui(fake)
	tu.pull.width = 120
	view, spans := tu.composeView(tu.pull)
	if len(spans) != 1 || spans[0].id != "think:1" {
		t.Fatalf("want exactly one thinking span, got %#v", spans)
	}
	tu.expand.toggle("think:1")
	view, _ = tu.composeView(tu.pull)
	if n := strings.Count(ansi.Strip(view), thinking); n != 1 {
		t.Fatalf("thinking rendered %d times, want 1:\n%s", n, ansi.Strip(view))
	}
}

func TestTailExpansionSurvivesStreamingCollapsesAtFinalize(t *testing.T) {
	// 流式思考点开 → 后续重组保持展开（回归：此前每帧 clearTail 会把点开
	// 立刻缩回去）→ 定稿即收起（流式展开是观看辅助，不跨定稿存活——否则
	// 块随重排"跳到上面去"）。清理在 applyFrame 的迁移检测做：重放路径的
	// clearTail 会被历史 message 记录反复触发，抹掉用户刚点开的展开
	// （回归：只有会话第一个思考块能点开）。
	fake := &fakeView{records: []string{
		engineWireToolRecord(t, "delta", mm("assistant", "streaming thought", "")),
	}}
	tu := NewTui(fake)
	tu.pull.width = 120
	view, spans := tu.composeView(tu.pull)
	if len(spans) != 1 || spans[0].id != tailThinkID {
		t.Fatalf("live tail must use pseudo id: %#v", spans)
	}
	tu.expand.toggle(tailThinkID) // 用户在流式期间点开
	tu.spans = spans

	// 仍在流式（新 delta 到达）：展开必须跨重组存活
	fake.records = append(fake.records,
		engineWireToolRecord(t, "delta", mm("assistant", "streaming thought", "")))
	view, spans = tu.composeView(tu.pull)
	if !strings.Contains(ansi.Strip(view), "streaming thought") {
		t.Fatalf("live tail must stay expanded across recomposes:\n%s", ansi.Strip(view))
	}
	if len(spans) != 1 || spans[0].id != tailThinkID {
		t.Fatalf("still-live tail id broken: %#v", spans)
	}
	tu.applyFrame(frameMsg{view: view, spans: spans}) // 帧落地：尾段仍在 → 不清理
	if !tu.expand.expanded(tailThinkID) {
		t.Fatalf("live tail expansion must survive frames while streaming")
	}

	// 定稿到达：收起（不迁移到正式 ID）；清理由帧迁移检测触发
	fake.records = append(fake.records,
		engineWireToolRecord(t, "message", mm("assistant", "streaming thought", "done")))
	view, spans = tu.composeView(tu.pull)
	if strings.Contains(ansi.Strip(view), "streaming thought") {
		t.Fatalf("finalized thinking must be collapsed after finalize:\n%s", ansi.Strip(view))
	}
	if len(spans) != 1 || spans[0].id != "think:2" {
		t.Fatalf("finalized thinking id broken: %#v", spans)
	}
	tu.applyFrame(frameMsg{view: view, spans: spans}) // 帧落地：尾段消失 → 清理
	if tu.expand.expanded(tailThinkID) {
		t.Fatalf("stale tail:think entry leaked into next step")
	}

	// 定稿块重新点开可见
	tu.expand.toggle("think:2")
	view, _ = tu.composeView(tu.pull)
	if !strings.Contains(ansi.Strip(view), "streaming thought") {
		t.Fatalf("expanded finalized thinking lost content")
	}
}

func TestApplyFrameClearsTailOnlyOnConsumption(t *testing.T) {
	// 迁移检测的边界：尾段仍在（重组/新 delta）不清理；消失才清理。
	tu := NewTui(&fakeView{})
	tu.expand.m[tailThinkID] = true

	// 尾段仍在：不清
	tu.spans = []blockSpan{{start: 3, id: tailThinkID}}
	tu.applyFrame(frameMsg{view: "x", spans: []blockSpan{{start: 3, id: tailThinkID}}})
	if !tu.expand.expanded(tailThinkID) {
		t.Fatalf("tail still present must not be cleared")
	}

	// 尾段消失（被定稿/边界消费）：清
	tu.applyFrame(frameMsg{view: "x", spans: []blockSpan{{start: 3, id: "think:1"}}})
	if tu.expand.expanded(tailThinkID) {
		t.Fatalf("tail consumed must be cleared")
	}

	// 旧帧没有尾段：新帧也没有 → no-op
	tu.expand.m[tailThinkID] = true
	tu.applyFrame(frameMsg{view: "x", spans: []blockSpan{{start: 3, id: "think:1"}}})
	if !tu.expand.expanded(tailThinkID) {
		t.Fatalf("no tail in old frame must be a no-op")
	}
}

// truncAnsi 截断到 n 个显示列（剥 ANSI 后），测试诊断输出用。
func truncAnsi(s string, n int) string {
	flat := ansi.Strip(s)
	if len(flat) <= n {
		return flat
	}
	return flat[:n] + "…"
}

// 折叠态单行块必须落在本轮视图宽度内：折叠行不走 hardWrap，超宽会让终端
// 视觉折行、整屏行号错位（点击命中依赖行号对齐）。限宽分 120/40 两档逐行
// 断言（历史 bug：摘要截断固定 60/80 runes、不跟实际宽度走，窄终端必溢出）。
func TestCollapsedLinesWithinWidth(t *testing.T) {
	cases := []struct {
		name  string
		build func(w int) string
	}{
		{"折叠思考行", func(_ int) string {
			return msgIndent.Render(thinkingCollapsedView(strings.Repeat("c", 3000)))
		}},
		{"折叠工具行-长路径", func(w int) string {
			return msgIndent.Render(toolCollapsedView("read", []byte(`{"path":"`+strings.Repeat("/", 200)+`f.go"}`), w))
		}},
		{"折叠工具行-长参数", func(w int) string {
			return msgIndent.Render(toolCollapsedView("CommandRun", []byte(`{"process":"bash","args":["-c","`+strings.Repeat("x", 300)+`"]}`), w))
		}},
	}
	for _, w := range []int{120, 40} {
		for _, c := range cases {
			for i, ln := range strings.Split(c.build(w), "\n") {
				if lw := ansi.StringWidth(ln); lw > w {
					t.Errorf("w=%d %s line %d overflows (%d > %d): %q", w, c.name, i, lw, w, truncAnsi(ln, 60))
				}
			}
		}
	}
}

func TestBoundaryFlushAssignsStableID(t *testing.T) {
	// 尾段在边界记录处冲刷：块 ID 用该记录序号，跨重组稳定（重放每次同 ID）
	fake := &fakeView{records: []string{
		engineWireToolRecord(t, "delta", mm("assistant", "partial think", "")),
		`{"type":"user","text":"next question"}`,
	}}
	tu := NewTui(fake)
	tu.pull.width = 120
	_, spans := tu.composeView(tu.pull)
	if len(spans) != 1 || spans[0].id != "think:1" {
		t.Fatalf("boundary flush id broken: %#v", spans)
	}
	// 重放确定性：再组一次 ID 不变
	_, spans = tu.composeView(tu.pull)
	if spans[0].id != "think:1" {
		t.Fatalf("replay must be deterministic: %#v", spans)
	}
}

func TestApplyFrameAnchorPinsHeaderRow(t *testing.T) {
	tu := NewTui(&fakeView{})
	tu.vp.SetHeight(10)
	tu.vp.SetWidth(80)
	// 假设点击时块头部在屏幕行 2、重组后块首行变成 40：锚定应把 YOffset 设为 38
	tu.anchor = &scrollAnchor{id: "tool:1", row: 2}
	m := frameMsg{
		view:  strings.Repeat("line\n", 100),
		spans: []blockSpan{{start: 40, id: "tool:1"}},
	}
	tu.applyFrame(m)
	if got := tu.vp.YOffset(); got != 38 {
		t.Fatalf("anchor must pin block header to click row: YOffset=%d want 38", got)
	}
	if tu.anchor != nil {
		t.Fatalf("anchor must be consumed")
	}
	// 锚已消费：下一帧不再锚定（走常规底部跟随/位置保持）
	m2 := frameMsg{view: m.view, spans: m.spans}
	tu.applyFrame(m2)
	if got := tu.vp.YOffset(); got != 38 { // 不在底部 → 位置保持
		t.Fatalf("stale anchor must not reapply: YOffset=%d", got)
	}
}

func TestStaleFrameDoesNotConsumeAnchor(t *testing.T) {
	tu := NewTui(&fakeView{})
	tu.vp.SetHeight(10)
	tu.vp.SetWidth(80)
	tu.expand.toggle("tool:1") // 模拟点击后的状态版本
	ver := tu.expand.version()
	tu.anchor = &scrollAnchor{id: "tool:1", row: 2, ver: ver}

	// 点击前已在飞的旧帧（版本更小）：不能消费锚——它落的是点击前排版。
	view := strings.Repeat("line\n", 100)
	spans := []blockSpan{{start: 40, id: "tool:1"}}
	tu.applyFrame(frameMsg{view: view, spans: spans, expandVer: ver - 1})
	if tu.anchor == nil {
		t.Fatalf("stale frame must not consume anchor")
	}

	// 点击后重组的帧（版本 >= 锚）才消费锚。
	tu.vp.SetYOffset(0) // 确保不处于底部，锚定结果不会被底部跟随覆盖
	tu.applyFrame(frameMsg{view: view, spans: spans, expandVer: ver})
	if tu.anchor != nil {
		t.Fatalf("post-click frame must consume anchor")
	}
	if got := tu.vp.YOffset(); got != 38 {
		t.Fatalf("anchor must pin on post-click frame: YOffset=%d want 38", got)
	}
}

func TestExpandedThinkingIndentAligned(t *testing.T) {
	fake := &fakeView{records: []string{
		engineWireToolRecord(t, "message", mm("assistant", "first line\nsecond line", "")),
	}}
	tu := NewTui(fake)
	tu.pull.width = 80
	tu.expand.toggle("think:0")
	view, _ := tu.composeView(tu.pull)
	lines := strings.Split(ansi.Strip(view), "\n")
	if len(lines) < 2 {
		t.Fatalf("expanded thinking too short:\n%s", ansi.Strip(view))
	}
	headCol := strings.Index(lines[0], "▾")
	bodyCol := strings.Index(lines[1], "│")
	if headCol < 0 || bodyCol < 0 {
		t.Fatalf("missing header/body markers: %q / %q", lines[0], lines[1])
	}
	if headCol != bodyCol {
		t.Fatalf("thinking body double-indented: header col %d, body border col %d\n%s",
			headCol, bodyCol, ansi.Strip(view))
	}
}

func TestClearResetsExpandOnlyOnSuccess(t *testing.T) {
	fake := &fakeView{denyClear: true}
	tu := NewTui(fake)
	tu.expand.toggle("think:0")
	tu.anchor = &scrollAnchor{id: "think:0", row: 1, ver: tu.expand.version()}
	tu.forceCompose.Store(false)

	// running 拒绝：历史没清，展开态与锚点必须原样保留。
	tu.dispatchSlash("/clear")
	if !tu.expand.expanded("think:0") {
		t.Fatalf("rejected /clear must not reset expand state")
	}
	if tu.anchor == nil || tu.forceCompose.Load() {
		t.Fatalf("rejected /clear must not touch anchor/forceCompose")
	}

	// 成功：清展开态、清锚、强制立即重组。
	fake.denyClear = false
	tu.dispatchSlash("/clear")
	if tu.expand.expanded("think:0") {
		t.Fatalf("successful /clear must reset expand state")
	}
	if tu.anchor != nil || !tu.forceCompose.Load() {
		t.Fatalf("successful /clear must clear anchor and force recompose")
	}
}

func TestNewResetsExpandOnlyOnSuccess(t *testing.T) {
	// running 拒绝：历史没清，展开态与锚点必须原样保留。
	fake := &fakeView{denyNew: true}
	tu := NewTui(fake)
	tu.expand.toggle("think:0")
	tu.anchor = &scrollAnchor{id: "think:0", row: 1, ver: tu.expand.version()}
	tu.forceCompose.Store(false)

	tu.dispatchSlash("/new")
	if !tu.expand.expanded("think:0") {
		t.Fatalf("rejected /new must not reset expand state")
	}
	if tu.anchor == nil || tu.forceCompose.Load() {
		t.Fatalf("rejected /new must not touch anchor/forceCompose")
	}

	// 成功：/new 自动清屏，展开态、锚点与视图一起重置。
	fake.denyNew = false
	tu.dispatchSlash("/new")
	if tu.expand.expanded("think:0") {
		t.Fatalf("successful /new must reset expand state")
	}
	if tu.anchor != nil || !tu.forceCompose.Load() {
		t.Fatalf("successful /new must clear anchor and force recompose")
	}
}

func TestNewKeepsExpandWhenClearRejected(t *testing.T) {
	// NewConversation 成功但 ClearRecords 被 running 拒绝：历史没清，展开态保留。
	fake := &fakeView{denyClear: true}
	tu := NewTui(fake)
	tu.expand.toggle("think:0")
	tu.anchor = &scrollAnchor{id: "think:0", row: 1, ver: tu.expand.version()}
	tu.forceCompose.Store(false)

	tu.dispatchSlash("/new")
	if !tu.expand.expanded("think:0") {
		t.Fatalf("rejected /clear must not reset expand state")
	}
	if tu.anchor == nil || tu.forceCompose.Load() {
		t.Fatalf("rejected /clear must not touch anchor/forceCompose")
	}
}

func TestCheckoutSessionResetsExpand(t *testing.T) {
	tu := NewTui(&fakeView{})
	tu.expand.toggle("think:0")
	tu.anchor = &scrollAnchor{id: "think:0", row: 1, ver: tu.expand.version()}
	tu.forceCompose.Store(false)

	if !tu.checkoutSession("session-1") {
		t.Fatalf("checkout must succeed")
	}
	if tu.expand.expanded("think:0") {
		t.Fatalf("checkout must reset expand state")
	}
	if tu.anchor != nil || !tu.forceCompose.Load() {
		t.Fatalf("checkout must clear anchor and force recompose")
	}
}
