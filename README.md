# HackerTeam

> **仅供授权渗透测试和安全研究使用。请遵守当地法律法规。**

HackerTeam 是面向专业渗透测试人员的 **AI 驱动多智能体渗透测试平台**，将大语言模型与本地 PTY 命令执行结合，按 PTES 标准自动完成完整攻击链。

**核心优势：**
- **独立二进制部署** — 单文件，无运行时依赖，跨平台开箱即用
- **多 Agent 分工架构** — Captain 统一调度，4 个专职 Agent 各司其职
- **可扩展 Skill 配置** — 每个 Agent 独立 Skill 目录，支持 MCP 热扩展
- **内置完整操作工具** — PTY 命令管理 + 文件系统工具，覆盖渗透全流程

---

## 目录

- [独立二进制部署](#独立二进制部署)
- [多 Agent 分工架构](#多-agent-分工架构)
- [可扩展 Skill 配置](#可扩展-skill-配置)
  - [预置模板](#预置模板)
- [内置操作工具](#内置操作工具)
- [快速开始](#快速开始)
- [构建](#构建)

---

## 独立二进制部署

单个可执行文件，**无需安装 Go 运行时或任何依赖**，下载即用：

| 平台 | 架构 |
|------|------|
| Linux | x64 / arm64 |
| macOS | x64 / arm64 |
| Windows | x64 |

**首次运行**会在可执行文件同目录自动生成配置目录（含主配置文件及各 Agent 的 Skill 模板目录），随后退出并提示填入 API Key。修改配置后再次运行即进入 TUI 交互界面。

```
.HackerTeam/
├── HackerTeam.yaml          ← 主配置文件（填入 API Key 即可运行）
├── HackerTeam.log
├── ReconSkills/             ← 各子 Agent 独立的 Skill 目录
├── VulnAnalyzeSkills/
├── ExploitSkills/
└── PostExploitSkills/
```

> 目录结构及 Skill 模板的详细说明见[可扩展 Skill 配置](#可扩展-skill-配置)章节。

---

## 多 Agent 分工架构

`Captain` 作为中枢调度，将目标分解并下发给 4 个专职子 Agent，按 PTES 标准自动流转攻击阶段：

```
Recon → VulnAnalyst → Exploit → PostExploit → (内网循环)
```

| Agent | 职责 | 工具集 |
|-------|------|--------|
| **Captain** | 任务规划、子 Agent 调度、结果审核（置信度 ≥ 70%）、生成报告 | 文件系统工具 |
| **Recon** | 子域名枚举、端口扫描、Web 指纹、目录爆破、被动情报收集 | LocalExec |
| **VulnAnalyst** | CVE 关联、配置缺陷识别、Web 漏洞分析、攻击路径规划（只分析，不利用） | LocalExec |
| **Exploit** | SQLi / RCE / LFI / SSRF / XXE、认证绕过、反弹 Shell / WebShell | LocalExec |
| **PostExploit** | 提权、凭证窃取、横向移动、持久化、痕迹清理 | LocalExec |

Captain 通过 `<command>` JSON 标签向子 Agent 下发任务；子 Agent 将结果写入 `output/` 目录的结构化 Markdown 文件，Captain 读取并审核后决定下一步行动。

---

## 可扩展 Skill 配置

每个 Agent 拥有独立的 Skill 目录（`ReconSkills` / `VulnAnalyzeSkills` / `ExploitSkills` / `PostExploitSkills`），通过 **`SKILL.md`** 文件以知识注入方式告知 Agent 如何使用外部安全工具：

- Skill 注入到 Agent 的系统提示词，**工具本身仍通过 LocalExec 调用**，无需编写适配代码
- 在 `SKILL.md` 中描述工具用法、参数和输出格式，Agent 即可正确使用该工具
- 自由增删 Skill 文件后执行 `/flush` 即可热更新，无需重启

### 预置模板

首次运行时，每个 Agent 的 Skill 目录下会自动生成 `pentest-tools/SKILL.md.template`（`.template` 后缀防止被框架自动加载），内容如下：

```markdown
---
name: pentest-tools
description: Quick lookup of pentest tools.
---

# Pentest Tools

- **nmap**        — Network Scanning   — /opt/pentest/nmap/nmap
- **fscan**       — Network Scanning   — /opt/pentest/fscan/fscan
- **dirsearch**   — Web Testing        — /opt/pentest/dirsearch/dirsearch.py
- **sqlmap**      — Web Testing        — /opt/pentest/sqlmap/sqlmap.py
- **nuclei**      — Vuln Scanning      — /opt/pentest/nuclei/nuclei
```

使用时将 `SKILL.md.template` 重命名为 `SKILL.md` 即生效。

> **强烈建议按需定制：** 模板中的路径为占位示例，请替换为工具的实际安装路径，并根据各 Agent 职责配置对应的工具：
> - **Recon** — nmap、fscan、dirsearch、subfinder、assetfinder 等信息收集工具
> - **VulnAnalyst** — nuclei、searchsploit、CVEMap 等漏洞分析工具
> - **Exploit** — sqlmap、metasploit、hydra 等漏洞利用工具
> - **PostExploit** — mimikatz、chisel、ligolo-ng 等后渗透工具
>
> Agent 只会「知道」你写进 SKILL.md 的工具 —— 未配置的工具即使已安装也无法被正确调用。

---

## 内置操作工具

### LocalExec — PTY 命令管理（6 个工具）

以伪终端（PTY）模式执行命令，完整支持 `ssh`、`sudo`、`msfconsole` 等需要交互式终端的工具，且不破坏 TUI 布局：

| 工具 | 功能 |
|------|------|
| `submit_command` | 提交命令（进程名 + 参数），返回任务 ID |
| `start_command` | 按 ID 在 PTY 中启动命令 |
| `get_status` | 查询任务状态（pending / running / done / failed / killed） |
| `get_output` | 获取 stdout/stderr 输出（支持滑动窗口） |
| `intervene_command` | 向进程写 stdin 或发送信号（SIGINT / SIGTERM / SIGKILL） |
| `kill_command` | 强制结束进程 |

### Captain 文件系统工具（5 个）

Captain 使用这些工具读写子 Agent 产出的报告文件：

| 工具 | 功能 |
|------|------|
| `PWD` | 打印当前工作目录 |
| `CD` | 切换工作目录 |
| `LS` | 列出目录内容 |
| `ReadFile` | 读取文件内容（支持滑动窗口） |
| `WriteFile` | 创建或覆写文件 |

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

## 构建

```sh
# 当前平台
go build -ldflags "-s -w" -o HackerTeam .

# 跨平台（PowerShell）
.\build.ps1

# 跨平台（Make）
make                 # 全平台
make linux-x64
make macos-arm64
make windows-x64
```
