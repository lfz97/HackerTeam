package engine

import (
	"context"
	"fmt"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// ---------- 一轮对话 ----------

type AgentError struct {
	Error     error
	ErrorType string
}

// turn 跑一轮对话：调 runner 流式执行，失败时按重试控制器自动重试（干净重放 + 退避）；
// 预算耗尽则交还用户，等下一次输入充值。中断（Esc→Interrupt）不充值：已注册的 run
// 由 runner.Cancel 立即终止；注册表空窗（退避间隙）的取消意图由 retry.cancelRequested
// 在退避 tick / Run 边界生效（见 uistate.go Interrupt 与 retry_controller.go）。
// 占坑（running）与 requestID 记账均由 SubmitInput 同步完成，这里只负责执行与收尾。
func (e *Engine) turn(prompt string, reqID string) {
	// defer 复位是 panic 安全：runOnce 跑的是框架代码，panic 时指示器不能永远转下去。
	// endTurn 单临界区清 retry 瞬态+running 的理由见其注释。
	defer (*e).endTurn()

	for {
		if e.consumeCancelRequested() {
			// 取消意图兑现（Run 边界消费点）：覆盖退避结束后的下轮重试
			// 与注册表空窗的 Esc。负载场景分析见 retry_controller.go。
			(*e).setNotice(NoticeCancelled, "")
			return
		}
		// reqID 请求粒度（SubmitInput 铸造，错误重试沿用）：失败的 run 在 Run
		// 返回错误前已注销，同 ID 复用无冲突；极端竞速撞上上一轮收尾的
		// "already running" 会走错误预算自愈（退避 3s 远大于收尾时长）。
		err := e.agentRunOnce(context.Background(), prompt, reqID)
		if err == nil { //正常结束，或被用户取消
			// 取消的提示兜底：完成事件（RunOutcome）的投递会被已取消的 ctx
			// 掐断（框架 EmitEventWithTimeout 在 ctx done 后丢事件，实测 WARN
			// 可见），runOnce 里的 RunOutcome 检测因此不可靠。Interrupt 置位的
			// cancelRequested 是取消意图的可靠记账，在成功分支消费它：
			// run 被取消 → 此刻必然残留 true；正常完成 → 无残留不误报。
			if e.consumeCancelRequested() {
				(*e).setNotice(NoticeCancelled, "")
			}
			return
		}
		n, exhausted := (*e).retryFail()
		if exhausted {
			// 放弃自动重试，把控制权交还用户：SubmitInput 回到可占坑状态，下一次提交
			// 用户输入时充值。预算不在这里充值——耗尽只可能由零输出失败触发。
			(*e).appendTyped("error", fmt.Sprintf("failed %d times in a row, auto-retry stopped. Check network/config and try again.", n))
			return
		}
		// 必须在 Sleep 之前打：sleep 期间引擎不收输入，用户需要知道程序在等什么。
		(*e).appendTyped("warn", fmt.Sprintf("retrying in %ds (attempt %d/%d)...", (*e).retryBackoff()/time.Second, n, (*e).retryLimit()))
		// 退避可中断：以 tick 粒度响应取消意图（Esc 在退避里的生效延迟
		// 从"等满整个 gap"降到 ≤tick），取消则本轮直接收尾。
		if !e.retrySleep((*e).retryBackoff()) {
			(*e).setNotice(NoticeCancelled, "")
			return
		}
		// 干净重试：原 prompt 已作为未回复的 user 消息留在 session 尾部
		// （失败尝试零 assistant 落盘），重试以空 invocation 发起——session
		// 零新写入、模型视图回到原上下文，失败与重试对模型完全隐形（实验9
		// 验证，等价 pi 的"干净重放"语义）。不喂错误详情：预算耗尽只可能由
		// 零输出失败（网络/API 类）触发，模型修复不了 500——喂回去只会往
		// session 里落一条永久噪音消息污染后续每一轮上下文。
		prompt = ""
	}
}

// retrySleep 退避等待，期间以 tick 粒度检查取消意图；被取消返回 false。
// tick 取 200ms：Esc 生效延迟上界，远小于最小退避间隔（默认 3s）。
func (e *Engine) retrySleep(d time.Duration) bool {
	const tick = 200 * time.Millisecond
	deadline := time.Now().Add(d)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			return true
		}
		if e.consumeCancelRequested() {
			return false
		}
		if remain > tick {
			remain = tick
		}
		time.Sleep(remain)
	}
}

// ---------- runner 桥接 ----------

// agentRunOnce 把一个 prompt 交给 runner 执行：流式事件转发进消息日志、
// 处理取消。返回 nil 表示成功或被取消。
// reqID 由调用方（SubmitInput/turn）铸造：WithRequestID 传入后 runner 把它注入
// 每个事件的 event.RequestID，并据此支持 Cancel/RunStatus（见 Interrupt）。
func (e *Engine) agentRunOnce(Ctx context.Context, userPrompt string, reqID string) *AgentError {
	eventChan, err := (*(*e).AgentRunner_p).Runner.Run(
		Ctx,
		(*(*e).Config_p).User.UserID,
		(*e).SessionId,
		model.Message{
			Role:    model.RoleUser,
			Content: userPrompt,
		},
		agent.WithRequestID(reqID),                         //HackerTeam 保留请求级 RequestId 机制（区别于 HyperBot v3 的删除）
		agent.WithToolCallArgumentsJSONRepairEnabled(true), //开启工具调用参数的JSON修复功能，解决因模型输出格式不规范导致的工具调用失败问题
		// 开启 pre-LLM 延迟诊断 span（含 compaction 的 summary/rebuild 两段）：
		// Langfuse 只收 OTel span，不开则压缩阶段在 trace 里是黑盒。
		// 事件开关显式关掉——诊断 span 已足够，不要往事件流里灌状态事件。
		agent.WithLatencyDiagnostics(true),
		agent.WithLatencyDiagnosticsEvents(false),
	)
	if err != nil {
		err = fmt.Errorf("AgentRunner.Run failed: %v", err)
		// 在源头打印，与下面的 TerminalError 分支保持一致：每类错误只打一次。
		(*e).appendTyped("error", err.Error())
		return &AgentError{
			Error:     err,
			ErrorType: "RunError",
		}
	}

	for event := range eventChan {
		//只有terminal error才会中断对话，其他error直接continue
		if event.Error != nil {
			if event.IsTerminalError() {
				err = fmt.Errorf("event returned TerminalError: %v", event.Error)
				(*e).appendTyped("error", err.Error())
				return &AgentError{
					Error:     err,
					ErrorType: "TerminalError",
				}
			}
			continue

		}
		if (*event).Response != nil && len((*(*event).Response).Choices) > 0 {

			// 收到任何带 Choices 的 Response 事件（含流式部分块）即归零。语义是"配置层
			// 健康检查"而非"有界重试预算"：能吐 token = key/端点/鉴权/本地网络都通，
			// 故障只剩传输层抖动（随机、重试期望为正），自动重试到成功为止。
			// 已知且刻意接受的代价：确定性中途失败（超时、内容过滤）会无限自动重试，
			// 由人盯着兜底。不要改成"只在 completion 归零"——那会把语义改回有界预算。
			(*e).retryRecharge()
			for _, choice := range (*(*event).Response).Choices {
				(*e).emitChoice(choice, (*(*event).Response).IsPartial)
			}

		}
		if event.IsRunnerCompletion() {
			// 取消检测挂到框架的完成事件上（原 ctx.Done 分支已随中断桥接删除）：
			// RunOutcome 只在 run 被显式取消（Cancel）或超时时非 nil。尽力而为
			// ——取消后完成事件的投递可能被已取消的 ctx 掐断（框架 WARN 可见），
			// 可靠兜底见 turn 成功分支的 cancelRequested 消费。
			if event.RunOutcome != nil {
				(*e).setNotice(NoticeCancelled, "")
			}
			break
		}

	}

	return nil

}
