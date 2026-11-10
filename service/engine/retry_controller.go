package engine

import "time"

// 重试控制器：错误预算 + 重试期观察态 + 取消意图，三块状态同体、
// 经 e.mu 守护（与其他引擎状态同一临界区），供 SubmitInput 的充值点、
// turn 重试循环、agentRunOnce 与 Interrupt 使用。

const (
	// 连续错误自动重试策略的默认值（构造时注入）；将来要按配置调整，提升为 Engine 字段或 yaml 项即可。
	defaultErrorMaxTimes = 3
	defaultErrorSleepGap = 3 * time.Second
)

// retryController 重试控制器。
//
// 状态的读写一律经 Engine 侧的锁包装（newTurnRetry/retryFail/retryRecharge/
// retryBackoff/retryLimit/Interrupt/consumeCancelRequested/endTurn，均持 e.mu）：
// 结构体方法不自带锁——RunStateJSON 的 TUI 拉取 goroutine 会并发读 retrying，
// 无锁直调即数据竞争。
//
// 生命周期：**每轮 turn 由 SubmitInput 重铸**（占坑后同步执行，跨轮零残留——
// streak、观察态、取消意图一律不跨轮）。轮内流转：
//   - fail()（未耗尽）→ retrying=true，进入退避阶段；
//   - recharge()（流式健康证据）→ streak=0、retrying=false（不动取消意图——
//     流式期间按 Esc 后到来的无关事件不应吞掉取消意图）；
//   - endTurn() → resetTransient() 清 retrying+cancelRequested：跨轮泄漏已被
//     重铸根除，这里的清场只为观察态正确（idle 期间不残留 retrying=true 的
//     错误显示）与防御深度。
//
// cancelRequested 是"取消意图"粘滞位，置位在 Interrupt（running 即写，
// 无条件——不用 retrying 做写入门槛）：主动取消（Cancel 杀活跃 run）后的
// 完成事件投递会被已取消的 ctx 掐断（EmitEventWithTimeout 实测 WARN），
// NoticeCancelled 的可靠兜底全靠 turn 收尾消费此标志，门槛化会断链
// （TestEscInterruptEndToEnd 锁定该链路）。消费点：退避 tick、Run 边界、
// turn 成功收尾（读即清）。
//
// retrying 是纯观察态：标识退避/重试阶段（RunStateJSON 暴露给 TUI，
// 对应 pi 的 auto_retry_start 语义），不做任何判定的门槛。
type retryController struct {
	streak int           //连续失败计数（充值归零）
	max    int           //连续失败自动重试上限
	gap    time.Duration //重试退避间隔

	retrying        bool //观察态：处于退避/重试阶段
	cancelRequested bool //取消意图（置位见 Interrupt，消费见类型注释）
}

func newRetryController(max int, gap time.Duration) retryController {
	return retryController{max: max, gap: gap}
}

func (rc *retryController) canAutoRetry() bool { return rc.streak < rc.max }

// fail 记一次失败并决定是否继续自动重试；进入退避即置 retrying。
func (rc *retryController) fail() (n int, exhausted bool) {
	rc.streak++
	rc.retrying = rc.streak < rc.max
	return rc.streak, rc.streak >= rc.max
}

// recharge 健康证据充值：预算归零、重试期结束（不动 cancelRequested——
// 流式期间按 Esc 后到来的无关事件不应吞掉取消意图）。
func (rc *retryController) recharge() {
	rc.streak = 0
	rc.retrying = false
}

// resetTransient 每轮 turn 收尾清瞬态（retrying/cancelRequested），预算不动。
func (rc *retryController) resetTransient() {
	rc.retrying = false
	rc.cancelRequested = false
}

// limit 自动重试上限（提示文案用）。
func (rc *retryController) limit() int { return rc.max }

// backoff 重试退避间隔（退避 sleep 用）。
func (rc *retryController) backoff() time.Duration { return rc.gap }
