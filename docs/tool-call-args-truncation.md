# Tool Call 参数消失/截断问题：根因分析与修复方案

> 日期：2026-08-11
> 症状：agent 调用 `WriteFile` / `submit_command`（localexec）时参数"消失"，载荷越大越容易触发
> 结论：**不是序列化/传递失败，也不是框架 bug。根因 = 未设置 max_tokens（实际生效默认值仅 4096）→ 大载荷 tool call JSON 被模型输出截断 → 项目启用的 JSON 修复功能把残缺 JSON "修复"成静默的不完整参数。**

---

## 1. 根因链（五步，均已在源码级验证）

### ① 项目从未设置 MaxTokens，实际生效上限只有 4096

- `service/engine/members.go`：全部 6 个 agent 的生成配置只设了 Stream：
  ```go
  llmagent.WithGenerationConfig(model.GenerationConfig{
      Stream: (*(*e).Config_p).Model.Stream,
  }),
  ```
  `MaxTokens` 为 nil。
- `service/engine/config/config.go` 的 `Model` 结构体**没有** max_tokens 字段——配置层无法设置。
- 框架行为：
  - openai adapter（`model/openai/openai.go:962`）：`MaxTokens == nil` 时请求里**不发送** max_tokens 字段 → 提供商默认值生效。**DeepSeek API 的 max_tokens 默认值是 4096**（官方 OpenAPI spec：default 4096）。
  - anthropic adapter（`model/anthropic@v1.11.0/anthropic.go:359-363`）：未设置时框架强制 `chatRequest.MaxTokens = 4096`。
- 默认模型 `deepseek-reasoner`（config.yaml 模板）：**thinking 过程与正式输出（含 tool call JSON）共享这 4096 token 预算**。R1 的思考动辄几千 token，留给 tool call 的预算非常有限。

### ② 大载荷 tool call 撞上限 → 输出被拦腰截断

- `WriteFile` 写大文件（`content` 几十 KB）、`submit_command` 跑大 heredoc，其 tool call JSON 轻松超出剩余预算。
- 流式输出在 4096 token 处终止，`finish_reason="length"`，此时 tool call 的 arguments JSON 是**残缺的**（断在字符串中间、key 中间或两个参数之间）。
- 框架 v1.11.0 对 `finish_reason=length` **零处理**——不报错、不警告（已 grep 确认 processor/agent/runner 无相关逻辑），残缺参数原样流向工具执行层。
- 流式拼接本身没问题：参数 delta 由官方 openai-go SDK 的 `ChatCompletionAccumulator` 累积，`processAccumulatedToolCalls`（openai.go:2650）原样透传，无丢失。

### ③ JSON 修复把"显性失败"变成"隐性参数丢失"

- `service/engine/engineRun.go:142` 启用了：
  ```go
  agent.WithToolCallArgumentsJSONRepairEnabled(true)
  ```
- 工具执行前框架调用 `jsonrepair.RepairToolCallArgumentsInPlace()`（`internal/flow/processor/functioncall.go:3436`）→ 残缺 JSON 走 `internal/jsonrepair.Repair()`：补上未闭合的引号/括号/数组，产出"合法"JSON。
- **Repair 对截断 JSON 的行为（已用框架自己的源码复现实验验证，见 §3）：**

  | 截断位置 | 修复结果 |
  |---|---|
  | 某个 value 中间 | 值被截断 → WriteFile 只写入半个文件；heredoc 内容不完整 |
  | 某个 key 中间（如 `"app`） | **该参数整个消失** |
  | 两个参数之间（逗号后） | 后面的参数全部丢失 |
  | submit_command 的 heredoc 中间 | bash 收不到结束符 → 命令挂起等待 stdin，或写入不完整文件 |

- 工具执行**返回成功**，TUI 显示一切正常——全链路静默。这就是"参数凭空消失"的直接原因。

### ④ 为什么 agent 的"序列化失败"猜测不对

参数不是在传递/序列化层丢的。数据流上每一层都验证过：工具实现无截断逻辑、流式累积无丢失、TUI 渲染只影响显示不影响执行、上下文压缩/摘要只处理历史 tool result 不碰当前 assistant tool_call 参数。丢失发生在**最上游**——模型输出本身就没生成完整。

---

## 2. 已排除的假设

| 假设 | 排除依据 |
|---|---|
| localexec / WriteFile 工具自身截断参数 | `tools/toolsets/localexec/tools.go`、`tools/functions/file.go` 无任何长度限制逻辑 |
| 流式 delta 拼接丢块 | 官方 SDK `ChatCompletionAccumulator` 累积；`processAccumulatedToolCalls` 原样透传 |
| TUI 渲染丢参数 | `messageRender.go` 的 `addToolCallMsg` 原样保存 Arguments，只影响显示 |
| 上下文压缩/摘要删参数 | Compaction Pass1/2 只处理 role=tool 的结果消息；摘要跳过最近一轮（WithSkipRecent）；均不触碰当前调用的 assistant tool_call |
| DeepSeek variant 特殊处理引入丢失 | variant 差异仅在 thinking 格式/认证/文件上传，不影响参数累积 |

---

## 3. 复现实验记录

把框架 `internal/jsonrepair` 的源码（jsonrepair.go/loads.go/utils.go/error.go，均只依赖标准库）拷到临时模块，模拟 max_tokens 截断后的残缺 JSON 走 Repair：

```go
// 场景1：WriteFile 截断于 content 值中间
input := `{"path": "/tmp/payload.py", "content": "import socket\n...（大段内容，中途截断）`
// 严格 JSON 解析失败: invalid character '\n' in string literal
// Repair 输出: {"path":"/tmp/payload.py","content":"import socket\n...（截断处闭合）"}
// → content 参数存在但只剩一半 → 文件被静默写入一半

// 场景2：WriteFile 截断于下一个 key 中间（"append": false 的 "app 处）
// Repair 输出: {"path":"...","content":"...完整...","append":"fa"}（不完整的 key/值被丢弃或补全为残缺值）
// → 后续参数消失或变成垃圾值

// 场景3：submit_command 大 heredoc 截断
input := `{"process":"bash","args":["-c","cat > /tmp/x.py << 'PYEOF'\n...(截断)"]}`
// → args[1] 被截断：heredoc 没有结束符 → bash 挂起等 stdin / 写入半个文件

// 场景4：截断恰好在两个参数之间（逗号后）
input := `{"path": "/tmp/payload.py", "content": "abc123", `
// Repair 输出: {"path":"/tmp/payload.py","content":"abc123"}
// → 后面的参数全部消失
```

实验结论与线上症状完全吻合：**参数消失还是内容截断，取决于截断恰好落在 JSON 的哪个位置**——这解释了为什么症状看起来"时有时无、形态不一"。

---

## 4. 在你自己机器上的验证方法

```bash
# 1. 找 JSON 修复日志（Info 级，框架修复时必打）——命中即实锤
grep "JSON repaired" <binary目录>/.HackerTeam/HackerTeam.log
# 期望看到: Tool call arguments JSON repaired for WriteFile / submit_command

# 2. 若出现频率高，可对照 session 数据库里对应 assistant 消息的 tool_call arguments，
#    应能看到残缺 JSON（引号/括号不闭合）的原始形态
```

---

## 5. 修复方案（按优先级）

### 修复 1（核心）：给 config 增加 max_tokens 并透传

**`service/engine/config/config.go`** — Model 结构体加字段：

```go
type Model struct {
    Model                       string `yaml:"model"`
    BaseURL                     string `yaml:"baseurl"`
    APIKey                      string `yaml:"apikey"`
    APIType                     string `yaml:"apitype"`
    AnthropicAuthHeaderTransfer bool   `yaml:"anthropicAuthHeaderTransfer"`
    Stream                      bool   `yaml:"stream"`
    MaxTokens                   int    `yaml:"max_tokens"` // 新增：最大输出 token（thinking+输出共享）；<=0 时代码兜底 32768
    ContextWindow               int    `yaml:"contextwindow"`
    ShowReasoning               bool   `yaml:"show_reasoning"`
}
```

**`service/engine/config/config.yaml`**（embed 模板，影响首次运行生成的配置）加一行：

```yaml
  max_tokens: 32768 # 最大输出token（思考与工具调用共享此预算）。DeepSeek默认仅4096，不足会导致大载荷工具调用被截断，详见 docs/tool-call-args-truncation.md
```

注意：**已有用户的 .HackerTeam/HackerTeam.yaml 不会自动升级**，缺失字段解析为 0，所以代码里要有兜底默认值（见下）。

**`service/engine/members.go`** — 加一个 helper，替换 6 处 `WithGenerationConfig`：

```go
// generationConfig 统一构造生成配置。MaxTokens 必须显式指定：
// 不指定时框架不发送该字段，DeepSeek API 默认仅 4096（thinking 与工具调用输出共享），
// 大载荷 tool call 撞限会被截断，再经 JSON 修复静默丢参。详见 docs/tool-call-args-truncation.md
func generationConfig(m config.Model) model.GenerationConfig {
    maxTokens := m.MaxTokens
    if maxTokens <= 0 {
        maxTokens = 32768 // 老配置无此字段时的兜底值
    }
    return model.GenerationConfig{
        Stream:    m.Stream,
        MaxTokens: &maxTokens,
    }
}
```

6 个 `init*()` 里统一改为：

```go
llmagent.WithGenerationConfig(generationConfig((*(*e).Config_p).Model)),
```

注意事项：
- 框架的 `ClampMaxTokensForModel` 会按内置表对 deepseek-reasoner 钳制到 393216；但 **DeepSeek API 对 deepseek-reasoner 的实际输出上限是 64K**，配置值不要超过 65536，否则 API 会报错。32768 是安全值；预算只是上限，实际按生成量计费，设大不浪费钱。
- anthropic 路径同样受益（之前未设置时框架也强制 4096）。

### 修复 2（强烈建议）：让截断可见

目前框架对 `finish_reason="length"` 完全静默。在 `service/engine/engineRun.go` 的 `agentRunOnce` 事件循环里加检测（放在非 ToolResponse 分支，`Choice := response.Choices[0]` 之后）：

```go
// 检测输出截断：撞 max_tokens 上限时 finish_reason=length，tool call 参数可能已被截断
if !response.IsPartial && Choice.FinishReason != nil && *Choice.FinishReason == "length" {
    (*e).tui.PrintToMsgView(pretty.TErrorF("⚠ 模型输出达到 max_tokens 上限被截断，本次工具调用参数可能不完整！\n"), false)
}
```

说明：
- `Choice.FinishReason` 是 `*string`，框架已确认流式最终响应会传播该字段（openai.go `createFinalResponse`）。
- 用 `!response.IsPartial` 避免 partial 事件与 final 事件重复告警。
- 子 agent（team 串行调度）的响应也会流经这个循环（`WithMemberToolStreamInner(true)`），所以 5 个子 agent 的截断同样能被看到。

### 修复 3（不需要做，但要知道）：不要关闭 JSON 修复

`WithToolCallArgumentsJSONRepairEnabled(true)` 建议保留——它还兜底 DeepSeek 偶发的 JSON 格式问题（如多余尾随文本）。关闭它会把本问题从"静默错参"变回"显式报错"，但也会让其他格式小毛病重新变成硬失败。修复 1 解决根因后，repair 只会作为保险存在，且每次触发都会打 Info 日志（"Tool call arguments JSON repaired for ..."），可继续用作监控信号。

---

## 6. 参考资料

- 框架源码（v1.11.0，Go module cache）：
  - `model/openai/openai.go:962` — MaxTokens 为 nil 时不发送
  - `model/anthropic@v1.11.0/anthropic.go:359-363` — 未设置时强制 4096
  - `tool/function/function_tool.go` `unmarshalToolArgs` — repair 入口
  - `internal/flow/processor/functioncall.go:3436` — 执行前原地修复
  - `internal/jsonrepair/{jsonrepair,toolcall}.go` — 修复算法与日志点
  - `model/internal/model/model_info.go` — 框架内置 max output 表
- DeepSeek API max_tokens 默认 4096：官方 OpenAPI spec / api-docs.deepseek.com/news/news0725
- 项目内相关：`CLAUDE.md` Context Management 章节（Compaction/摘要只影响历史，与本次根因无关）
