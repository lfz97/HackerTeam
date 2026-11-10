package engine

// 直通式 SubmitInput/Interrupt 的状态机测试（fake ManagedRunner，不触框架与网络）。
// 覆盖：忙碌门、Cancel 按当前 requestID 精确命中、注册表空窗的取消意图
// 在 Run 边界消费、endTurn 单临界区防取消意图泄漏到下一轮。

import (
	"context"
	"sync"
	"testing"
	"time"

	"HackerTeam/service/engine/config"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// fakeManagedRunner 记录 Run 的 requestID 与 Cancel 调用；事件通道由测试方
// 关闭以结束 turn。runCalled 每次 Run 发信号（带缓冲，不阻塞 turn）。
type fakeManagedRunner struct {
	mu        sync.Mutex
	runIDs    []string
	messages  []string // 每次 Run 收到的 invocation 消息内容
	cancelled []string
	runCalled chan struct{}
	cur       chan *event.Event
}

func newFakeManagedRunner() *fakeManagedRunner {
	return &fakeManagedRunner{runCalled: make(chan struct{}, 8)}
}

func (f *fakeManagedRunner) Run(
	_ context.Context, _ string, _ string, msg model.Message, opts ...agent.RunOption,
) (<-chan *event.Event, error) {
	var ro agent.RunOptions
	for _, o := range opts {
		o(&ro)
	}
	ch := make(chan *event.Event)
	f.mu.Lock()
	f.runIDs = append(f.runIDs, ro.RequestID)
	f.messages = append(f.messages, msg.Content)
	f.cur = ch
	f.mu.Unlock()
	f.runCalled <- struct{}{}
	return ch, nil
}

func (f *fakeManagedRunner) Close() error { return nil }

func (f *fakeManagedRunner) Cancel(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, id)
	return true
}

func (f *fakeManagedRunner) RunStatus(string) (runner.RunStatus, bool) {
	return runner.RunStatus{}, false
}

func (f *fakeManagedRunner) lastRunID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.runIDs) == 0 {
		return ""
	}
	return f.runIDs[len(f.runIDs)-1]
}

func (f *fakeManagedRunner) cancelIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cancelled...)
}

func (f *fakeManagedRunner) runCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runIDs)
}

// closeCurrent 关闭当前事件通道（模拟 run 正常收尾），turn 的抽干循环随之结束。
func (f *fakeManagedRunner) closeCurrent() {
	f.mu.Lock()
	ch := f.cur
	f.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// sendCurrent 向当前事件通道发一个事件后关闭（模拟带 terminal error 的收场）。
func (f *fakeManagedRunner) sendCurrent(evt *event.Event) {
	f.mu.Lock()
	ch := f.cur
	f.mu.Unlock()
	if ch != nil {
		ch <- evt
		close(ch)
	}
}

func (f *fakeManagedRunner) sentMessages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.messages...)
}

func newDirectSubmitEngine(fake *fakeManagedRunner) *Engine {
	e := GetEngineService("test") // retry 等策略字段就位
	e.Config_p = &config.Config{}
	e.AgentRunner_p = &Agentrunner{Runner: fake}
	return e
}

func waitIdle(t *testing.T, e *Engine) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !e.isRunning() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("turn 未在 2s 内结束（running 仍为 true）")
}

func TestSubmitInputGateBusyAndIdle(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	if !e.SubmitInput("hello") {
		t.Fatal("空闲时必须接受提交")
	}
	<-fake.runCalled //等 turn 真正发起 Run（占坑同步、起跑异步）
	if e.SubmitInput("second") {
		t.Fatal("turn 进行中必须拒绝提交")
	}
	fake.closeCurrent()
	waitIdle(t, e)

	// 结束后恢复可提交
	if !e.SubmitInput("again") {
		t.Fatal("turn 结束后必须恢复可提交")
	}
	<-fake.runCalled
	fake.closeCurrent()
	waitIdle(t, e)

	if got := fake.runCount(); got != 2 {
		t.Fatalf("Run 调用次数=%d, want 2", got)
	}
	// 用户输入两条都落了记录（挪自原 agentLoop 的职责）
	if got := len(e.Records()); got != 2 {
		t.Fatalf("用户记录条数=%d, want 2", got)
	}
}

func TestSubmitInputEmptyRejected(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	if e.SubmitInput("") {
		t.Fatal("空输入必须拒绝（return false）")
	}
	if e.SubmitInput("   ") {
		t.Fatal("纯空白输入同上")
	}
	if e.isRunning() {
		t.Fatal("空输入不得占坑")
	}
	if got := fake.runCount(); got != 0 {
		t.Fatalf("空输入不得触发 Run，got %d", got)
	}
	// 空输入不得动预算：预算充值语义只剩"流式健康证据"一源（轮内），
	// 意外的空提交不是任何证据——静默充值会让耗尽的预算假装健康。
	e.mu.Lock()
	e.retry.streak = 2
	e.mu.Unlock()
	if e.SubmitInput("") {
		t.Fatal("空输入必须拒绝")
	}
	e.mu.Lock()
	streak := e.retry.streak
	e.mu.Unlock()
	if streak != 2 {
		t.Fatalf("空输入不得动预算: streak=%d, want 2", streak)
	}
}

func TestInterruptCancelsCurrentRequestID(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	e.SubmitInput("hello")
	<-fake.runCalled // turn 已发起 Run，requestID 已记账

	if !e.Interrupt() {
		t.Fatal("运行期 Interrupt 必须返回 true")
	}
	ids := fake.cancelIDs()
	if len(ids) != 1 || ids[0] != fake.lastRunID() {
		t.Fatalf("Cancel 收到 %v, want [%s]（当前 requestID）", ids, fake.lastRunID())
	}
	// 取消信号已发；关通道收尾
	fake.closeCurrent()
	waitIdle(t, e)
}

func TestPendingCancelConsumedAtRunBoundary(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	// 模拟注册表空窗的 Esc：已占坑但 requestID 未铸（SubmitInput 后、首个 Run 前）
	e.tryClaimRunning()
	if !e.Interrupt() {
		t.Fatal("空窗期 Interrupt 应返回 true（信号已接收）")
	}
	if got := len(fake.cancelIDs()); got != 0 {
		t.Fatalf("无 requestID 时不该调 Cancel, got %d", got)
	}
	e.turn("hello", "req-pending-cancel") // 同步跑：第一个 Run 边界应消费取消意图

	if got := fake.runCount(); got != 0 {
		t.Fatal("取消意图必须在 Run 之前生效（不得发起 Run）")
	}
	if got := noticeKind(t, e); got != NoticeCancelled {
		t.Fatalf("notice=%q, want %q", got, NoticeCancelled)
	}
	if e.isRunning() {
		t.Fatal("turn 结束后 running 必须复位")
	}
}

func TestPendingCancelNotLeakedToNextTurn(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	// 占坑 → 空窗 Esc（置 pending）→ turn 无人消费即收尾（endTurn 清意图）
	e.tryClaimRunning()
	e.Interrupt()
	e.endTurn()

	// 下一轮输入不得被上一轮残留的取消意图误杀
	if !e.SubmitInput("next") {
		t.Fatal("收尾后必须可提交")
	}
	<-fake.runCalled
	fake.closeCurrent()
	waitIdle(t, e)

	if got := fake.runCount(); got != 1 {
		t.Fatalf("下一轮必须真实发起 Run, got %d", got)
	}
	if got := noticeKind(t, e); got == NoticeCancelled {
		t.Fatal("取消意图泄漏：下一轮被误取消")
	}
}

func TestRetryIsCleanEmptyInvocation(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	if !e.SubmitInput("Q1") {
		t.Fatal("空闲时必须接受提交")
	}
	<-fake.runCalled // 第一次 Run 已发起

	// 压缩退避间隔（重铸已随 SubmitInput 同步完成，改 gap 只影响本轮节奏）
	e.mu.Lock()
	e.retry.gap = 5 * time.Millisecond
	e.mu.Unlock()

	// 第一次 Run 以 terminal error 收场（零 assistant 产出）
	fake.sendCurrent(&event.Event{Response: &model.Response{
		Done:  true,
		Error: &model.ResponseError{Type: "api_error", Message: "boom"},
	}})

	<-fake.runCalled // 重试的第二次 Run
	fake.closeCurrent()
	waitIdle(t, e)

	msgs := fake.sentMessages()
	if len(msgs) != 2 {
		t.Fatalf("Run 次数=%d, want 2（首次+重试）: %v", len(msgs), msgs)
	}
	if msgs[0] != "Q1" {
		t.Fatalf("首次 invocation=%q, want 原始输入", msgs[0])
	}
	if msgs[1] != "" {
		t.Fatalf("重试 invocation=%q, want 空消息（干净重试，不喂错误详情）", msgs[1])
	}
	// 错误已记录（交还用户可读），错误详情没有变成发给模型的 prompt
	recs := e.Records()
	if len(recs) == 0 {
		t.Fatal("terminal error 应产生错误记录")
	}
}

func TestInterruptIdleReturnsFalse(t *testing.T) {
	e := newDirectSubmitEngine(newFakeManagedRunner())
	if e.Interrupt() {
		t.Fatal("非运行期 Interrupt 必须返回 false")
	}
}

func TestRetryControllerRegeneratedPerTurn(t *testing.T) {
	fake := newFakeManagedRunner()
	e := newDirectSubmitEngine(fake)

	// 手工污染上一轮遗留状态（模拟极端场景：取消后未清场的残留）
	e.mu.Lock()
	e.retry.streak = 2
	e.retry.retrying = true
	e.retry.cancelRequested = true
	e.mu.Unlock()

	if !e.SubmitInput("next") {
		t.Fatal("提交必须成功")
	}
	// 重铸同步发生（SubmitInput 内、go turn 前）：此刻状态必须已归零
	e.mu.Lock()
	staleStreak, staleRetrying, staleCancel := e.retry.streak, e.retry.retrying, e.retry.cancelRequested
	e.mu.Unlock()
	if staleStreak != 0 || staleRetrying || staleCancel {
		t.Fatalf("重铸未生效: streak=%d retrying=%v cancel=%v", staleStreak, staleRetrying, staleCancel)
	}

	<-fake.runCalled // 上一轮的取消意图不误杀本轮
	fake.closeCurrent()
	waitIdle(t, e)
	if got := noticeKind(t, e); got == NoticeCancelled {
		t.Fatal("上一轮取消意图泄漏：本轮被误取消")
	}
}
