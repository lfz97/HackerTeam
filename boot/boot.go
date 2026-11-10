package boot

import (
	"HackerTeam/service/engine"
	"HackerTeam/service/tui"
)

// Boot 引擎启动与 TUI 生命周期。
// Init 在 TUI 启动前同步完成：失败直接上抛（main 打印退出）——此时终端尚未
// 进入 alt-screen/raw mode，标准 CLI 报错即可，不需要 TUI 渲染终态；
// TUI 运行期的致命路径（配置页 flush 失败）才走 fatal 态渲染，两类机制不混用。
// 同步的代价是首帧延迟一个 Init 时长（磁盘检查+装配，实测 <100ms），
// 换取 booting 中间态与启动期 fatal 渲染整条链路消失。
func Boot() error {
	e := engine.GetEngineService("HackerTeam")
	if err := e.Init(); err != nil {
		return err
	}
	// TUI 的 Run 内部消化 tea 程序错误（退出码语义归 TUI/进程约定），不外抛。
	tui.NewTui(e).Run()
	return nil
}
