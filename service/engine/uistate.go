package engine

// 引擎对上层 UI 暴露的可观察状态与方法（pull + JSON 契约）。
// 上下游零 import：跨界只有 stdlib 类型与 JSON 文本，schema 自描述（type/style/kind 字段）。
// 消息日志每条一行 JSON：{"type":"delta"|"message","msg":框架 model.Message 原样}，
// 或 {"type":"user"|"slash"|"warn"|"error"|"summary","text":"..."}；引擎只透传原始 message，渲染归 UI。

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"HackerTeam/service/engine/config"
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
	NoticeClear           = "cleared"
)

// 引擎启动状态（FirstStartupJSON 的 state 字段取值）。
const (
	BootBooting      = "booting" //bootstrap 检查/装配未完成，消费端继续等下一帧
	BootConfigNeeded = "config"  //首次启动：等待 TUI 配置页提交模型配置
	BootReady        = "ready"   //正常状态：配置就绪（首次提交完成或非首次启动）
)

// Fatal 进程终态（RunStateJSON 里 fatal 对象的形状）。
type Fatal struct {
	Text    string `json:"text"`
	Style   string `json:"style"`   // FatalXxx 常量
	WaitKey bool   `json:"waitKey"` // true=渲染后等用户按任意键再退出
}

// RunState 运行状态快照（幂等，last-write-wins）。
type RunState struct {
	Running  bool   `json:"running"`
	Retrying bool   `json:"retrying"` // 退避/重试阶段（观察态，TUI 可显示"重试中"）
	Fatal    *Fatal `json:"fatal"`    // 非 nil 表示进程应结束
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

// 清空记录。必须递增 version：TUI 按 version != seenVersion 决定是否重组视图，
// 只清 records 不 bump，聊天区不会刷新。
func (e *Engine) clearRecords() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = []string{}
	e.version++
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

// emitChoice 把框架 Response choice 透传序列化进消息日志，增量/完整靠 type 字段区分，
// 怎么渲染是 UI 的事。ShowReasoning=false 时剥掉 reasoning_content；空增量/空定稿不入日志。
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

// RunStateJSON 返回运行状态快照 JSON（{"running":bool,"retrying":bool,"fatal":{...}|null}）。
// retrying 从 retry 控制器合并（同一临界区，避免与 fail/recharge 竞争）。
func (e *Engine) RunStateJSON() string {
	e.mu.Lock()
	rs := RunState{
		Running:  e.runState.Running,
		Retrying: e.retry.retrying,
		Fatal:    e.runState.Fatal,
	}
	e.mu.Unlock()
	b, err := json.Marshal(rs)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// setRunning 标记 agent 是否在运行（指示器/提示条/Esc 行为的依据）。
// 生产路径的置位/复位走 tryClaimRunning/endTurn（单临界区成对操作），
// 本方法保留给测试直接模拟忙碌态。
func (e *Engine) setRunning(running bool) {
	e.mu.Lock()
	e.runState.Running = running
	e.mu.Unlock()
}

// setFatal 设置进程终态（TUI 轮询 runState.Fatal 渲染终态屏，waitKey 决定
// 是否等待按键后退出）。当前唯一运行期调用点是 ApplyFirstStartupConfig
// 的 flush 失败（启动链路的错误走返回值，不进此状态）。
func (e *Engine) setFatal(style, text string, waitKey bool) {
	e.mu.Lock()
	e.runState.Fatal = &Fatal{Text: text, Style: style, WaitKey: waitKey}
	e.mu.Unlock()
}

// parkWithFatal 已删除：唯一调用点 LoadConfig 改为软失败（沿用上一代配置+
// 告警，见 init.go）。goroutine 永久驻留会让 turn 永不结束、输入永久拒绝——
// 对话以整个进程退出收场，比"本轮降级跑在旧配置上"代价大一个量级。
// TUI 运行期的致命路径只剩 ApplyFirstStartupConfig 的 flush 失败：
// setFatal + return（不驻留，TUI 渲染终态），无需 park 形态。

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

// ── 首次启动状态 ──────────────────────────────────────────

// FirstStartupJSON 返回首次启动状态快照 JSON：
// {"state":"booting"|"config"|"ready","fields":{"model":...}}。
// fields 仅在 state=config 时携带（模板 model 块的预填值）。
// 消费端（TUI）每帧拉取，state 离开 booting 即可停止拉取。
func (e *Engine) FirstStartupJSON() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := json.Marshal(struct {
		State  string            `json:"state"`
		Fields map[string]string `json:"fields,omitempty"`
	}{e.bootState, e.firstStartupFields})
	if err != nil {
		return `{"state":"` + e.bootState + `"}`
	}
	return string(b)
}

// setBootState 推进启动状态，并成对写入配置页预填值快照（nil = 清除）。
// bootState 与 firstStartupFields 是同一可观察状态的两个字段，FirstStartupJSON
// （TUI pull goroutine，逐帧）在同一个临界区里成对读取——写入也必须在同一
// 临界区成对完成，否则读取方会看到"状态已切换而快照未就绪/未清除"的中间态。
// 所有对这两个字段的写都走这里；在别处裸写即数据竞争。
func (e *Engine) setBootState(state string, fields map[string]string) {
	e.mu.Lock()
	e.firstStartupFields = fields
	e.bootState = state
	e.mu.Unlock()
}

// templateModelFields 快照模板 model 块为配置页预填值（纯函数）。
// 在进入 config 态前调用并经 setBootState 定格；键集与 engineCore.go 的
// modelFormKeys（提交侧白名单）成对，增删字段需两处同步。
// Config_p 此刻只在引擎 goroutine 上读写，无需加锁。
func templateModelFields(m config.Model) map[string]string {
	return map[string]string{
		"model":   m.Model,
		"baseurl": m.BaseURL,
		"apikey":  m.APIKey,
		"apitype": m.APIType,
	}
}

// bootStateSnapshot 已删除：唯一调用方是原 agentLoop 的横幅 gate，
// 横幅改由 Init 就绪尾段直接组装（先于 BootReady 写入），gate 不复存在。

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

// ── 消费端 → 引擎 ────────────────────────────────────────

// SubmitInput 提交一行用户输入：空闲则同步占坑并异步发起一轮 turn（直通 runner）；
// 忙碌（turn 进行中，含重试退避）返回 false，消费端（TUI）应保留输入框内容。
// 空输入同样返回 false（拒绝）：TUI 侧已挡空（handler.go），此处是防御；
// 不充值预算——错误预算的充值语义只有两源（真实用户输入、流式健康证据），
// 意外的空提交不是任何证据，静默充值会让耗尽的预算假装健康。
// 占坑必须同步发生在 go turn 之前：engine 侧标志位是唯一的提交门——runner 的
// 注册表要等 Run 深处 registerRun 才落账（实测注册前窗口 <3ms，见
// window_measure_test.go），靠查 runner 判忙存在双放行窗口。
// 斜杠命令不经过这里：语义与注册表在前端（service/tui/commands.go），后端只暴露能力接口。
func (e *Engine) SubmitInput(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false //空输入拒绝：占坑/充值/turn 一概不发生
	}
	if !e.tryClaimRunning() {
		return false
	}
	// 每轮重铸重试控制器：构造期不注入、跨轮零残留（streak/观察态/取消意图
	// 全部归零）。必须与占坑同序同步执行——重铸若发生在 turn 内（异步），
	// 落在"占坑完成→重铸完成"间隙的 Esc 会写在旧控制器上被重铸丢弃。
	// 与 Interrupt 同在 TUI 事件循环（单 goroutine），此处不存在交错。
	e.newTurnRetry()
	// 注：原"用户输入充值预算"随每轮重铸消解——新控制器 streak 本就归零；
	// 预算充值语义只剩轮内一源（agentRunOnce 的流式健康证据）。
	(*e).appendTyped(RecUser, line)
	// requestID 请求粒度，调用方铸造：一次用户请求一个 ID，错误重试沿用
	// （见 turn 注释），事件流/Langfuse 可整轮归因。同步铸好并记账——
	// Interrupt 从提交时刻起就有可 Cancel 的目标，不依赖 turn 的起跑时序。
	// HackerTeam 的 RequestId 字段继续承载这个请求级追踪标签（区别于 HyperBot
	// v3 删除该字段）：每次提交刷新，经 agent.WithRequestID 盖进本轮每条事件。
	reqID := uuid.NewString()
	(*e).setCurRequestID(reqID)
	go e.turn(line, reqID)
	return true
}

// Interrupt 提交一次中断（前端 Esc 时调用）。两级生效：
//  1. runner.Cancel(curRequestID)——run 已注册（绝大多数时刻），框架内传播取消；
//  2. retry.cancelRequested 取消意图——Esc 落在 runner 注册表空窗（注册前
//     窗口或重试退避间隙，Cancel 查不到）时由退避 tick / Run 边界 / turn
//     成功收尾消费（消费点分析见 retry_controller.go）。
//
// 非运行期返回 false 可安全忽略。返回值语义是"信号已接收"，不是"已停止"——
// 停止判定只认 TUI 帧拉取的 running 状态（Esc 丢失于空窗时指示器继续转，
// 用户再按一次即命中已注册的 run，无需显式重试逻辑）。
// cancelRequested 无条件置位（不以 retrying 做门槛）：主动取消的完成事件
// 投递会被已取消 ctx 掐断，NoticeCancelled 兜底链全靠此标志（见 turn 注释）。
func (e *Engine) Interrupt() bool {
	e.mu.Lock()
	if !e.runState.Running {
		e.mu.Unlock()
		return false
	}
	e.retry.cancelRequested = true
	id := e.curRequestID
	e.mu.Unlock()

	if id != "" && (*e).AgentRunner_p != nil && (*(*e).AgentRunner_p).Runner != nil {
		(*(*e).AgentRunner_p).Runner.Cancel(id) //注册表命中则立即生效；miss 交给取消意图
	}
	return true
}

// ── turn 生命周期状态机（curRequestID / cancelRequested / running 的唯一入口）──

// tryClaimRunning 原子地"查忙+占坑"：check-then-set 收在同一临界区，
// SubmitInput 的提交门因此无竞态（调用方仅 TUI 单事件循环，这里仍收敛成
// 原子操作——防御未来出现第二调用方）。
func (e *Engine) tryClaimRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runState.Running {
		return false
	}
	e.runState.Running = true
	return true
}

// endTurn 收尾 turn：清 retry 瞬态（防 idle 期残留 retrying 的错误显示）并复位
// running，同一临界区。跨轮泄漏已由 SubmitInput 的每轮重铸根除，这里的同临界区
// 是防御深度（重铸前若有任何直写路径残留，收尾清场兜底）。
func (e *Engine) endTurn() {
	e.mu.Lock()
	e.retry.resetTransient()
	e.runState.Running = false
	e.mu.Unlock()
}

// consumeCancelRequested 读取并清除取消意图（退避 tick、Run 边界、turn
// 成功收尾三个消费点共用；读即清）。
func (e *Engine) consumeCancelRequested() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	requested := e.retry.cancelRequested
	e.retry.cancelRequested = false
	return requested
}

// ── retry 状态的引擎级锁入口（结构体方法不自带锁，理由见 retry_controller.go）──

// newTurnRetry 重铸本轮重试控制器：策略取默认值，一切状态归零。
// 调用点仅 SubmitInput（占坑后同步执行，理由见其注释）。
func (e *Engine) newTurnRetry() {
	e.mu.Lock()
	e.retry = newRetryController(defaultErrorMaxTimes, defaultErrorSleepGap)
	e.mu.Unlock()
}

func (e *Engine) retryFail() (n int, exhausted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retry.fail()
}

func (e *Engine) retryRecharge() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.retry.recharge()
}

func (e *Engine) retryBackoff() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retry.backoff()
}

func (e *Engine) retryLimit() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retry.limit()
}

// setCurRequestID 记账当前请求的 requestID（请求粒度，错误重试沿用）。
// 同步写 RequestId：HackerTeam 保留的请求级追踪标签与中断目标始终一致。
func (e *Engine) setCurRequestID(id string) {
	e.mu.Lock()
	e.curRequestID = id
	e.RequestId = id
	e.mu.Unlock()
}

// ── 会话恢复弹框 ──────────────────────────────────────────

// ResumeItem 会话恢复候选列表的一行（JSON 形状，消费端见 service/tui）。
// 数据经 ListResumeItems 同步拉取，引擎不持有弹框 UI 状态。
type ResumeItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`     // 会话内首条用户输入（截断）
	UpdatedAt string `json:"updatedAt"` // 已格式化 "01-02 15:04"
	Current   bool   `json:"current"`   // 是否当前会话
}

// isRunning 是否有对话在跑（turn 内/重试中）。命令类接口据此拒绝并发操作。
func (e *Engine) isRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runState.Running
}
