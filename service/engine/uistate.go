package engine

// 本文件是引擎对上层 UI 暴露的可观察状态与方法（pull + JSON 契约）。
// 上下游零 import：跨界只有 stdlib 类型与 JSON 文本，消息类型放在 JSON 的
// type/style/kind 字段上，schema 自描述。
//
// 消息日志每条记录是一行 JSON：
//   {"type":"delta","msg":{...框架 model.Message 原样...}}   流式增量
//   {"type":"message","msg":{...框架 model.Message 原样...}} 完整消息（定稿/工具声明/工具结果）
//   {"type":"user","text":"..."} / {"type":"slash","text":"..."}
//   {"type":"warn","text":"..."} / {"type":"error","text":"..."} / {"type":"summary","text":"..."}
// 引擎只透传框架原始 message（model.Message 自带 json 标签，可直接序列化），
// 怎么渲染、怎么解释是 UI 侧的事。

import (
	"encoding/json"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

// 记录 type 字段取值。
const (
	RecDelta   = "delta"   // 流式增量（msg=框架 message）
	RecMessage = "message" // 完整消息（msg=框架 message）
	RecUser    = "user"
	RecSlash   = "slash"
	RecWarn    = "warn"
	RecError   = "error"
	RecSummary = "summary"
)

// 终态 style 字段取值。
const (
	FatalPlain   = "plain"
	FatalError   = "error"
	FatalSuccess = "success"
	FatalExit    = "exit"
)

// 通知 kind 字段取值。
const (
	NoticeNone            = "none"
	NoticeNewConversation = "new_conversation"
	NoticeCancelled       = "cancelled"
	NoticeSuccess         = "success"
	NoticeWarning         = "warning"
)

// Fatal 进程终态（RunStateJSON 里 fatal 对象的形状）。
type Fatal struct {
	Text    string `json:"text"`
	Style   string `json:"style"`   // FatalXxx 常量
	WaitKey bool   `json:"waitKey"` // true=渲染后等用户按任意键再退出
}

// RunState 运行状态快照（幂等，last-write-wins）。
type RunState struct {
	Running bool   `json:"running"`
	Fatal   *Fatal `json:"fatal"` // 非 nil 表示进程应结束
}

// SkillItem 帮助页的一行（JSON 形状 {"cmd","desc"}）。
type SkillItem struct {
	Cmd  string `json:"cmd"`
	Desc string `json:"desc"`
}

// notice 通知栏槽位内容。SetAt 供消费端计算 TTL（旧契约 4s，语义不变）。
type notice struct {
	Kind  string    `json:"kind"`
	Text  string    `json:"text"`
	SetAt time.Time `json:"setAt"`
}

// ── 消息日志 ─────────────────────────────────────────────

// Version 返回全局单调版本号：任何记录追加都会 +1。
// 消费端每帧比对版本号，有变化才拉取增量——空闲时零成本。
func (e *Engine) Version() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.version
}

// Records 返回全部消息日志记录（每条一行 JSON，按写入顺序）。
func (e *Engine) Records() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.records...)
}

// appendRecord 追加一条已序列化的 JSON 记录（引擎内部写入口）。
func (e *Engine) appendRecord(rawJSON string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, rawJSON)
	e.version++
}

// appendTyped 追加一条 {"type":typ,"text":...} 形状的记录（引擎内部文本事件用）。
func (e *Engine) appendTyped(typ, text string) {
	b, err := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}{typ, text})
	if err != nil {
		return // 纯 string 字段不会失败，防御性兜底
	}
	e.appendRecord(string(b))
}

// AppendSummary 追加一条摘要提示（session.summarySink 消费方小接口的实现）。
func (e *Engine) AppendSummary(text string) { e.appendTyped(RecSummary, text) }

// AppendRecordJSON 追加一条已序列化的 JSON 记录（emitChoice 的透传序列化产物走这里）。
func (e *Engine) AppendRecordJSON(raw string) { e.appendRecord(raw) }

// wireRecord 消息记录的 JSON 形状：type 承载消息类型，msg 为框架 message 原样。
type wireRecord struct {
	Type string         `json:"type"`
	Msg  *model.Message `json:"msg,omitempty"`
}

func wireMsg(typ string, m model.Message) string {
	b, err := json.Marshal(wireRecord{Type: typ, Msg: &m})
	if err != nil {
		return "" // 纯 string/json 字段的 message 不会 marshal 失败，防御性兜底
	}
	return string(b)
}

// emitChoice 把框架 Response choice 透传序列化进消息日志——引擎只传原始 message
// （model.Message 自带 json 标签），增量/完整靠 type 字段区分；怎么渲染是 UI 的事。
// ShowReasoning=false 时剥掉 reasoning_content（配置属于引擎，UI 无需感知）。
// 空增量/空定稿不入日志（只 bump 版本没有任何意义）。
func (e *Engine) emitChoice(choice model.Choice, isPartial bool) {
	if (*(*e).AgentRunner_p).Stream {
		d := choice.Delta
		if !(*(*e).Config_p).Model.ShowReasoning {
			d.ReasoningContent = ""
		}
		if d.Content != "" || d.ReasoningContent != "" || len(d.ToolCalls) != 0 || d.Role == "tool" {
			e.AppendRecordJSON(wireMsg("delta", d))
		}
	}
	if !isPartial {
		m := choice.Message
		if !(*(*e).Config_p).Model.ShowReasoning {
			m.ReasoningContent = ""
		}
		if strings.TrimSpace(m.Content) != "" || len(m.ToolCalls) != 0 || m.Role == "tool" {
			e.AppendRecordJSON(wireMsg("message", m))
		}
	}
}

// ── 运行状态 ─────────────────────────────────────────────

// RunStateJSON 返回运行状态快照 JSON（{"running":bool,"fatal":{...}|null}）。
func (e *Engine) RunStateJSON() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := json.Marshal(e.runState)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// setRunning 标记 agent 是否在运行（指示器/提示条/Esc 行为的依据）。
func (e *Engine) setRunning(running bool) {
	e.mu.Lock()
	e.runState.Running = running
	e.mu.Unlock()
}

// setFatal 设置进程终态（配套 parkWithFatal 的驻留由调用方完成）。
func (e *Engine) setFatal(style, text string, waitKey bool) {
	e.mu.Lock()
	e.runState.Fatal = &Fatal{Text: text, Style: style, WaitKey: waitKey}
	e.mu.Unlock()
}

// parkWithFatal 置进程终态并永久驻留当前 goroutine。
// 旧版等价物是 tui 的 Show*InMsgViewAndExit 内部末尾的 select{}，语义必须原样保留：
//   - 引擎 init 序列打完致命消息后绝不能带着未初始化状态继续往下跑（loadConfig 失败
//     时 Config_p 仍是 nil，继续走 init 会在别处 panic）；
//   - 不能在 TUI 回调里 os.Exit——bubbletea 的终端复原在其 Run() 返回路径上，
//     硬退出会把终端留在 alt-screen + raw mode。
//
// pull 之后的分工：引擎置终态 + 驻留；渲染、等待按键、停循环由 TUI 完成。
func (e *Engine) parkWithFatal(style string, text string, waitKey bool) {
	e.setFatal(style, text, waitKey)
	select {}
}

// ── todo 清单 ────────────────────────────────────────────

// TodoText 返回当前清单文本（多行纯文本，空串=无清单）。
func (e *Engine) TodoText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.todoText
}

// SetTodoText 更新清单文本（满足 status.go 的 todoTextSink 消费方小接口）。
// ⚠️ 空串也必须写入：清单全部完成后框架会返回 ""，消费端靠空串把清单栏塌回 0 行。
func (e *Engine) SetTodoText(text string) {
	e.mu.Lock()
	e.todoText = text
	e.mu.Unlock()
}

// ── 通知栏槽位 ───────────────────────────────────────────

// setNotice 写入通知栏槽位（瞬时通知；文案原样存储，配色由消费端按 kind 决定）。
func (e *Engine) setNotice(kind, text string) {
	e.mu.Lock()
	e.notice = notice{Kind: kind, Text: text, SetAt: time.Now()}
	e.mu.Unlock()
}

// NoticeJSON 返回通知栏槽位快照 JSON（{"kind","text","setAt":RFC3339}）。
func (e *Engine) NoticeJSON() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := json.Marshal(e.notice)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ── 静态数据 ─────────────────────────────────────────────

// StartupInfo 返回启动横幅信息行；引擎尚未就绪时第二返回值为 false，
// 消费端应等下一帧再拉（横幅只在数据齐了之后出现一次）。
func (e *Engine) StartupInfo() ([]string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startup, e.startupOK
}

// setStartupInfo 就绪启动横幅信息行（引擎 init 完成后的第一轮调用）。
func (e *Engine) setStartupInfo(lines []string) {
	e.mu.Lock()
	e.startup = lines
	e.startupOK = true
	e.mu.Unlock()
}

// SkillItemsJSON 返回技能帮助项 JSON（[{"cmd","desc"}]；默认项 /new /exit 由消费端自持）。
func (e *Engine) SkillItemsJSON() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.skills == nil {
		return "[]" // nil 切片会 marshal 成 null，契约上空就是 []
	}
	b, err := json.Marshal(e.skills)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// setSkillItems 整体替换技能帮助项（loadSkills 每次刷新都重建）。
func (e *Engine) setSkillItems(items []SkillItem) {
	e.mu.Lock()
	e.skills = append([]SkillItem(nil), items...)
	e.mu.Unlock()
}

// ── 消费端 → 引擎 ────────────────────────────────────────

// SubmitInput 提交一行用户输入。引擎忙（无人接收）时返回 false，
// 消费端（TUI）应保留输入框内容——与旧版 unbuffered chan + default 行为一致。
func (e *Engine) SubmitInput(line string) bool {
	select {
	case e.inputCh <- line:
		return true
	default:
		return false
	}
}

// Interrupt 提交一次中断信号（前端 Esc 时调用）。引擎运行循环读取该通道判定中断：
// 运行期的桥接 goroutine 消费信号并取消当前轮 ctx；非运行期无接收方，返回 false
// 可安全忽略。无缓冲通道保证信号不会跨回合残留。
func (e *Engine) Interrupt() bool {
	select {
	case e.interruptCh <- struct{}{}:
		return true
	default:
		return false
	}
}

// InterruptChan 返回中断信号通道（桥接 goroutine 读取）。
func (e *Engine) InterruptChan() <-chan struct{} { return e.interruptCh }
