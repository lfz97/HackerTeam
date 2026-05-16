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

`Captain` 作为中枢调度，将目标分解后按攻击链顺序分发给子 Agent。**先 Recon 后 Scanner**，结果汇合到 Exploit 交叉验证后精准利用：

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
            PostExploit
         后渗透 + 横向移动
```

| Agent | 角色 | 职责 | 工具集 |
|-------|------|------|--------|
| **Captain** | 队长 | 任务规划、Agent 顺序调度、结果审核、最终定级裁决、生成报告 | 文件系统工具 |
| **Recon** | 侦察兵 | 子域名枚举、端口服务扫描、Web 指纹、目录爆破、被动情报（深度单点侦察） | LocalExec |
| **Scanner** | 脚本小子 | 使用自动化扫描工具批量广撒网，覆盖面广速度快，不追求精准（误报交给 Exploit） | LocalExec |
| **Exploit** | 老师傅 | 交叉比对 Recon + Scanner 结果，去误报后精准利用。负责漏洞最终技术定级 | LocalExec |
| **PostExploit** | 后渗透 | 提权、凭证窃取、内网探测、横向移动、持久化、痕迹清理 | LocalExec |

**协作关键：** Recon 告诉 Exploit "目标是什么"，Scanner 告诉 Exploit "哪里可能有洞"，Exploit 自主判断真伪后动手。

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
| **output_consensus** | 报告格式（MD only）、原始输出必须保存到 `TASK-{id}_raw/`、对话回复 JSON 通用字段、命令记录规范 |

**定级原则：** 抛弃 CVSS 评分，从攻击者实际获得的技术能力出发 —— 拿到 Shell / 拿到任意身份 / 拿到核心凭证 → Critical；能读敏感数据 / 能越权 / 能打通内网 → High。

### 首次运行目录结构

```
.HackerTeam/
├── HackerTeam.yaml          ← 主配置文件
├── HackerTeam.log
├── ReconSkills/             ← 各 Agent 独立的 Skill 目录
├── ScannerSkills/
├── ExploitSkills/
├── PostExploitSkills/
└── output/                  ← 任务报告与原始输出
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
> - **Recon** — nmap、fscan、dirsearch、subfinder、whatweb 等信息收集工具
> - **Scanner** — nuclei、sqlmap（`--batch` 模式）、nikto、wafw00f 等自动化扫描工具
> - **Exploit** — sqlmap（利用模式）、metasploit、hydra、反弹 Shell 工具等漏洞利用工具
> - **PostExploit** — mimikatz、chisel、ligolo-ng 等后渗透工具
>
> Agent 只会「知道」你写进 SKILL.md 的工具 —— 未配置的工具即使已安装也无法被正确调用。

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
