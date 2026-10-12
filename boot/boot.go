package boot

import (
	"HackerTeam/service/engine"
	"HackerTeam/service/tui"
)

func Boot() {
	// 引擎先做轻量构造（不触盘、不阻塞），把 *Engine 直接交给 TUI——
	// 状态就是 Engine 的字段，UI 经引擎暴露的方法按帧拉取，无中间层。
	// 生命周期统一收敛到引擎的 Run：Init 同步编排、主循环 goroutine 由
	// AgentStart 内置，boot 不再手拼引擎内部序列。这里保留一个 go 只是为了
	// 不阻塞 TUI 启动——Init 走致命终态时 parkWithFatal 驻留，终态消息要靠
	// 已在运行的 TUI 渲染。
	e := engine.GetEngineService("HackerTeam")
	t := tui.NewTui(e)
	go e.Run()
	t.Run()

}
