package tui

// ── 斜杠命令注册表与分发 ─────────────────────────────────
//
// 命令语义与维护都在前端，引擎只暴露能力接口（见 engine.go 的 EngineView）
// ——不再有经 inputCh 的字符串 magic word。清单与分发共用 slashRegistry
// 同一条目：分成两份会出现「浮层能选到、分发不认」的静默错配。

import (
	tea "charm.land/bubbletea/v2"
)

// slashCmd 一条斜杠命令：命令文本 + 浮层里显示的描述 + 执行体。
type slashCmd struct {
	cmd  string
	desc string
	run  func(t *TUI) tea.Cmd
}

// slashRegistry 前端自持的斜杠命令注册表。编译期常量，启动第一刻即非空——
// 不存在"引擎还没发布条目"的空窗，也不存在拉取失败的中间态。
// 顺序即浮层里的呈现顺序。
// NewConversation/ClearRecords 在 running 时拒绝并自行 setNotice；返回值
// 决定是否清空展开状态——拒绝时历史与展开态都必须保持原样。
var slashRegistry = []slashCmd{
	{"/new", "start a new session", func(t *TUI) tea.Cmd {
		// /new 只换 session id，不清消息记录（旧历史仍留在屏上）；块 ID 按
		// 记录序号生成、追加记录不改变旧 ID，因此不要 reset 展开态。
		t.engine.NewConversation() //running 时引擎拒绝并自行 setNotice
		return nil
	}},
	{"/clear", "clear the screen", func(t *TUI) tea.Cmd {
		if t.engine.ClearRecords() { //running 时引擎拒绝：历史没清，展开态也不能清
			t.resetExpand()
		}
		return nil
	}},
	{"/resume", "resume a previous session", func(t *TUI) tea.Cmd {
		t.openResume()
		return nil
	}},
	{"/exit", "exit HackerTeam", func(t *TUI) tea.Cmd {
		//Shutdown 刻意不加 isRunning 守卫：长任务运行中 /exit 是常态，
		//加了守卫会整段跳过清理，MCP stdio 子进程与 localexec PTY 作业活过进程。
		t.engine.Shutdown()
		return tea.Quit
	}},
}

// dispatchSlash 分发一条斜杠命令（调用方已 TrimSpace）。返回 (tea.Cmd, handled)：
// /exit 返回 tea.Quit；未登记的命令 handled=false，由调用方按普通输入兜底提交。
//
// 已识别的命令一律 handled=true，绝不与能力接口的返回值（执行成功与否）合并成一个
// 布尔：running 时 NewConversation/ClearRecords 返回 false，若把它当 handled=false
// 就会让命令文本泄漏进 submitInput 当普通 prompt 提交。拒绝语义由引擎侧 setNotice
// 表达，前端只管"这条输入是命令、不是对话"。
func (t *TUI) dispatchSlash(input string) (tea.Cmd, bool) {
	for _, sc := range slashRegistry {
		if sc.cmd == input {
			return sc.run(t), true
		}
	}
	return nil, false
}
