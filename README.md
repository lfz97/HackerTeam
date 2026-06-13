# HackerTeam

> **仅供授权渗透测试和安全研究使用。请遵守当地法律法规。**

HackerTeam 是面向专业渗透测试人员的 **AI 驱动多智能体渗透测试平台**，将大语言模型与本地 PTY 命令执行结合，按 PTES 标准自动完成完整攻击链。

**核心优势：**
- **独立二进制部署** — 单文件，无运行时依赖，跨平台开箱即用
- **共识驱动的多 Agent 协作** — 统一的漏洞定级共识和结果输出共识，Agent 间职责清晰、协作一致
- **可扩展 Skill 配置** — 每个 Agent 独立 Skill 目录，支持 MCP 热扩展
- **内置完整操作工具** — PTY 命令管理 + 文件系统工具，覆盖渗透全流程

---

## 目录

- [共识驱动的多 Agent 架构](#共识驱动的多-agent-架构)
- [可扩展 Skill 配置](#可扩展-skill-配置)
- [内置操作工具](#内置操作工具)
- [快速开始](#快速开始)
- [构建](#构建)

---

## 共识驱动的多 Agent 架构

### Agent 分工与协作流

`Captain` 作为中枢调度，将目标分解后按攻击链顺序分发给子 Agent。**先 Recon 后 Scanner**，结果汇合到 Exploit 交叉验证后精准利用，最后由 Reproducer 为所有已确认漏洞生成可运行的 Python 复现脚本：

```
              Captain
                 |
               Recon (深度侦察)
                 |
              Scanner (自动化广撒网)
                 |
            Exploit (老师傅)
         交叉比对 + 去误报 + 精准利用
                 |
         ┌───────┴───────┐
         │               │
    PostExploit      Reproducer (Batch1)
  后渗透+横向移动   为 Scanner+Exploit 确认的漏洞写脚本
         │
    Reproducer (Batch2)
  为 PostExploit 的提权/横向等写脚本
```

| Agent | 角色 | 职责 | 工具集 |
|-------|------|------|--------|
| **Captain** | 队长 | 任务规划、Agent 顺序调度、结果审核、最终定级裁决、生成报告 | 文件系统工具 |
| **Recon** | 侦察兵 | 子域名枚举、端口服务扫描、Web 指纹、目录爆破、被动情报（深度单点侦察） | LocalExec + Skills |
| **Scanner** | 脚本小子 | 使用自动化扫描工具批量广撒网，覆盖面广速度快，不追求精准（误报交给 Exploit） | LocalExec + Skills |
| **Exploit** | 老师傅 | 交叉比对 Recon + Scanner 结果，去误报后精准利用。负责漏洞最终技术定级 | LocalExec + Skills |
| **PostExploit** | 后渗透 | 提权、凭证窃取、内网探测、横向移动、持久化、痕迹清理 | LocalExec + Skills |
| **Reproducer** | 复现员 | 读取前序 Agent 报告中的漏洞结构化数据，为每个已确认漏洞生成 PoC + Exploit 双模式 Python 脚本 | LocalExec（仅语法检查） |

**协作关键：** Recon 告诉 Exploit "目标是什么"，Scanner 告诉 Exploit "哪里可能有洞"，Exploit 自主判断真伪后动手，Reproducer 根据前序报告中的漏洞结构化块自动生成可运行的复现脚本。

### Agent 间共识体系

所有 Agent 共享三份共识文件，通过系统提示词自动注入：

```
bootstrap/prompts/common/
├── env.md                   # 执行环境信息
├── command_execution.md     # 命令执行规范
├── vuln_consensus.md        # 漏洞定义与定级共识
└── output_consensus.md      # 结果输出共识
```

| 共识文件 | 核心内容 |
|----------|----------|
| **vuln_consensus** | 漏洞定义（攻击者获得了什么技术能力）、严重性等级（Critical/High/Medium/Low）纯技术标准、各 Agent 定级职责、定级冲突解决流程、Confidence 语义 |
| **output_consensus** | 报告格式（MD only）、原始输出必须保存到 `TASK-{id}_raw/`、对话回复 JSON 通用字段、命令记录规范、**漏洞结构化块**（YAML 格式，为 Reproducer 提供满信号数据：entry_point / payload / verification / prerequisites / evidence） |

**定级原则：** 抛弃 CVSS 评分，从攻击者实际获得的技术能力出发 —— 拿到 Shell / 拿到任意身份 / 拿到核心凭证 → Critical；能读敏感数据 / 能越权 / 能打通内网 → High。

**漏洞结构化块：** 所有产出漏洞的 Agent（Scanner/Exploit/PostExploit）必须在报告中为每个漏洞输出 YAML 结构化块，包含完整的攻击入口、payload、验证方式、前置条件和证据。这是 Reproducer 生成脚本的唯一数据源——结构化块不完整，脚本就无法产出。Captain 在审核时会检查结构化块完整性，模糊描述会被打回重写。

### 首次运行目录结构

```
.HackerTeam/
├── HackerTeam.yaml          ← 主配置文件
├── HackerTeam.log
├── memory.db                ← 长期记忆数据库
├── ReconSkills/             ← 各 Agent 独立的 Skill 目录
├── ScannerSkills/
├── ExploitSkills/
├── PostExploitSkills/
├── ReproducerSkills/        ← Reproducer 专用（默认为空，不加载 pentest-tools）
└── output/                  ← 任务报告与原始输出
    └── poc_scripts/         ← Reproducer 生成的 Python 复现脚本
```

---

## 可扩展 Skill 配置

每个 Agent 拥有独立的 Skill 目录，通过 **`SKILL.md`** 文件以知识注入方式告知 Agent 如何使用外部安全工具：

- Skill 注入到 Agent 的系统提示词，**工具本身仍通过 LocalExec 调用**，无需编写适配代码
- 在 `SKILL.md` 中描述工具用法、参数和输出格式，Agent 即可正确使用该工具
- 自由增删 Skill 文件后执行 `/flush` 即可热更新，无需重启

### 预置模板

首次运行时，每个 Agent 的 Skill 目录下会自动生成 `pentest-tools/SKILL.md.template`（`.template` 后缀防止被框架自动加载），内容如下：

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

### 配置示例

以下为各 Agent 的 SKILL.md 实际配置示例，按职责分配工具：

**Recon** — `.HackerTeam/ReconSkills/pentest-tools/SKILL.md`

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

**Scanner** — `.HackerTeam/ScannerSkills/pentest-tools/SKILL.md`

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

**Exploit** — `.HackerTeam/ExploitSkills/pentest-tools/SKILL.md`

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

**PostExploit** — `.HackerTeam/PostExploitSkills/pentest-tools/SKILL.md`

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

**Reproducer** — `.HackerTeam/ReproducerSkills/` — 保持为空，不需要配置任何 Skill。

---

## 内置操作工具

### LocalExec — PTY 命令管理（5 个工具）

以伪终端（PTY）模式异步执行命令，完整支持 `ssh`、`sudo`、`msfconsole` 等需要交互式终端的工具，且不破坏 TUI 布局：

| 工具 | 功能 |
|------|------|
| `submit_command` | 异步执行命令（提交并立即启动），返回任务 ID 与运行状态 |
| `get_status` | 查询任务状态（running / done / failed / killed） |
| `get_output` | 获取 stdout/stderr 输出（支持滑动窗口） |
| `intervene_command` | 向进程写 stdin 或发送信号（SIGINT / SIGTERM / SIGKILL） |
| `kill_command` | 强制结束进程 |

### 所有 Agent 共享工具

除 Captain 外，所有子 Agent 均挂载完整工具集：

| 工具集 | 工具 | 功能 |
|--------|------|------|
| **FileSystem** | `PWD` / `CD` / `LS` / `Mkdir` / `CP` / `MV` / `Glob` | 文件系统导航与操作 |
| **FileOps** | `ReadFile` / `WriteFile` / `EditFile` / `SearchInFile` / `DeleteFile` / `FileStat` / `Diff` | 文件读写与搜索 |
| **Date** | `date_now` | 获取当前日期时间 |
| **LocalExec** | `submit_command` / `get_status` / `get_output` / `intervene_command` / `kill_command` | PTY 命令管理 |
| **Skills** | 知识注入 | 将 SKILL.md 内容注入系统提示词 |

Captain 仅挂载 FileSystem + FileOps + Date，不挂载 LocalExec 和 Skills。Reproducer 挂载 LocalExec 但仅用于语法检查（`python3 -m py_compile`），不执行实际攻击。

---

## 快速开始

### 前置条件

- 有效的 LLM API Key（OpenAI 兼容接口 或 Anthropic）

### 三步启动

**1. 下载可执行文件**（或自行编译，见[构建](#构建)章节）

**2. 首次运行**，自动生成配置文件后退出：

```sh
./HackerTeam
# → 生成 .HackerTeam/HackerTeam.yaml，请修改后重新运行
```

**3. 修改配置**，填入 API Key 后再次运行：

```yaml
model:
  model: "deepseek-reasoner"            # 模型名称
  baseurl: "https://api.deepseek.com"   # OpenAI 兼容端点，或 https://api.anthropic.com
  apikey: "YOUR_API_KEY"
  apitype: "openai"                     # "openai" 或 "anthropic"
  contextwindow: 64000
  stream: true
```

| `apitype` | 适用提供商 |
|-----------|------------|
| `openai` | OpenAI、DeepSeek、Ollama、vLLM 及任意兼容 API |
| `anthropic` | Anthropic Claude 系列（使用原生 SDK） |

### TUI 内置命令

| 命令 | 功能 |
|------|------|
| `/new` | 开始新会话 |
| `/flush` | 热重载配置、Skill 和 MCP 工具（无需重启） |
| `/exit` 或 `ESC` | 退出 |

---

## 长期记忆

每轮对话完成后自动提取关键信息并持久化到 SQLite，下次对话时智能注入上下文：

- **自动提取**：后台 LLM 异步分析对话，提取事实和事件记忆（增量去重）
- **智能预加载**：记忆少时全量注入，多了自动切语义检索
- **SQLite 持久化**：单文件 `memory.db`，零运维，重启不丢失
- **主动检索**：Captain 拥有 `memory_search` / `memory_load` 工具，可随时查找历史记忆

## 构建

```sh
# Linux
./build.sh

# Windows (PowerShell)
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
| `*Skills/` | 各 Agent 的 Skill 定义 |
| `HackerTeam.log` | 运行日志 |

> **注意**：`HackerTeam.yaml` 中的 API Key 是敏感信息，迁移前请确保目标环境安全。


## 许可证

仅限授权安全测试和研究使用。详见项目根目录 LICENSE 文件。
