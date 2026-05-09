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
- [工具集成](#工具集成)
- [TUI 界面](#tui-界面)
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
  │  任务规划、子 Agent 调度、结果汇总
  │  工具：PWD / CD / LS / ReadFile / WriteFile
  │
  ├──► Recon Agent（侦察）
  │        工具：LocalExec
  │        能力：子域名枚举、端口扫描、Web 指纹、目录爆破、被动情报
  │
  ├──► VulnAnalyst Agent（漏洞分析）
  │        工具：LocalExec
  │        能力：CVE 关联、配置缺陷识别、Web 漏洞分析、攻击路径规划
  │
  ├──► Exploit Agent（漏洞利用）
  │        工具：LocalExec
  │        能力：SQLi/RCE/LFI/SSRF/XXE、认证绕过、反弹 Shell/WebShell
  │
  └──► PostExploit Agent（后渗透）
           工具：LocalExec
           能力：提权、凭证窃取、横向移动、持久化、痕迹清理
```

**标准攻击链：** `Recon → VulnAnalyst → Exploit → PostExploit → 内网循环`

---

## 多智能体团队

| Agent | 角色 | 工具集 |
|-------|------|--------|
| **Captain** | 中枢调度官，依照 PTES 标准协调整个渗透流程，不直接执行命令 | 文件系统 + 文件读写 |
| **Recon** | 被动与主动信息收集 | LocalExec |
| **VulnAnalyst** | 漏洞分析与攻击路径规划（只分析，不利用） | LocalExec |
| **Exploit** | 漏洞利用，获取初始访问权限 | LocalExec |
| **PostExploit** | 后渗透：提权、横向移动、持久化、数据回传 | LocalExec |

所有 Agent 共享同一底层 LLM，通过专属系统提示词获得角色定位。

---

## 功能特性

- **多智能体协作**：Captain 统一调度，子 Agent 各司其职，自动流转攻击阶段
- **PTY 本地执行**：以伪终端模式运行命令，完整支持交互式工具（ssh/sudo/msfconsole 等），不破坏 TUI
- **6 个命令执行工具**：`submit_command` / `start_command` / `get_status` / `get_output` / `intervene_command` / `kill_command`，支持异步任务管理
- **MCP 工具热扩展**：支持 SSE、Streamable HTTP、Stdio 三种 MCP 传输协议，`/flush` 命令无需重启即可重载工具
- **自动上下文压缩**：基于 tiktoken 精确计数，超过上下文窗口 70% 时自动异步摘要，绿色高亮摘要结果
- **流式 & 思维链输出**：实时渲染 Token 流，DeepSeek / Claude 思维链内容黄色高亮显示
- **安全护栏**：内置 Prompt 级别的高危操作拦截与确认机制
- **跨平台构建**：支持 Linux/macOS/Windows，x64 与 arm64

---

## 快速开始

### 前置依赖

- Go 1.24+（编译时）
- 有效的 LLM API Key（OpenAI 兼容 API 或 Anthropic）

### 运行

1. **下载或编译**可执行文件（见 [构建](#构建) 章节）
2. **首次运行**：程序会在可执行文件同目录自动生成配置目录和配置文件，然后退出并提示修改配置

   ```
   .HackerTeam/
   └── HackerTeam.yaml   ← 主配置文件
   └── skills/           ← Agent 技能扩展目录（预留）
   └── HackerTeam.log    ← 框架运行日志
   ```

3. **修改配置**：填入 API Key、模型名称等（详见 [配置](#配置) 章节）
4. **再次运行**即可进入 TUI 交互界面

---

## 配置

配置文件路径：`<可执行文件目录>/.HackerTeam/HackerTeam.yaml`

```yaml
user:
  userid: "<自动生成的UUID>"

model:
  model: "deepseek-reasoner"        # 模型名称
  base_url: "https://api.deepseek.com"  # API 端点（兼容 OpenAI 格式）
  api_key: "YOUR_API_KEY"
  api_type: "openai"                # "openai" 或 "anthropic"
  stream: true                      # 是否启用流式输出
  context_window: 64000             # 上下文窗口大小（影响摘要触发阈值）

mcp:
  - enabled: false
    type: "sse"                     # "sse" 或 "streamable_http"
    endpoint: "http://localhost:3000/sse"
    headers:
      Authorization: "Bearer token"

stdin_mcp:
  - enabled: false
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-everything"]
```

### 模型提供商

| `api_type` | 支持的提供商 | 说明 |
|------------|-------------|------|
| `openai` | OpenAI、DeepSeek、Ollama、vLLM 及任意兼容 API | 通过 `base_url` 指向任意端点；DeepSeek 自动启用思维链回填 |
| `anthropic` | Anthropic Claude 系列 | 使用原生 Anthropic SDK |

---

## 工具集成

### LocalExec（内置，始终启用）

以 PTY 模式在本地执行系统命令，支持完整的交互式进程管理：

| 工具 | 功能 |
|------|------|
| `submit_command` | 提交命令（进程名 + 参数），返回任务 ID |
| `start_command` | 按 ID 启动命令 |
| `get_status` | 查询任务状态（ID/PID/ExitCode/Error） |
| `get_output` | 获取 stdout/stderr 输出（支持滑动窗口） |
| `intervene_command` | 向进程写 stdin 或发送信号（SIGINT/SIGTERM/SIGKILL） |
| `kill_command` | 强制结束进程 |

### MCP（按需配置）

通过配置文件添加任意 MCP 兼容的工具服务器：

- **SSE / Streamable HTTP**：连接远程 MCP 服务（网络传输）
- **Stdio**：启动本地子进程作为 MCP 服务（stdin/stdout 传输）

使用 `/flush` 命令可在不重启的情况下重新加载 MCP 工具配置。

---

## TUI 界面

```
┌─────────────────────────────────────────────────────┐
│ [StatusBar] Processing...                            │
├──────────────┬──────────────────────────────────────┤
│              │                                       │
│  [Sidebar]   │         [Message View]                │
│  操作提示     │   Agent 输出、工具调用、思维链          │
│  命令说明     │   （支持动态颜色、滚动）                │
│              │                                       │
├──────────────┴──────────────────────────────────────┤
│ ⇒ [输入区] Ctrl+Enter 提交                           │
└─────────────────────────────────────────────────────┘
```

**颜色约定：**
- 🟡 黄色：模型思维链内容（`<think>` 块）
- 🟢 绿色：上下文摘要通知
- 普通白色：Agent 正文输出

---

## 内置命令

在输入框中输入以下命令并按 `Ctrl+Enter` 执行：

| 命令 | 功能 |
|------|------|
| `/exit` | 退出程序 |
| `/new` | 开启新会话（清空对话历史） |
| `/flush` | 重新加载配置和 MCP 工具（热重载，无需重启） |
| `ESC` | 中断当前 Agent 执行 |

---

## 构建

### 一键交叉编译（PowerShell）

```powershell
.\build.ps1
```

输出目录：`release/`

| 目标平台 | 输出路径 |
|----------|----------|
| Linux arm64 | `release/linux-arm64/HackerTeam` |
| Linux x64 | `release/linux-x64/HackerTeam` |
| macOS arm64 (Apple Silicon) | `release/macos-arm64/HackerTeam` |
| macOS x64 | `release/macos-x64/HackerTeam` |
| Windows x64 | `release/windows-x64/HackerTeam.exe` |

### 手动编译（当前平台）

```bash
go build -ldflags "-s -w" -o HackerTeam .
```

---

## 技术栈

| 组件 | 库/框架 |
|------|---------|
| AI Agent 框架 | `trpc.group/trpc-go/trpc-agent-go` |
| MCP 协议 | `trpc.group/trpc-go/trpc-mcp-go` |
| TUI | `github.com/rivo/tview` + `github.com/gdamore/tcell/v2` |
| Anthropic SDK | `github.com/anthropics/anthropic-sdk-go` |
| OpenAI SDK | `github.com/openai/openai-go` |
| PTY 伪终端 | `github.com/creack/pty` |
| Token 计数 | `github.com/tiktoken-go/tokenizer` |
| 配置解析 | `gopkg.in/yaml.v2` |
| 日志 | `go.uber.org/zap` |
| UUID | `github.com/google/uuid` |

---

## 免责声明

本工具仅供授权的安全研究、渗透测试和教育目的使用。使用者须对自身行为负全部法律责任。未经授权对任何系统使用本工具均属违法行为。
