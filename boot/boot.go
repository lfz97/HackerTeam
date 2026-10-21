package boot

import (
	"HackerTeam/service/engine"
	"HackerTeam/service/tui"
)

func Boot() {
	// 引擎轻量构造后交给 TUI，生命周期收敛到 e.Run。这里用 go 是为了不阻塞
	// TUI 启动：Init 走致命终态时 parkWithFatal 驻留，终态消息要靠已在运行的 TUI 渲染。
	e := engine.GetEngineService("HackerTeam")
	t := tui.NewTui(e)
	go e.Run()
	t.Run()

}
