package boot

import (
	"HackerTeam/service/engine"
	"HackerTeam/service/tui"
)

func Boot() {
	// 引擎先做轻量构造（不触盘、不阻塞），把 *Engine 直接交给 TUI——
	// 状态就是 Engine 的字段，UI 经引擎暴露的方法按帧拉取，无中间层。
	// 重初始化（bootstrap/newRunner）放在 goroutine 里：失败会 parkWithFatal
	// 永久驻留，若同步执行会卡死 TUI 启动（终态都无从渲染）。
	e := engine.GetEngineService("HackerTeam")
	t := tui.NewTui(e)
	go func() {
		e.Init()
		e.AgentStart()
	}()
	t.Run()

}
