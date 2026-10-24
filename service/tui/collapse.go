package tui

// 块折叠与点击命中。
//
// 数据流：composeView 产出 []viewBlock（可折叠块携带稳定 ID），join 时算出
// 每块首行行号（blockSpan）随 frameMsg 回事件循环；点击时按"屏幕行 + 滚动
// 偏移"反查块 ID，切换展开态并触发立即重组。
//
// 块 ID 规则（日志 append-only，记录序号跨重组稳定）：
//   - 思考块  think:<定稿记录序号>；流式尾段用伪 ID tail:think（同一时刻
//     至多一个尾段；定稿即收起——流式展开不跨定稿存活，详见 clearTail）
//   - 工具块  tool:<消耗缓冲的那条结果记录序号>（delta 与定稿谁先到谁消费
//     toolBuf，重放确定性保证同一记录每次重组都命中）
//   - 正文/用户/边界块不可折叠，无 ID。
//
// 展开状态是全系统第一处"事件循环写、pull goroutine 读"的状态，互斥锁保护，
// compose 每块渲染时单点查询。默认折叠：key 不存在=折叠，true=用户点开。
// 状态带单调版本号，帧在 compose 起点记录；点击锚点只认版本不旧于自己的帧
// （frame.go），避免点击前已在飞的旧帧消费锚点。

import (
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tailThinkID 流式尾段思考块的伪 ID。
const tailThinkID = "tail:think"

// argSummaryMaxRunes 折叠工具行参数摘要的截断长度上限；真实预算 =
// min(此值, 本轮视图宽度 - 缩进 - 头部宽)（见 toolArgSummary）。
const argSummaryMaxRunes = 60

// viewBlock compose 产出的一个渲染块。
type viewBlock struct {
	id   string // 稳定 ID；空=不可折叠
	text string
}

// blockSpan 可折叠块在最终视图文本中的位置：块首行行号（0 起）。
// 每次重组重算，随 frameMsg 走，不跨帧持久——任何重组都可能改行号，
// 持久坐标永远是过期数据。
type blockSpan struct {
	start int
	id    string
}

// scrollAnchor 点击折叠切换后的滚动锚定：重组后把目标块头部钉回点击时的
// 屏幕行，避免展开内容把视口顶得跳动（覆盖 applyViewText 的底部跟随）。
type scrollAnchor struct {
	id  string
	row int    // 点击时块头部所在的屏幕行
	ver uint64 // 点击后的展开状态版本；更早的帧不得消费（见 applyFrame）
}

// expandState 思考/工具块的展开状态（默认折叠；true=用户点开）。
type expandState struct {
	mu  sync.Mutex
	m   map[string]bool
	ver uint64 // 每次状态变更 +1；帧记录重组起点的版本，供锚点判新旧
}

func newExpandState() *expandState { return &expandState{m: map[string]bool{}} }

// expanded 单点读（pull goroutine 的 compose 调用）。
func (e *expandState) expanded(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.m[id]
}

// toggle 翻转并返回新状态（事件循环调用）。
func (e *expandState) toggle(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := !e.m[id]
	e.m[id] = v
	e.ver++
	return v
}

// version 当前展开状态版本（事件循环读）。
func (e *expandState) version() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ver
}

// clearTail 清尾段伪 ID 的展开状态。流式期间的展开是"观看辅助"：跨 delta
// 存活（重组不清），尾段被消费时清一次。⚠️ 清理点在 applyFrame 的状态迁移
// 检测（旧帧有 tail:think、新帧没有 = 被消费），**绝不能放在 composeView 的
// 重放路径里**——重放每帧扫全量日志，历史 message/边界记录会反复触发清理，
// 把用户刚点开的展开抹掉（症状：只有会话第一个思考块能点开）。
func (e *expandState) clearTail() {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.m, tailThinkID)
	e.ver++
}

// reset 清空（/new、/clear、/resume 换历史后调用）。
func (e *expandState) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.m = map[string]bool{}
	e.ver++
}

// resetExpand 历史被清空/替换后重置展开状态：清锚点并强制下一轮重组，
// 避免旧视图残留与折叠状态脱节（/new、/clear、/resume 成功路径）。
func (t *TUI) resetExpand() {
	t.expand.reset()
	t.anchor = nil
	t.forceCompose.Store(true)
}

// ── compose 侧：折叠渲染 ────────────────────────────────

// thinkingBlock 思考块：默认折叠成单行摘要（流式期间字符数实时增长）；
// 点开后头部行 + 全文引用盒。头部行恒为块首行（点击热区）。
// renderThinking 自带 msgIndent，展开态只给头部套缩进，否则正文会双重缩进。
func (t *TUI) thinkingBlock(r string, id string, w int) string {
	if !t.expand.expanded(id) {
		return msgIndent.Render(thinkingCollapsedView(r))
	}
	return msgIndent.Render(thinkingExpandedHeader()) + "\n" + renderThinking(r, w)
}

// thinkingCollapsedView 折叠态摘要行：▸ + 字符计数（提示还有内容可展开）。
func thinkingCollapsedView(r string) string {
	return colorText(cFgMostSubtle, "▸ ") + colorText(cFgMuted,
		fmt.Sprintf("Thinking · %d chars", len([]rune(strings.TrimSpace(r)))))
}

// thinkingExpandedHeader 展开态头部行。
func thinkingExpandedHeader() string {
	return colorText(cFgMostSubtle, "▾ ") + colorText(cFgMuted, "Thinking")
}

// toolBlock 工具块：默认折叠成头部行 + 首个参数摘要；点开后完整 toolView。
func (t *TUI) toolBlock(entry *wireToolCall, result string, id string, w int) string {
	if !t.expand.expanded(id) {
		return msgIndent.Render(toolCollapsedView(entry.name, []byte(entry.args), w))
	}
	return msgIndent.Render(toolView(entry.name, []byte(entry.args), result, w))
}

// toolCollapsedView 折叠态工具行：▸ name + 首个参数摘要（路径优先），
// 摘要本身就是"还有内容"的提示。截断预算按本轮视图宽度 w 算。
// 不在折叠行做成功/失败判定：框架没有结构化错误位，枚举错误文本必然
// 漏判/误判；结果内容展开后可见，状态图标只会给出不可靠的暗示。
func toolCollapsedView(name string, args []byte, w int) string {
	head := toolHeader(name, false, w-msgIndentW)
	budget := min(argSummaryMaxRunes, w-msgIndentW-2-ansi.StringWidth(head))
	if s := toolArgSummary(name, args, budget); s != "" {
		head += "  " + colorText(cFgMostSubtle, s)
	}
	return head
}

// toolArgSummary 折叠态参数摘要：目标路径优先，否则首个参数值。
// 折叠行不走 hardWrap（单行块），超宽会让终端视觉折行、整屏行号错位
// （点击命中依赖行号对齐）——budget 由调用方按本轮实际宽度扣掉头部后传入：
// 预算到 0 就不出摘要，宁可缺失不溢出。截断用 ansi.Truncate（显示宽度
// 感知）而非 rune 数：CJK 路径 60 rune 可顶出远超宽度的列数。
func toolArgSummary(name string, args []byte, budget int) string {
	if budget <= 0 {
		return ""
	}
	obj, ok := decodeJSONObject(args)
	if !ok {
		return ""
	}
	if p := argPath(obj); p != "" {
		return ansi.Truncate(compactLine(p), budget, "…")
	}
	skip := codeArgFields[name] // 代码内容字段不进摘要（展开态也不展示）
	for _, r := range argRows(name, args) {
		if r.key != skip {
			return ansi.Truncate(compactLine(r.val), budget, "…")
		}
	}
	return ""
}

// flushTailBlocks 把流式尾段作为独立块上屏（先于边界记录/定稿块）并清空
// 累积。id 是尾段获得块身份的方式：边界冲刷用触发记录的序号（跨重组稳定），
// 重放末尾的 live 尾段用伪 ID（尚无记录可依附）。live 尾段的展开跨重组
// 存活；消费时的清理在 applyFrame 的迁移检测做（这里绝不能清——重放每帧
// 重扫历史记录，见 clearTail 注释）。原先 flushTail 把尾段与边界块拼成
// 一个文本块，拆开后思考尾段才有自己的块身份（可折叠）；拼接语义与原先
// 等价（块间空行分隔）。
func (t *TUI) flushTailBlocks(blocks []viewBlock, tailReasoning, tailContent *strings.Builder, w int, id string) []viewBlock {
	if tailReasoning.Len() > 0 {
		blocks = append(blocks, viewBlock{id: id, text: t.thinkingBlock(tailReasoning.String(), id, w)})
		tailReasoning.Reset()
	}
	if tailContent.Len() > 0 {
		blocks = append(blocks, viewBlock{text: renderBody(tailContent.String(), w)})
		tailContent.Reset()
	}
	return blocks
}

// ── join 与命中 ─────────────────────────────────────────

// composeBlocks 块 join 成视图全文（块间空一行，等价于原先的
// strings.Join(parts, "\n\n")），同时累计可折叠块的首行行号。
func composeBlocks(blocks []viewBlock) (string, []blockSpan) {
	var sb strings.Builder
	var spans []blockSpan
	cur := 0
	for i, b := range blocks {
		if i > 0 {
			sb.WriteString("\n\n") // 行尾换行 + 一个空行
			cur++
		}
		if b.id != "" {
			spans = append(spans, blockSpan{start: cur, id: b.id})
		}
		sb.WriteString(b.text)
		cur += strings.Count(b.text, "\n") + 1
	}
	return sb.String(), spans
}

// headerIDAt 视口内容行 → 可折叠块 ID。只认块首行：头部行是唯一热区，
// 点击正文不触发折叠（避免与文本选择/复制冲突）。
func headerIDAt(spans []blockSpan, line int) (string, bool) {
	for _, s := range spans {
		if s.id != "" && s.start == line {
			return s.id, true
		}
	}
	return "", false
}

// spanByID 按块 ID 查新位置（滚动锚定用）。
func spanByID(spans []blockSpan, id string) (blockSpan, bool) {
	for _, s := range spans {
		if s.id == id {
			return s, true
		}
	}
	return blockSpan{}, false
}

// ── 事件循环侧：点击处理 ─────────────────────────────────

// mouseClickHandler 左键点击命中可折叠块头部 → 切换展开态、设滚动锚、
// 触发绕过节流的立即重组。浮层打开时点击归浮层交互，此处让路。
func (t *TUI) mouseClickHandler(m tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.Button != tea.MouseLeft || t.slashPopup.isVisible() || t.resume.isVisible() {
		return t, nil
	}
	// 视口下方（todo/输入框/底栏）不是消息区：不加守卫时 line = m.Y +
	// YOffset 会算出视口外但仍属全文的行号，上滚回看时点输入框可能误命中
	// 屏幕外某行的块头并设锚拽走滚动位置。贴底时行号必然越过全文末尾
	// 自然扑空，所以常态无感，只有上滚时才咬人——必须有显式守卫。
	if m.Y >= t.vp.Height() {
		return t, nil
	}
	// viewport 竖向从屏幕第 0 行起（draw 里消息区是纵向第一个块，左右
	// padding 只占列不占行）：屏幕行 + 滚动偏移 = 视口内容行。
	line := m.Y + t.vp.YOffset()
	id, ok := headerIDAt(t.spans, line)
	if !ok {
		return t, nil
	}
	t.expand.toggle(id)
	t.anchor = &scrollAnchor{id: id, row: m.Y, ver: t.expand.version()}
	t.forceCompose.Store(true) // 点击反馈要即时：下一轮 pull 绕过 100ms 节流
	return t, nil
}
