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
	Error         error
	ErrorType     string
	PartialOutput string
}

// turn 跑一轮对话：调 runner 流式执行，失败时按错误预算自动重试（纠正 prompt + 退避）；
// 预算耗尽则交还用户，等下一次输入充值。中断（Esc）不充值。
func (e *Engine) turn(prompt string) {
	// 中断桥接：框架流式只认 ctx，把中断通道信号转成 ctx 取消。
	// runDone 在回合结束时 close，桥接随之退出——信号不会跨回合残留。
	Ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan struct{})
	defer close(runDone)
	go func() {
		select {
		case <-(*e).interruptCh:
			cancel()
		case <-runDone:
		}
	}()

	// defer 复位是 panic 安全：runOnce 跑的是框架代码，panic 时指示器不能永远转下去
	(*e).setRunning(true)
	defer (*e).setRunning(false)

	for {
		err := e.agentRunOnce(Ctx, prompt)
		if err == nil { //正常结束，或被用户取消（"会话已取消"提示已由 runOnce 打过）
			return
		}
		n, exhausted := (*e).errBudget.fail()
		if exhausted {
			// 放弃自动重试，把控制权交还用户：Run 循环回到等输入，下一次提交
			// 用户输入时充值。预算不在这里充值——耗尽只可能由零输出失败触发。
			(*e).appendTyped("error", fmt.Sprintf("failed %d times in a row, auto-retry stopped. Check network/config and try again.", n))
			return
		}
		// 必须在 Sleep 之前打：sleep 期间引擎不收输入，用户需要知道程序在等什么。
		(*e).appendTyped("warn", fmt.Sprintf("retrying in %ds (attempt %d/%d)...", (*e).errBudget.backoff()/time.Second, n, (*e).errBudget.limit()))
		time.Sleep((*e).errBudget.backoff())
		// 纠正 prompt：把错误与已产出的部分输出喂回去，让模型调整后续输出。
		if err.PartialOutput != "" {
			prompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 之前的输出内容是: %s, 请基于这些信息调整你的回答并继续完成对话", err.Error, err.PartialOutput)
		} else {
			prompt = fmt.Sprintf("之前的对话发生了错误，错误信息是: %s, 请基于这个信息调整你的回答并继续完成对话", err.Error)
		}
	}
}

// ---------- runner 桥接 ----------

// agentRunOnce 把一个 prompt 交给 runner 执行：流式事件转发进消息日志、
// 处理取消、收集失败前的部分输出。返回 nil 表示成功或被取消。
func (e *Engine) agentRunOnce(Ctx context.Context, userPrompt string) *AgentError {
	eventChan, err := (*(*e).AgentRunner_p).Runner.Run(
		Ctx,
		(*(*e).Config_p).User.UserID,
		(*e).SessionId,
		model.Message{
			Role:    model.RoleUser,
			Content: userPrompt,
		},
		agent.WithRequestID((*e).RequestId),                //HackerTeam 保留 RequestId 机制（区别于 HyperBot v3 的删除）
		agent.WithToolCallArgumentsJSONRepairEnabled(true), //开启工具调用参数的JSON修复功能，解决因模型输出格式不规范导致的工具调用失败问题
	)
	if err != nil {
		err = fmt.Errorf("AgentRunner.Run failed: %v", err)
		// 在源头打印，与下面的 TerminalError 分支保持一致：每类错误只打一次。
		(*e).appendTyped("error", err.Error())
		return &AgentError{
			Error:         err,
			ErrorType:     "RunError",
			PartialOutput: "",
		}
	}

	partialOutput := ""
	for event := range eventChan {
		//只有terminal error才会中断对话，其他error直接continue
		if event.Error != nil {
			if event.IsTerminalError() {
				err = fmt.Errorf("event returned TerminalError: %v", event.Error)
				(*e).appendTyped("error", err.Error())
				return &AgentError{
					Error:         err,
					ErrorType:     "TerminalError",
					PartialOutput: partialOutput,
				}
			}
			continue

		}
		select {
		case <-Ctx.Done():
			(*e).setNotice(NoticeCancelled, "")
			return nil
		default:
		}
		if (*event).Response != nil && len((*(*event).Response).Choices) > 0 {

			// 收到任何带 Choices 的 Response 事件（含流式部分块）即归零。语义是"配置层
			// 健康检查"而非"有界重试预算"：能吐 token = key/端点/鉴权/本地网络都通，
			// 故障只剩传输层抖动（随机、重试期望为正），自动重试到成功为止。
			// 已知且刻意接受的代价：确定性中途失败（超时、内容过滤）会无限自动重试，
			// 由人盯着兜底。不要改成"只在 completion 归零"——那会把语义改回有界预算。
			(*e).errBudget.recharge()
			for _, choice := range (*(*event).Response).Choices {
				(*e).emitChoice(choice, (*(*event).Response).IsPartial)
				gatherPartialOutput(&partialOutput, choice, (*(*e).AgentRunner_p).Stream)
			}

		}
		if event.IsRunnerCompletion() {
			break
		}

	}

	return nil

}

// gatherPartialOutput 收集失败前已产出的部分输出，供重试时喂回模型。
// 不能靠框架 session 代替：流式 delta 一律不落盘，中途失败时本轮 assistant 文本在
// session 里一个字都没有（框架的补救机制默认关闭，且只在 ctx 取消时触发，覆盖不到
// TerminalError），这里是唯一记录。只累积 Content，不含 ReasoningContent / ToolCalls。
func gatherPartialOutput(Container_p *string, Choice model.Choice, Stream bool) {
	if Stream {
		*Container_p += Choice.Delta.Content
	} else {
		*Container_p += Choice.Message.Content
	}
}
