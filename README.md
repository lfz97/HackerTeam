# HackerTeam

> **仅供授权渗透测试和安全研究使用。请遵守当地法律法规。**

HackerTeam 是一个面向专业渗透测试人员的 **AI 驱动多智能体渗透测试自动化平台**。它将大语言模型与本地命令执行能力结合，通过标准化的 PTES（渗透测试执行标准）工作流，自动化完成从信息收集到后渗透的完整攻击链。

---

## 目录

- [架构概览](#架构概览)
- [多智能体团队](#多智能体团队)
- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [配置](#配置)
- [工具系统](#工具系统)
- [技能系统](#技能系统)
- [TUI 界面](#tui-界面)
- [会话与上下文管理](#会话与上下文管理)
- [内置命令](#内置命令)
- [构建](#构建)
- [技术栈](#技术栈)

---

## 架构概览

```
用户 (TUI 终端界面)
       │
       ▼
 Captain Agent（队长）
  │  任务规划、子 Agent 调度、结果审核
  │  工具：PWD / CD / LS / ReadFile / WriteFile
  │
  ├──► Recon Agent（侦察）
  │        工具：LocalExec（6 个命令管理工具）
  │        能力：子域名枚举、端口扫描、Web 指纹、目录爆破、被动情报收集
  │
  ├──► VulnAnalyst Agent（漏洞分析）
  │        工具：LocalExec
  │        能力：CVE 关联、配置缺陷识别、Web 漏洞分析、攻击路径规划
  │
  ├──► Exploit Agent（漏洞利用）
  │        工具：LocalExec
  │        能力：SQLi / RCE / LFI / SSRF / XXE、认证绕过、反弹 Shell / WebShell
  │
  └──► PostExploit Agent（后渗透）
           工具：LocalExec
           能力：提权、凭证窃取、横向移动、持久化、痕迹清理
```

**标准攻击链：** `Recon → VulnAnalyst → Exploit → PostExploit → 内网循环`

Captain 通过 `<command>` JSON 标签向子 Agent 下发任务，子 Agent 将结果写入 `output/` 目录的结构化 Markdown 文件后回报路径，Captain 审核结果质量（置信度 ≥ 70%）后决定下一步。

---

## 多智能体团队

| Agent | 角色 | 工具集 | 技能目录 |
|-------|------|--------|----------|
| **Captain** | 中枢调度官，依照 PTES 标准协调整个渗透流程，不直接执行命令 | 文件系统 + 文件读写 | — |
| **Recon** | 被动与主动信息收集 | LocalExec | `ReconSkills/` |
| **VulnAnalyst** | 漏洞分析与攻击路径规划（只分析，不利用） | LocalExec | `VulnAnalyzeSkills/` |
| **Exploit** | 漏洞利用，获取初始访问权限 | LocalExec | `ExploitSkills/` |
| **PostExploit** | 后渗透：提权、横向移动、持久化、数据回传 | LocalExec | `PostExploitSkills/` |

所有 Agent 共享同一底层 LLM，通过专属系统提示词（`bootstrap/prompt/*.md`）获得角色定位。提示词中的 `{{ENV}}` 占位符在启动时自动替换为 OS 类型、用户目录、当前工作目录等环境信息。

---

## 功能特性

- **多智能体协作**：Captain 统一调度，4 个子 Agent 各司其职，自动流转 PTES 攻击阶段
- **PTY 本地执行**：以伪终端模式运行命令，完整支持交互式工具（ssh / sudo / msfconsole 等），不破坏 TUI 布局
- **6 个命令管理工具**：`submit_command` / `start_command` / `get_status` / `get_output` / `intervene_command` / `kill_command`，支持异步任务全生命周期管理
- **MCP 工具热扩展**：支持 SSE、Streamable HTTP、Stdio 三种 MCP 传输协议，`/flush` 命令无需重启即可重载工具和配置
- **知识型技能系统**：通过 `trpc-agent-go` 技能系统注入外部工具知识（nmap / nuclei / sqlmap / dirsearch / fscan 等），Agent 通过 LocalExec 调用
- **自动上下文压缩**：基于 tiktoken 精确计数，超过上下文窗口 70% 或 10 分钟无活动时触发异步摘要（2 并发 worker，最大 2000 词）
- **流式 & 思维链输出**：实时渲染 Token 流，DeepSeek / Claude 思维链内容黄色高亮显示
- **错误自动恢复**：Agent 执行出错时，错误信息和部分输出自动反馈给 LLM 进行自我修正
- **安全护栏**：所有 Agent 提示词内置操作约束（禁止破坏性操作、禁止未授权目标等）
- **跨平台构建**：支持 Linux / macOS / Windows，x64 与 arm64

---

## 快速开始

### 前置依赖

- Go 1.26+（编译时）
- 有效的 LLM API Key（OpenAI 兼容 API 或 Anthropic）

### 运行

1. **下载或编译**可执行文件（见 [构建](#构建) 章节）
2. **首次运行**：程序会在可执行文件同目录自动生成配置目录和配置文件，然后退出并提示修改配置

   ```
   .HackerTeam/
   ├── HackerTeam.yaml              ← 主配置文件
   ├── HackerTeam.log               ← 框架运行日志
   ├── ReconSkills/                 ← Recon Agent 技能
   │   └── pentest-tools/SKILL.md
   ├── VulnAnalyzeSkills/           ← VulnAnalyst Agent 技能
   │   └── pentest-tools/SKILL.md
   ├── ExploitSkills/               ← Exploit Agent 技能
   │   └── pentest-tools/SKILL.md
   └── PostExploitSkills/           ← PostExploit Agent 技能
       └── pentest-tools/SKILL.md
   ```

3. **修改配置**：填入 API Key、模型名称等（详见 [配置](#配置) 章节）
4. **再次运行**即可进入 TUI 交互界面

---

## 配置

配置文件路径：`<可执行文件目录>/.HackerTeam/HackerTeam.yaml`

```yaml
# 用户配置
user:
  userid: "<自动生成的UUID>"

# 模型配置
model:
  model: "deepseek-reasoner"             # 模型名称
  baseurl: "https://api.deepseek.com"    # API 端点（兼容 OpenAI 格式）
  apikey: "YOUR_API_KEY"
  apitype: "openai"                      # "openai" 或 "anthropic"
  contextwindow: 64000                   # 上下文窗口大小（影响摘要触发阈值）
  stream: true                           # 是否启用流式输出

# MCP 服务配置（远程）
# Type 可选: "sse" 或 "streamable_http"
mcp:
  - name: "mcpexec"
    enabled: true
    type: "sse"
    endpoint: "http://127.0.0.1:8080/mcp"
    headers: {}                          # 可选，如: {"Authorization": "Bearer xxx"}

# MCP 服务配置（本地 Stdio）
stdin_mcp:
  - name: "stdin-tool"
    enabled: true
    command: "npx"
    args: ["-y", "mcp-exec"]
```

### 模型提供商

| `apitype` | 支持的提供商 | 说明 |
|-----------|-------------|------|
| `openai` | OpenAI、DeepSeek、Ollama、vLLM 及任意兼容 API | 通过 `baseurl` 指向任意端点；DeepSeek 自动启用思维链回填 |
| `anthropic` | Anthropic Claude 系列 | 使用原生 Anthropic SDK |

---

## 工具系统

### LocalExec（内置，始终启用）

以 PTY 模式在本地执行系统命令，支持完整的交互式进程管理：

| 工具 | 功能 |
|------|------|
| `submit_command` | 提交命令（进程名 + 参数），返回任务 ID |
| `start_command` | 按 ID 启动命令（在 PTY 中运行） |
| `get_status` | 查询任务状态（pending / running / done / failed / killed，含 PID、ExitCode） |
| `get_output` | 获取 stdout/stderr 输出（支持滑动窗口） |
| `intervene_command` | 向进程写 stdin 或发送信号（SIGINT / SIGTERM / SIGKILL） |
| `kill_command` | 强制结束进程 |

PTY 设计确保 ssh、sudo、msfconsole 等需要伪终端的工具正常工作，且不会直接写入 `/dev/tty` 破坏 TUI。

### Captain 原生工具

| 工具 | 功能 |
|------|------|
| `PWD` | 打印当前工作目录 |
| `CD` | 切换工作目录 |
| `LS` | 列出目录内容 |
| `ReadFile` | 读取文件内容（支持滑动窗口） |
| `WriteFile` | 创建或覆写文件 |

Captain 使用这些工具在子 Agent 的输出目录中审核结果文件。

### MCP（按需配置）

通过配置文件添加任意 MCP 兼容的工具服务器：

- **SSE / Streamable HTTP**：连接远程 MCP 服务（网络传输，10s 超时，3 次重连）
- **Stdio**：启动本地子进程作为 MCP 服务（stdin/stdout 通信）

使用 `/flush` 命令可在不重启的情况下重新加载 MCP 工具和配置。

---

## 技能系统

外部渗透工具（nmap、dirsearch、sqlmap、nuclei、fscan 等）以 **知识型技能**（Knowledge-Only Skill）的形式集成，通过 `trpc-agent-go` 的技能系统注入到 Agent 系统提示词中。

技能内容描述工具的类别、路径和使用模式，作为上下文提供给 Agent 参考，但实际命令执行仍通过 LocalExec 工具完成。

每个 Agent 拥有独立的技能目录，首次运行自动从模板生成：

| Agent | 技能目录 | 用途 |
|-------|----------|------|
| Recon | `ReconSkills/pentest-tools/` | 端口扫描、子域名枚举、目录爆破工具 |
| VulnAnalyst | `VulnAnalyzeSkills/pentest-tools/` | 漏洞扫描、CVE 分析工具 |
| Exploit | `ExploitSkills/pentest-tools/` | 漏洞利用框架、Shell 生成工具 |
| PostExploit | `PostExploitSkills/pentest-tools/` | 提权、横向移动、持久化工具 |

`/flush` 命令会重新创建技能仓库并挂载到对应 Agent。

---

## TUI 界面

程序包含两个页面阶段：

**1. 配置页（启动时）**

ASCII 艺术 Banner + 实时日志视图，展示初始化进度（配置检查、技能目录创建、Agent 组装等）。初始化完成后自动跳转至 Agent 页。

**2. Agent 页（交互主界面）**

```
┌──────────────────────────────────────────────────────────┐
│ [StatusBar] 动态滚动提示文字                                │
├──────────────┬───────────────────────────────────────────┤
│              │                                            │
│  [Sidebar]   │         [AgentMessageView]                 │
│  操作提示     │   Agent 流式输出、工具调用、思维链、         │
│  命令说明     │   上下文摘要（支持动态颜色、自动滚动）        │
│              │                                            │
├──────────────┴───────────────────────────────────────────┤
│ ⇒ [InputArea] Ctrl+Enter 提交  ESC 中断                   │
└──────────────────────────────────────────────────────────┘
```

**颜色约定：**

| 颜色 | 含义 | 示例 |
|------|------|------|
| 黄色 | 模型思维链内容（`<think>` 块，标记为 `»` `«`） | DeepSeek-R1 / Claude 推理过程 |
| 品红 | 工具调用信息（标记为 `⚙`、`├─`、`└─`） | Agent 调用 submit_command 等 |
| 绿色 | 上下文摘要通知 | 自动压缩完成提示 |
| 白色 | Agent 正文输出 | 正常对话内容 |

**主题：** GitHub Dark 风格，背景色 `#0F1115`，面板 `#151821`，边框 `#2A2F3A`。

---

## 会话与上下文管理

### 会话服务

基于 `trpc-agent-go` 的 in-memory session 服务，支持多轮对话状态持久化。`/new` 命令清空会话历史重新开始。

### 自动上下文压缩

当对话历史超过 LLM 上下文窗口限制时，自动触发异步摘要：

| 参数 | 值 |
|------|-----|
| Token 计数器 | tiktoken（精确计数） |
| 触发阈值 | 新 token 数超过 `contextwindow` 的 70% |
| 时间阈值 | 10 分钟无活动 |
| 并发 worker | 2 个 |
| 摘要上限 | 2000 词 |
| 摘要模型 | 与 Agent 使用相同 LLM |
| 队列容量 | 100 |

摘要完成后，绿色高亮通知显示在 TUI 中，`<think>` 标签自动剥离。

### 错误恢复

Agent 执行过程中如遇错误（非致命），下一轮对话会自动将错误信息和部分输出反馈给 LLM，使其能够感知上下文并尝试自行修正。

---

## 内置命令

在输入框中输入以下命令并按 `Ctrl+Enter` 执行：

| 命令 / 按键 | 功能 |
|-------------|------|
| `/exit` | 退出程序 |
| `/new` | 开启新会话（清空对话历史，重置 Session ID） |
| `/flush` | 重新加载配置和 MCP 工具，重建技能仓库（热重载，无需重启） |
| `ESC` | 中断当前 Agent 执行（取消 Runner Context） |
| `Ctrl+Enter` | 提交当前输入 |

---

## 构建

### Makefile 交叉编译

```bash
make              # 全平台编译
make linux-x64    # 仅 Linux x64
make linux-arm64  # 仅 Linux arm64
make macos-x64    # 仅 macOS x64
make macos-arm64  # 仅 macOS arm64 (Apple Silicon)
make windows-x64  # 仅 Windows x64
```

### PowerShell 交叉编译

```powershell
.\build.ps1
```

### 手动编译（当前平台）

```bash
go build -ldflags "-s -w" -o HackerTeam .
```

### 输出目录

```
release/
├── linux-arm64/HackerTeam
├── linux-x64/HackerTeam
├── macos-arm64/HackerTeam
├── macos-x64/HackerTeam
└── windows-x64/HackerTeam.exe
```

所有平台均使用 `-s -w` 链接参数去除调试符号以减小体积。

---

## 技术栈

| 组件 | 库/框架 |
|------|---------|
| AI Agent 框架 | `trpc.group/trpc-go/trpc-agent-go` v1.9.0 |
| MCP 协议 | `trpc.group/trpc-go/trpc-mcp-go` v0.0.15 |
| TUI | `github.com/rivo/tview` v0.42.0 + `github.com/gdamore/tcell/v2` |
| PTY 伪终端 | `github.com/creack/pty` |
| 配置解析 | `gopkg.in/yaml.v2` |
| 日志 | `go.uber.org/zap` |
| UUID | `github.com/google/uuid` |
| 文件复制 | `github.com/otiai10/copy`（技能模板部署） |
| Token 计数 | `github.com/tiktoken-go/tokenizer`（上下文压缩） |
| LLM 后端 | OpenAI 兼容 API + Anthropic 原生 SDK |

---

## 免责声明

本工具仅供授权的安全研究、渗透测试和教育目的使用。使用者须对自身行为负全部法律责任。未经授权对任何系统使用本工具均属违法行为。
