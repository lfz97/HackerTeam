# Agent 时间意识 — 回调实现笔记

> 目的:让 agent 产生时间意识(now 当前时间 / duration 每命令耗时 / recency 事件新旧)。
> 时间数据框架里已有(LocalExec 有 StartedAt/EndedAt),缺的是**呈现到模型可见的内容**。

## 机制选择

用 **ModelCallbacks + ToolCallbacks**(v1.10.0 已有,已验证非 main-only)。
**不要用 Planner**:自定义 Planner 会替换 builtin,丢掉 thinking/reasoning 配置(show_reasoning / ReasoningEffort 等)。

## 两个回调

### 1. BeforeModel — 状态行(now 锚点)

```go
mc := model.NewCallbacks()
mc.RegisterBeforeModel(func(ctx context.Context, args *model.BeforeModelArgs) (*model.BeforeModelResult, error) {
    if time.Since(lastStatusAt) < 2*time.Minute { // 门控:≥2min 或每 5 轮
        return nil, nil
    }
    lastStatusAt = time.Now()
    statusMsg := model.NewSystemMessage(fmt.Sprintf(
        "[STATUS] 当前时间: %s,会话已运行: %s",
        time.Now().Format("2006-01-02 15:04:05"),
        time.Since(sessionStart).Round(time.Second)))
    // 前置插入(不是追加到末尾)
    args.Request.Messages = append([]model.Message{statusMsg}, args.Request.Messages...)
    return nil, nil // 只改 Request,不拦截链路
})
```

- 签名:`func(ctx, *model.BeforeModelArgs) (*model.BeforeModelResult, error)`
- 内容一行、陈述句,不用祈使句
- 放**最前端** prepend system,不追加、不合并进历史 user 消息

### 2. BeforeTool + ToolResultMessages — 耗时戳(duration/recency)

```go
tc := tool.NewCallbacks()
tc.RegisterBeforeTool(func(ctx context.Context, args *tool.BeforeToolArgs) (*tool.BeforeToolResult, error) {
    startTimes.Store(args.ToolCallID, time.Now())
    return nil, nil
})
tc.RegisterToolResultMessages(func(ctx context.Context, in *tool.ToolResultMessagesInput) (any, error) {
    start, ok := startTimes.Load(in.ToolCallID)
    if !ok {
        return nil, nil // 回退默认消息
    }
    d := time.Since(start.(time.Time))
    content := fmt.Sprintf("[%s 完成,耗时 %s] %s",
        time.Now().Format("15:04:05"), d.Round(time.Second), in.DefaultToolMessage)
    return []model.Message{{Role: model.RoleTool, Content: content, ToolID: in.ToolCallID}}, nil
})
```

- 时间戳放**结果最前面**(`WithToolResultFormatter` 截断 head 500,tail 500,放前面截不掉)
- 覆盖所有工具(文件、memory、localexec),统一横切,不改任何 tool 代码

## 两种回调模式(易混淆,先分清)

### 模式一:Before/After 型(链式拦截)

返回值 = **控制信号**,副作用 = 修改。默认执行(发 API / 执行工具)在后面,想让它继续就跑 `nil, nil`:

| 返回值 | 效果 |
|---|---|
| `nil, nil` | 不拦截,默认流程继续(请求照发 / 工具照执行) |
| `CustomResponse` / `CustomResult` 非空 | 拦截:跳过模型调用 / 跳过工具执行 |
| `err` | 报错:请求失败 / 工具不执行 |

- BeforeModel 拦"模型调用"(API 请求不发);BeforeTool 拦"工具执行"(结果仍回给模型)
- 执行控制选项(`WithContinueOnError` / `WithContinueOnResponse`)只影响"剩余回调是否继续遍历",**不影响"请求是否发送"**——发送与否只看最终返回的 result/error(llmflow.go:1681-1690)

### 模式二:ToolResultMessages 型(纯转换函数)

返回值 = **交付物本身**。没有默认执行可拦,唯一职责是把工具结果转换成送回模型的消息:

| 返回值 | 效果 |
|---|---|
| `nil, nil` 或空切片 | 放弃转换,框架回退 DefaultToolMessage(原始结果) |
| `[]model.Message{...}` | 用构造的消息替换默认消息 |

### 为什么笔记里两个回调的写法不一样

- **BeforeModel**:靠"改 Request 的副作用"注入状态行 → `return nil, nil`
- **BeforeTool**:纯记账(startTimes),连 ModifiedArguments 都不用 → `return nil, nil`
- **ToolResultMessages**:装饰后的消息就是交付物,不返回 = 没装饰 → **必须返回构造的消息**;只有 `startTimes.Load` 失败(无起点记录)才 `nil, nil` 回退默认

## 为什么这样放(踩过的坑)

| 方案 | 结论 |
|---|---|
| 头部 prepend system 消息 | ✅ 位置恒定,tool loop 迭代里也可见 |
| user role 独立消息 | ❌ Anthropic tool 循环角色交替约束会报错;模型可能误读为新指令 |
| 合并进"最后一个 user 消息" | ❌ tool loop 迭代里没有新 user 消息,会合并进最旧的派发消息,锚点位置差 |
| 改 LocalExec 工具加时间字段 | ❌ 时间意识是横切关注点,不该由 tool 承载;回调层统一覆盖 |

## 双后端兼容(已验证)

- **OpenAI/DeepSeek**:多条 system 消息按序拼接,原生支持
- **Anthropic**:`convertMessages` 把 RoleSystem 全部提取进顶层 `system` 参数(anthropic.go:1042-1047),`system` 是 blocks 数组,原生支持多条
- **summarizer 不受影响**:摘要模型调用不带这些 callbacks(`s.modelCallbacks` 默认 nil),状态行不会污染摘要输入
- **缓存**:`cacheSystemPrompt` 默认关(anthropic/options.go:24,opt-in),当前无缓存失效问题;将来开了缓存,状态行在 SOP block 之后也安全

## 挂载(bootstrap/members.go)

```go
// 工厂函数每次返回新实例 —— 门控状态(lastStatusAt)按 agent 隔离
func statusModelCallbacks() *model.Callbacks { ... }
func durationToolCallbacks() *tool.Callbacks { ... }

// 每个 initXxx 的 opts 里:
opts := []llmagent.Option{
    ...
    llmagent.WithModelCallbacks(statusModelCallbacks()),
    llmagent.WithToolCallbacks(durationToolCallbacks()),
}
```

- 子 agent(Recon/Scanner/Exploit/PostExploit/Reproducer)全挂
- Captain 可不挂状态行(不跑长命令),但建议挂 AgentCallbacks 保险丝

## 可选:AgentCallbacks 保险丝(防死循环/配额)

```go
ac := agent.NewCallbacks()
ac.RegisterBeforeAgent(func(ctx context.Context, args *agent.BeforeAgentArgs) (*agent.BeforeAgentResult, error) {
    if args.Invocation.TokenUsage.Total >= maxTokens || args.Invocation.GetToolCallCount() >= maxCalls {
        return nil, agent.NewStopError("tool call limit reached") // → stop_agent_error,循环终止
    }
    return nil, nil
})
```

- 代码级硬约束,不靠 LLM 自觉
- 与 ModelCallbacks 区别:AgentCallbacks 每次 agent 执行一次(循环外),拿得到完整 Invocation(累计 TokenUsage);ModelCallbacks 每次 LLM 请求(循环内),能改消息但拿不到累计用量

## API 位置(v1.10.0)

| 符号 | 位置 |
|---|---|
| `model.NewCallbacks` | model/callbacks.go:132 |
| `RegisterBeforeModel` | model/callbacks.go:158 |
| `tool.NewCallbacks` / `RegisterToolResultMessages` | tool/callbacks.go:242 |
| `agent.NewCallbacks` / `NewStopError` | agent/callbacks.go / agent/agent.go:57 |
| `WithModelCallbacks` / `WithToolCallbacks` / `WithAgentCallbacks` | llmagent/option.go:1313 / 1320 / 1306 |
| BeforeModel 触发点 | internal/flow/llmflow/llmflow.go:1681 |

## 踩坑清单

- [ ] 回调永远 `return nil, nil`(返回 CustomResponse/CustomResult 会短路后续链路)
- [ ] 状态行用陈述句,不用祈使句("当前时间 X" ✅,"注意时间" ❌)
- [ ] 门控必须做(每 5 轮或 ≥2min),否则注意力稀释、状态行失效
- [ ] 耗时戳放结果最前面(防 1000-rune 截断)
- [ ] Callbacks 实例按 agent 独立,不共享(门控状态会串)
- [ ] 不碰 Planner(替换 builtin 会丢 thinking 配置)
