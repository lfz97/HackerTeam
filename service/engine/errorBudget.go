package engine

import "time"

// 错误自动重试预算：策略与状态的自洽单元，供 Run 主循环、turn 重试循环与
// agentRunOnce 使用。

const (
	// 连续错误自动重试策略的默认值（构造 errorBudget 时注入）。将来要按
	// 实例/配置调整，把这两个值提升为 Engine 字段或 yaml 配置即可。
	defaultErrorMaxTimes = 3
	defaultErrorSleepGap = 3 * time.Second
)

// errorBudget 连续错误的自动重试预算：策略（max/gap）与状态（streak）同体。
// 状态流转全部收在方法里，调用方只表达意图，不裸改字段；耗尽后不自动清零，
// 等用户输入充值（各充值点的触发原因见调用处注释）：
//   - canAutoRetry：错误后判断还有没有自动重试资格
//   - fail：记一次失败，返回次数与是否耗尽
//   - recharge：归零。触发点：用户手动提交输入 / 流式吐 token 的健康证据 /
//     /new 新对话 / 中断与正常结束的兜底
type errorBudget struct {
	streak int
	max    int           //连续失败自动重试上限
	gap    time.Duration //重试退避间隔
}

func newErrorBudget(max int, gap time.Duration) errorBudget {
	return errorBudget{max: max, gap: gap}
}

func (b *errorBudget) canAutoRetry() bool { return b.streak < b.max }

func (b *errorBudget) fail() (n int, exhausted bool) {
	b.streak++
	return b.streak, b.streak >= b.max
}

func (b *errorBudget) recharge() { b.streak = 0 }

// limit 自动重试上限（提示文案用）。
func (b *errorBudget) limit() int { return b.max }

// backoff 重试退避间隔（Run 循环 sleep 用）。
func (b *errorBudget) backoff() time.Duration { return b.gap }
