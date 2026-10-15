package tui

// ── 引擎交互层 ───────────────────────────────────────────
//
// 本包对引擎的全部访问都集中在这里——grep "t.engine" 只应命中本文件。
// 上下游零 import：跨界只有 stdlib 类型与 JSON 文本，wire JSON 形状定义在
// model.go，这里的 fetch* 负责拉取与解析、语义在方法注释上自描述。
// 拉取类在 pull 链的 goroutine 上调用（引擎侧自带锁），提交/中断类在
// 事件循环上调用——两者都不碰 model 字段。

import (
	"encoding/json"
	"time"
)

// EngineView 是 TUI 对引擎的全部依赖（消费方定义的接口）。
// 引擎侧的 *Engine 天然满足本接口；控制权全部在 TUI：状态由 TUI 按帧
// 拉取，输入/取消由 TUI 主动调用。
type EngineView interface {
	Version() uint64
	Records() []string // 消息日志全量（每条一行 JSON，type 字段承载消息类型）
	RunStateJSON() string
	TodoText() string
	NoticeJSON() string
	StartupInfo() ([]string, bool)
	SubmitInput(line string) bool
	Interrupt() bool
	// ── 斜杠命令的能力接口（命令注册表与分发都在前端 commands.go）──
	Shutdown()                       // /exit：释放 runner/MCP/localexec，前端随后自行退出
	NewConversation() bool           // /new：running 时拒绝（false）
	ClearRecords() bool              // /clear：running 时拒绝（false）
	ListResumeItems() string         // /resume 候选 JSON（"[]"=无可恢复，引擎已附提示）
	CheckoutSession(id string) error // 提交弹框选择；失败弹框保持可重选
}

// noticeTTL 临时通知的停留时长，到期回落兜底提示。TTL 从引擎写入槽位的
// setAt 起算（契约：写入那一刻即计时起点）。
const noticeTTL = 4 * time.Second

// fetchRunState 拉取并解析运行状态快照（running + fatal）。
// 解析失败返回 false（坏帧不致盲：调用方等下一轮重拉）。
func (t *TUI) fetchRunState() (wireRunState, bool) {
	var rs wireRunState
	if err := json.Unmarshal([]byte(t.engine.RunStateJSON()), &rs); err != nil {
		return wireRunState{}, false
	}
	return rs, true
}

// fetchStartupInfo 拉取启动横幅信息行；引擎尚未就绪时第二返回值为 false
// （调用方应等下一轮再拉——横幅只在数据齐了之后出现一次）。
func (t *TUI) fetchStartupInfo() ([]string, bool) {
	return t.engine.StartupInfo()
}

// fetchVersion 拉取消息日志版本号（任何记录追加都会 +1）。
func (t *TUI) fetchVersion() uint64 { return t.engine.Version() }

// fetchRecords 拉取全部消息日志记录（每条一行 JSON，按写入顺序）。
func (t *TUI) fetchRecords() []string { return t.engine.Records() }

// fetchTodoText 拉取当前清单文本（多行纯文本，空串=无清单）。
func (t *TUI) fetchTodoText() string { return t.engine.TodoText() }

// fetchNotice 组装底部通知文本：槽位在 TTL 内则显示通知（renderNotice 上色），
// 否则回落常驻 hint（运行态多一条 esc 提示）。
func (t *TUI) fetchNotice(running bool) string {
	var wn wireNotice
	if err := json.Unmarshal([]byte(t.engine.NoticeJSON()), &wn); err == nil {
		if setAt, err := time.Parse(time.RFC3339, wn.SetAt); err == nil &&
			wn.Kind != "none" && time.Since(setAt) < noticeTTL {
			return renderNotice(wn.Kind, wn.Text)
		}
	}
	return composeHint(running)
}

// submitInput 提交一行用户输入。引擎忙（无人接收）时返回 false，
// 调用方应保留输入框内容——用户输入永不丢失。
func (t *TUI) submitInput(line string) bool { return t.engine.SubmitInput(line) }

// interrupt 提交一次中断信号（运行期 Esc）。非运行期无接收方，返回
// false 可安全忽略。
func (t *TUI) interrupt() bool { return t.engine.Interrupt() }

// openResume /resume：同步拉候选开弹框（数据在手，无需槽位/逐帧拉取）；
// 空列表不开（引擎侧已附 notice），弹框显隐与选中态由前端自持。
func (t *TUI) openResume() {
	var items []wireResumeItem
	if json.Unmarshal([]byte(t.engine.ListResumeItems()), &items) != nil {
		return
	}
	if len(items) == 0 {
		return
	}
	t.resume.setItems(items)
	t.resume.visible = true
}

// checkoutSession 提交弹框选择（enter 选中后调用）。错误引擎侧已自行
// 记录（appendTyped/notice），调用方无需处理；true=成功（据此关框），
// false=失败（弹框保持可重选）。
func (t *TUI) checkoutSession(id string) bool { return t.engine.CheckoutSession(id) == nil }
