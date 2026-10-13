package tui

// wire JSON 形状（与引擎侧 uistate.go 的 schema 对齐）+ 事件循环消息。
// demo 的 model/model.go 同位物：跨 goroutine 传递的消息类型与 JSON 契约
// 集中在本文件；引擎访问的封装在 engine.go，事件循环在 tui.go。

import (
	"encoding/json"
)

// ── 跨界 JSON 形状（与引擎侧 uistate.go 的 wire schema 对齐）──────────

// wireFatal / wireRunState 运行状态快照。
type wireFatal struct {
	Text    string `json:"text"`
	Style   string `json:"style"` // plain | error | success | exit
	WaitKey bool   `json:"waitKey"`
}

type wireRunState struct {
	Running bool       `json:"running"`
	Fatal   *wireFatal `json:"fatal"`
}

// wireNotice 通知栏槽位快照。
type wireNotice struct {
	Kind  string `json:"kind"` // none | new_conversation | cancelled | success | warning
	Text  string `json:"text"`
	SetAt string `json:"setAt"` // RFC3339
}

// wireHelpItem 帮助页一行。
type wireSkillItem struct {
	Cmd  string `json:"cmd"`
	Desc string `json:"desc"`
}

// wireRecord 消息日志记录：type 承载消息类型。
//   - delta / message：msg 为框架 model.Message 原样（框架自带 json 标签，两端同型收发）
//   - user / slash / warn / error / summary：text 为语义原文
type wireRecord struct {
	Type string          `json:"type"`
	Msg  json.RawMessage `json:"msg,omitempty"`
	Text string          `json:"text,omitempty"`
}

// wireToolCall 待结果的工具调用缓冲条目（按 ToolCall.ID 索引）。
type wireToolCall struct {
	name string
	args string
}

// ── 事件循环消息 ─────────────────────────────────────────

// frameMsg 一帧渲染结果（全部为已渲染的最终形态文本）。
// 由 pullCmd（tea.Tick）在框架的 cmd goroutine 上产出、return 交给框架送进
// Update——不经过 program.Send，走标准的 Elm 消息通道。
type frameMsg struct {
	view    string // 消息区全文（含横幅/终态，glamour 已渲染、lipgloss 已上色）
	todo    string // 清单栏渲染结果（空串=无清单）
	notice  string // 底部通知渲染结果（含 TTL 兜底）
	running bool   // agent 是否在运行（spinner 开关依据）
	fatal   *wireFatal
}

// pullSkipMsg 坏帧保活消息。pullCmd 的 fn 绝不能返回 nil（nil 不会触发
// Update 的续链分支，链会静默断掉）——RunStateJSON 解析失败时返回它，
// Update 收到后什么都不应用、只返回下一个 pullCmd 续链。
type pullSkipMsg struct{}
