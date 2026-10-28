# HackerTeam

> **仅供授权渗透测试和安全研究使用。请遵守当地法律法规。**

HackerTeam 是面向专业渗透测试人员的 **AI 驱动渗透测试平台**，将大语言模型与本地 PTY 命令执行结合，由 Operator 在共享共识约束下自己动手推进任务，并按需把可并行的广度工作交给无状态 Worker。

**核心优势：**
- **独立二进制部署** — 单文件，无运行时依赖，Linux/macOS 开箱即用
- **Operator 自执行** — 主 agent 直接持有 shell、读原始输出、构建攻击链，不做只转发任务的调度器
- **无状态 Worker 并行** — 同一套工具/技能/模型的临时副本，无记忆、无委派，适合批量枚举与独立复核
- **可扩展 Skill 配置** — Operator 与 Worker 共享一个 Skill 目录，支持 MCP 热扩展
- **内置完整操作工具** — PTY 命令管理 + 文件系统工具，覆盖渗透全流程

---

## 系统要求

- **操作系统**：Linux / macOS（必需）。HackerTeam 核心依赖 PTY（伪终端）来运行 `ssh`、`sudo`、`msfconsole` 等交互式渗透工具——Windows 原生不支持 PTY，命令执行会降级为普通管道（pipe），交互式工具无法正常工作。**Windows 用户请使用 [WSL](https://learn.microsoft.com/zh-cn/windows/wsl/)（WSL 2 推荐）**，在 WSL 中可获得完整的 PTY 支持和 Linux 渗透工具链。
- **Go 1.26+**（仅源码编译需要；亦可直接下载预编译二进制）
- 有效的 LLM API Key（OpenAI 兼容接口 或 Anthropic）

---

## 目录

- [Operator + Worker 架构](#operator--worker-架构)
- [可扩展 Skill 配置](#可扩展-skill-配置)
- [内置操作工具](#内置操作工具)
- [快速开始](#快速开始)
- [构建](#构建)

---

## Operator + Worker 架构

### 分工

系统只有两个角色，共用同一份实现：同一套模型参数、同一批执行工具、同一个技能库、同一套上下文压缩策略。差别只在提示词、额外工具与状态权限。

- **Operator** — 唯一与用户对话的 agent，自己执行工作：持有 shell、读原始输出、构建攻击链、审核证据、定级、出最终报告；同时维护 `todo_write` 计划和长期记忆。
- **Worker** — 一次性无状态助手。它只看得到 Operator 写进 brief 的内容，没有记忆、没有 todo、没有调度权；做完一件事，把报告与结构化结论落盘后退出。

Worker 不是另一套专家实现，也不是调度器的延伸：它运行在隔离会话里，但与 Operator 共享同一个 LocalExec 作业表——它起的后台任务，Operator 用 `CommandStatus` 直接能查。交互式会话（ssh、msfconsole、反弹 shell）只能由 Operator 持有。

```
用户目标 + 当前证据
        │
  Operator（todo_write + 记忆）
        │  自己跑命令 / 读输出 / 构建攻击链
        │  只有广度或可并行的任务才派发
        ▼
  start_task_run ──▶ Worker（隔离会话，无记忆）
        │                  │
        │                  └─ 落盘：TASK-<id>_result.json
        │                            TASK-<id>_<time>_<type>_result.md
        │                            TASK-<id>_<time>_raw/
        ◀── wait / get / read_transcript（取回状态与进度）
        │
  读文件复核 → 更新 todo → 继续 / 重派 / 自己接手
```

| Agent | 职责 | 工具 |
|-------|------|------|
| **Operator** | 规划、亲自执行、按需派 Worker、审核证据、定级、出报告、维护记忆 | 全部执行工具 + Skills + MCP + `todo_write` + memory 工具 + taskrun 调度工具 |
| **Worker** | 执行一个自包含 brief，产出报告与结构化结论 | 全部执行工具 + Skills + MCP（无 memory / todo / 调度权） |

**派发是 pull 模型：** `start_task_run` 默认 `async`，立即返回 run id；只有下一步决策依赖结果时才用 `sync`。Operator 用 `wait_task_run(timeout)` 做有界等待、`get_task_run` 查状态、`read_task_run_transcript` 看运行中进度、`cancel_task_run` 停止。等待超时不会取消任务。

**结果以文件为准：** 框架只保留 Worker 叙述的前约 4000 字符，`result` 只能当存活信号。真正的结论在 `TASK-<task_id>_result.json`，完整报告与原始输出在 `TASK-<task_id>_<time>_<type>_result.md` / `TASK-<task_id>_<time>_raw/`。Worker 返回 "done" 不构成证据，Operator 必须读文件复核。

**派发边界：** 一两条命令能做完的事 Operator 自己跑；brief 必须写清 `task_id`、`OBJECTIVE`、`SCOPE`、`INPUTS`、`DELIVERABLE`、`SUCCESS`。缺 `INPUTS` 对应的真实文件，就还没准备好派发。

### 共享共识

Operator 与 Worker 注入同一套公共提示词与共识文件：

```
service/engine/prompts/common/
├── env.md                   # 执行环境信息
├── command_execution.md     # 命令执行规范
├── vuln_consensus.md        # 漏洞定义与定级共识
└── output_consensus.md      # 结果输出共识
```

| 共识文件 | 核心内容 |
|----------|----------|
| **vuln_consensus** | 漏洞定义（攻击者获得了什么技术能力）、严重性等级（Critical/High/Medium/Low）纯技术标准、执行者的定级职责、定级冲突解决流程、Confidence 语义 |
| **output_consensus** | 报告格式（MD only）、原始输出必须保存到 `TASK-{id}_*_raw/`、对话回复 JSON 通用字段、命令记录规范、**漏洞结构化块**（YAML 格式，为 PoC 脚本提供满信号数据：entry_point / payload / verification / prerequisites / evidence） |

**定级原则：** 抛弃 CVSS 评分，从攻击者实际获得的技术能力出发 —— 拿到 Shell / 拿到任意身份 / 拿到核心凭证 → Critical；能读敏感数据 / 能越权 / 能打通内网 → High。

**漏洞结构化块：** 产出漏洞的 Operator 或 Worker 必须在报告中为每个漏洞输出 YAML 结构化块，包含完整的攻击入口、payload、验证方式、前置条件和证据。这是 PoC 脚本生成的唯一数据源——结构化块不完整，脚本就无法产出。Operator 审核时会检查结构化块完整性，模糊描述会被打回重写。

### 首次运行目录结构

```
.HackerTeam/
├── HackerTeam.yaml          ← 主配置文件
├── HackerTeam.log
├── memory.db                ← 长期记忆数据库
├── Skills/                  ← 唯一技能目录（Operator 与 Worker 共用，内容由用户添加）
│   └── pentest-tools/       ← 工具清单占位（SKILL.md.template）
└── output/                  ← 任务报告与原始输出
    └── poc_scripts/         ← 复现脚本
```

---

## 可扩展 Skill 配置

Operator 与 Worker 共享 `.HackerTeam/Skills/` 一个目录，通过 **`SKILL.md`** 文件以知识注入方式告知 Agent 如何使用外部安全工具：

- Skill 注入到 Agent 的系统提示词，**工具本身仍通过 LocalExec 调用**，无需编写适配代码
- 在 `SKILL.md` 中描述工具用法、参数和输出格式，Agent 即可正确使用该工具
- 增删 Skill 文件后下一轮对话自动生效（每轮 run 重建配置/技能/MCP 工具集，无需 `/flush` 或重启）

### 预置模板

首次运行时，`.HackerTeam/Skills/` 下会自动生成 `pentest-tools/SKILL.md.template`（`.template` 后缀防止被框架自动加载），内容如下：

```markdown
---
name: pentest-tools
description: >-
  Quick lookup of pentest tools.
  Use when you need the name, type, or path of any installed pentest tool.
---

# Pentest Tools

- **<tool example>**
  - category: <tool category>
  - path: `<tool absolute path>`
  - symlink: `<tool symlink if you need>`
  - repo: <tool github or gitee repo>
```

使用时将 `SKILL.md.template` 重命名为 `SKILL.md` 即生效。

> Agent 只会「知道」你写进 SKILL.md 的工具 —— 未配置的工具即使已安装也无法被正确调用。

### 知识型 Skill 由用户添加

Skills 目录默认只有 `pentest-tools` 占位模板，不预置任何红队知识。通用概念模型已经具备，精确细节会随版本过期，而各团队的 SOP 差异很大——预置一套通用知识，收益低于维护成本。

需要时在 `.HackerTeam/Skills/<name>/SKILL.md` 下自建技能，索引在下一轮对话自动生效。写什么：特有 SOP、专有工具用法、版本边界、成功/失败签名；不必写：通用概念、工具教程、能在官方文档里查到的参数。

- 知识以 `SKILL.md` + `references/` 形式注入，references 仅按需读取，不占每轮提示词
- Operator 与 Worker 读同一份 `Skills/` 索引；增删改直接在 `.HackerTeam/Skills/` 下对应目录操作，下一轮对话生效
- 删除某个技能目录后 Operator 与 Worker 同时失去它

### 配置示例

以下是 `Skills/pentest-tools/SKILL.md` 的配置示例，按工具类别分组；Operator 与 Worker 共用同一份，可把各类清单合并写进同一个文件：

**侦察类工具** — `.HackerTeam/Skills/pentest-tools/SKILL.md`

```markdown
---
name: pentest-tools
description: >-
  Quick lookup of pentest tools.
  Use when you need the name, type, or path of any installed pentest tool.
---

# Pentest Tools

- **nmap**
  - category: Network Scanning
  - path: `/opt/pentest/nmap/nmap`
  - symlink: `/usr/bin/nmap`
  - repo: https://github.com/nmap/nmap

- **subfinder**
  - category: Subdomain Enumeration
  - path: `/opt/pentest/subfinder/subfinder`
  - repo: https://github.com/projectdiscovery/subfinder

- **whatweb**
  - category: Web Fingerprinting
  - path: `/opt/pentest/whatweb/whatweb`
  - repo: https://github.com/urbanadventurer/WhatWeb

- **dirsearch**
  - category: Directory Brute-forcing
  - path: `/opt/pentest/dirsearch/dirsearch.py`
  - repo: https://github.com/maurosoria/dirsearch

- **fscan**
  - category: Network Scanning
  - path: `/opt/pentest/fscan/fscan`
  - repo: https://github.com/shadow1ng/fscan
```

**扫描类工具** — `.HackerTeam/Skills/pentest-tools/SKILL.md`

```markdown
---
name: pentest-tools
description: >-
  Quick lookup of pentest tools.
  Use when you need the name, type, or path of any installed pentest tool.
---

# Pentest Tools

- **nuclei**
  - category: Vulnerability Scanning
  - path: `/opt/pentest/nuclei/nuclei`
  - repo: https://github.com/projectdiscovery/nuclei

- **sqlmap**
  - category: SQL Injection Detection
  - path: `/opt/pentest/sqlmap/sqlmap.py`
  - repo: https://github.com/sqlmapproject/sqlmap

- **nikto**
  - category: Web Server Audit
  - path: `/opt/pentest/nikto/nikto`
  - repo: https://github.com/sullo/nikto

- **wafw00f**
  - category: WAF Detection
  - path: `/opt/pentest/wafw00f/wafw00f`
  - repo: https://github.com/EnableSecurity/wafw00f

- **dirsearch**
  - category: Directory Brute-forcing
  - path: `/opt/pentest/dirsearch/dirsearch.py`
  - repo: https://github.com/maurosoria/dirsearch
```

**利用类工具** — `.HackerTeam/Skills/pentest-tools/SKILL.md`

```markdown
---
name: pentest-tools
description: >-
  Quick lookup of pentest tools.
  Use when you need the name, type, or path of any installed pentest tool.
---

# Pentest Tools

- **sqlmap**
  - category: SQL Injection Exploitation
  - path: `/opt/pentest/sqlmap/sqlmap.py`
  - repo: https://github.com/sqlmapproject/sqlmap

- **msfconsole**
  - category: Exploitation Framework
  - path: `/opt/pentest/metasploit/msfconsole`
  - repo: https://github.com/rapid7/metasploit-framework

- **hydra**
  - category: Credential Brute-forcing
  - path: `/opt/pentest/hydra/hydra`
  - repo: https://github.com/vanhauser-thc/thc-hydra

- **msfvenom**
  - category: Payload Generation
  - path: `/opt/pentest/metasploit/msfvenom`
  - repo: https://github.com/rapid7/metasploit-framework
```

**后渗透类工具** — `.HackerTeam/Skills/pentest-tools/SKILL.md`

```markdown
---
name: pentest-tools
description: >-
  Quick lookup of pentest tools.
  Use when you need the name, type, or path of any installed pentest tool.
---

# Pentest Tools

- **mimikatz**
  - category: Credential Dumping
  - path: `/opt/pentest/mimikatz/mimikatz.exe`
  - repo: https://github.com/gentilkiwi/mimikatz

- **chisel**
  - category: Tunneling
  - path: `/opt/pentest/chisel/chisel`
  - repo: https://github.com/jpillora/chisel

- **ligolo-ng**
  - category: Tunneling
  - path: `/opt/pentest/ligolo/proxy`
  - repo: https://github.com/nicocha30/ligolo-ng

- **pypykatz**
  - category: Credential Dumping
  - path: `/opt/pentest/pypykatz/pypykatz`
  - repo: https://github.com/skelsec/pypykatz

- **impacket-psexec**
  - category: Lateral Movement
  - path: `/opt/pentest/impacket/psexec.py`
  - repo: https://github.com/fortra/impacket
```


---

## 内置操作工具

### LocalExec — PTY 命令管理（5 个工具）

以伪终端（PTY）模式异步执行命令，完整支持 `ssh`、`sudo`、`msfconsole` 等需要交互式终端的工具，且不破坏 TUI 布局：

| 工具 | 功能 |
|------|------|
| `CommandRun` | 执行命令，最多阻塞 20 秒：期间结束就直接返回状态/退出码/输出，超时则转后台并返回任务 ID |
| `CommandStatus` | 查询任务状态（running / done / failed / killed）；`wait_seconds` 可阻塞等到完成再返回 |
| `CommandOutput` | 获取 stdout/stderr 输出；超过内联上限的整份落盘，只回头部预览与文件路径 |
| `CommandIntervene` | 向进程写 stdin 或发送信号（SIGHUP / SIGINT / SIGQUIT / SIGABRT / SIGTERM / SIGKILL） |
| `CommandKill` | 强制结束进程 |

> **平台说明**：PTY 在 Linux/macOS 上完整可用。Windows 原生不支持 PTY，LocalExec 会自动降级为普通管道（pipe），此时 `ssh`、`sudo`、`msfconsole` 等交互式工具无法正常工作，且 AI 操控 PowerShell 的失败率显著偏高。**使用 HackerTeam 请务必在 Linux/macOS 或 WSL 中运行。**

### 工具集

Operator 与 Worker 挂载同一套执行工具，并共享同一个 LocalExec 作业表：

| 工具集 | 工具 | 功能 |
|--------|------|------|
| **FileSystem** | `LS` / `Mkdir` / `CP` / `MV` / `Glob` | 文件系统导航与操作 |
| **FileOps** | `read` / `write` / `edit` / `search` / `delete` / `stat` / `diff` | 文件读写与搜索 |
| **LocalExec** | `run` / `status` / `output` / `intervene` / `kill` | PTY 命令管理 |
| **Skills** | 知识注入 | 将 SKILL.md 内容注入系统提示词 |
| **MCP** | 配置声明的工具 | 安装的 MCP server 工具，实例共享 |

Operator 另外挂载三组状态与调度工具，Worker 一个都没有：

| 工具 | 功能 |
|------|------|
| `todo_write` | 维护跨轮任务计划（Worker 没有 todo） |
| memory 工具 | `memory_search` / `memory_load` / `memory_update` 等长期记忆读写 |
| taskrun 工具 | `start_task_run` / `list_task_runs` / `get_task_run` / `wait_task_run` / `read_task_run_transcript` / `cancel_task_run` |

Worker 只做短任务：没有跨轮记忆、不能派活、不能向用户提问。长任务、交互式会话和需要历史上下文的判断都由 Operator 自己承担。

---

## 快速开始

### 前置条件

- **操作系统**：Linux / macOS。Windows 用户请在 WSL 中运行（见下方说明）。
- 有效的 LLM API Key（OpenAI 兼容接口 或 Anthropic）

### Windows 用户必读

HackerTeam 的核心依赖 PTY（伪终端）在 Windows 上不可用。交互式渗透工具（`msfconsole`、`ssh`、`sudo` 等）在 Windows 原生环境下无法正常工作，AI 操控 PowerShell/cmd 的失败率也显著偏高。

**请先安装 WSL，然后在 WSL 中运行 HackerTeam：**

```powershell
# 在 PowerShell 中安装 WSL（如已安装可跳过）
wsl --install

# 进入 WSL，后续所有操作在 WSL 终端中进行
wsl
```

### 三步启动

**1. 下载可执行文件**（或自行编译，见[构建](#构建)章节）

**2. 首次运行**，自动生成配置文件后退出：

```sh
./HackerTeam
# → 生成 .HackerTeam/HackerTeam.yaml，请修改后重新运行
```

**3. 修改配置**，填入 API Key 后再次运行：

```yaml
# 用户配置（首次运行自动生成 UUID）
user:
  userid: "{USERID}"

# 模型配置
model:
  model: "deepseek-reasoner"            # 模型名称
  baseurl: "https://api.deepseek.com"   # OpenAI 兼容端点，或 https://api.anthropic.com
  apikey: "YOUR_API_KEY"
  apitype: "openai"                     # "openai" 或 "anthropic"
  anthropicAuthHeaderTransfer: false    # true = 走 Authorization: Bearer 认证；false = 走 X-Api-Key（代理/网关需要 Bearer 时开启）
  contextwindow: 128000                 # 上下文窗口大小，请参考模型文档设置，影响自动摘要触发时机
  stream: true                          # 流式输出，开启后可实时看到推理过程和工具调用
  maxtokens: 12800                      # 每次请求的最大生成 token 数，默认 12800
  httptimeout: 600                      # HTTP 请求超时，单位/秒
  reasoning_effort: ""                  # 思考强度档位（low/medium/high/max），留空不下发该参数
  show_reasoning: false                 # 是否显示推理内容，默认 false（不显示）

# MCP server 配置（可选）。启用的 server 挂载给 Operator 与 Worker，实例跨 agent 共享
# http_mcp：SSE / streamable_http 传输
#http_mcp:
#  - enabled: true
#    name: "example_http"      # 必须唯一，决定工具前缀 {name}_{toolName}
#    type: "streamable_http"   # sse 或 streamable_http
#    endpoint: "http://127.0.0.1:8080/mcp"
#    headers: {}
#    description: "示例 HTTP MCP server"

# stdin_mcp：stdio 传输（每个 server 全进程一个子进程，懒连接）
#stdin_mcp:
#  - enabled: true
#    name: "example_stdio"     # 必须唯一，决定工具前缀 {name}_{toolName}
#    command: "uvx"
#    args: ["mcp-server-fetch"]
#    description: "示例 stdio MCP server"
```

| `apitype` | 适用提供商 |
|-----------|------------|
| `openai` | OpenAI、DeepSeek、Ollama、vLLM 及任意兼容 API |
| `anthropic` | Anthropic Claude 系列（使用原生 SDK） |

> 完整字段以首次运行自动生成的 `.HackerTeam/HackerTeam.yaml` 为准（含注释）。

### TUI 内置命令

| 命令 | 功能 |
|------|------|
| `/new` | 开始新会话并清屏（会话上下文与屏幕记录一并重置） |
| `/clear` | 清屏（只清屏幕记录，会话上下文保留） |
| `/resume` | 恢复历史会话（弹框选择） |
| `/exit` | 退出 |
| `ESC` | 中断当前回合 |
| `鼠标左键点击 ▸ 头部` | 展开/收起思考块与工具块（默认折叠） |
| `/` | 命令列表（继续输入即前缀过滤；Enter 补全选中项，再按 Enter 执行） |

> 配置/Skill/MCP 工具集在每轮对话自动重载，无需手动刷新（旧版 `/flush` 已移除）。

---

## 长期记忆

长期记忆只属于 Operator：由它主动写入 SQLite，下轮对话开始前预加载相关条目：

- **显式写入**：记忆只由 Operator 用 memory 工具主动写入，不会自动落库
- **智能预加载**：每轮开始注入 10 条记忆（`[MEMORY]` 块），其余通过 `memory_search` / `memory_load` 检索
- **SQLite 持久化**：单文件 `memory.db`，零运维，重启不丢失
- **Worker 无记忆**：它只看到 brief 中显式写出的内容，读完即弃

## 构建

```bash
# Linux / macOS
./build.sh

# Windows PowerShell（不推荐，建议在 WSL 中构建）
.\build.ps1
```

产物输出到 `release/` 目录。


## 部署与迁移

HackerTeam 是**单文件部署**——一个二进制 + 一个配置目录就是完整的运行实例，无需 Docker、数据库或任何运行时依赖。

### 部署

```bash
# 编译或下载二进制后，放到任意目录直接运行
./HackerTeam
# 首次运行自动生成 .HackerTeam/ 配置目录
```

只需确保二进制**所在目录可读写**。

### 迁移

把二进制和 `.HackerTeam/` 目录打包拷到另一台机器即可，**所有数据完整保留**：

```bash
scp HackerTeam user@new-host:/opt/hackerteam/
scp -r .HackerTeam user@new-host:/opt/hackerteam/
```

`.HackerTeam/` 目录包含：

| 文件/目录 | 内容 |
|-----------|------|
| `HackerTeam.yaml` | API Key、模型配置 |
| `memory.db` | 长期记忆（渗透过程的关键信息提取） |
| `Skills/` | Operator 与 Worker 共用的 Skill 定义 |
| `HackerTeam.log` | 运行日志 |

> **注意**：`HackerTeam.yaml` 中的 API Key 是敏感信息，迁移前请确保目标环境安全。


## 许可证

仅限授权安全测试和研究使用。详见项目根目录 LICENSE 文件。
