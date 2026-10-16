package tui

// ── 斜杠命令注册表与分发 ─────────────────────────────────
//
// 命令语义与维护都在前端：清单（帮助浮层的条目来源）与分发（dispatchSlash）
// 同处一地，引擎只暴露能力接口（见 engine.go 的 EngineView）——不再有经
// inputCh 的字符串 magic word。新增一条命令只改本文件：slashCommands 加一行 +
// dispatchSlash 加一个 case，引擎侧按需补一个能力接口。

import (
	tea "charm.land/bubbletea/v2"
)

// slashCommand 一条斜杠命令：命令文本 + 帮助浮层里显示的描述。
type slashCommand struct {
	cmd  string
	desc string
}

// slashCommands 前端自持的斜杠命令注册表。帮助浮层（skills.go）直接以它为条目，
// 不经引擎下发——清单是编译期常量，启动期就有，不存在"引擎还没发布"的空窗。
var slashCommands = []slashCommand{
	{cmd: "/exit", desc: "Exit"},
	{cmd: "/new", desc: "start a new session"},
	{cmd: "/clear", desc: "clear the screen"},
	{cmd: "/resume", desc: "resume a previous session"},
}

// dispatchSlash 分发一条斜杠命令（调用方已 TrimSpace）。返回 (tea.Cmd, handled)：
// /exit 返回 tea.Quit；未知命令 handled=false，由调用方按普通输入兜底提交。
//
// 已识别的命令一律 handled=true，绝不与能力接口的返回值（执行成功与否）合并成一个
// 布尔：running 时 NewConversation/ClearRecords 返回 false，若把它当 handled=false
// 就会让命令文本泄漏进 submitInput 当普通 prompt 提交。拒绝语义由引擎侧 setNotice
// 表达，前端只管"这条输入是命令、不是对话"。
func (t *TUI) dispatchSlash(cmd string) (tea.Cmd, bool) {
	switch cmd {
	case "/exit":
		t.engine.Shutdown()
		return tea.Quit, true
	case "/new":
		t.engine.NewConversation() //running 时引擎拒绝并自行 setNotice
		return nil, true
	case "/clear":
		t.engine.ClearRecords() //running 时引擎拒绝并自行 setNotice
		return nil, true
	case "/resume":
		t.openResume()
		return nil, true
	}
	return nil, false
}
