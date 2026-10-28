package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// keyMsgHandler 按键分发，优先级从高到低：
//  1. 终态等键模式（fatal.waitKey）——任意键退出
//  2. 模态层接管（各浮层自持 xxxKeyHandler，按键不漏进主输入框）：
//     首次启动配置页 / 斜杠命令浮层 / /resume 弹框
//  3. 主路径 mainKeyHandler：ctrl+c 退出、enter 提交、esc 中断（仅运行态），
//     其余转发给 textarea，再按输入框内容重算浮层
//
// 浮层无独立打开快捷键：斜杠浮层由输入框内容派生（输入 "/" 起前缀匹配），
// 配置页由引擎帧一次性开页，resume 由 /resume 命令打开。
func (t *TUI) keyMsgHandler(m tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	input := m.String()

	// 终态等任意键退出，必须最先判，不再让按键进输入框。
	if t.waitingKey {
		return t, tea.Quit
	}

	// 模态层：各浮层自持按键语义（isVisible 均可安全调用）。
	if t.cfgPage.isVisible() {
		return t.configPageKeyHandler(m)
	}
	if t.slashPopup.isVisible() {
		return t.slashPopupKeyHandler(m)
	}
	if t.resume.isVisible() {
		return t.resumeKeyHandler(m)
	}
	return t.mainKeyHandler(m, input)
}

// mainKeyHandler 主路径：ctrl+c 退出、enter 提交、esc 中断（仅运行态），
// 其余转发给 textarea，再按输入框内容重算浮层。
func (t *TUI) mainKeyHandler(m tea.KeyPressMsg, input string) (tea.Model, tea.Cmd) {
	switch input {
	case "ctrl+c":
		return t, tea.Quit

	case "enter":
		//斜杠命令在前端分发；未知命令按普通输入发给引擎兜底。
		//引擎忙时提交失败、保留输入框内容，用户输入不丢失。
		text := strings.TrimSpace(t.ta.Value())
		if text == "" {
			return t, nil
		}

		//处理斜杠指令
		cmd, handled := t.dispatchSlash(text)
		if handled {
			t.ta.Reset()
			t.vp.GotoBottom()
			t.recalcComponentSize()
			return t, cmd
		}
		if t.submitInput(text) {
			t.ta.Reset()
		}
		t.vp.GotoBottom()
		t.recalcComponentSize()
		return t, nil

	case "esc":
		// 浮层已在上面处理；运行期 → 中断 agent，非运行期 no-op。
		if t.running {
			t.interrupt()
		}
		return t, nil

	default:
		var cmd tea.Cmd
		t.ta, cmd = t.ta.Update(m)
		t.refilterSlashItems()
		t.recalcComponentSize()
		return t, cmd
	}
}

// slashToken 返回输入框里正在输入的半截斜杠命令；含空白说明已在写参数
// （或就是普通消息），此时补全没有意义，ok=false。
func slashToken(text string) (string, bool) {
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t\n") {
		return "", false
	}
	return text, true
}

// refilterSlashItems 按输入框当前内容重算斜杠命令浮层：有前缀匹配才显示，
// 否则收起。必须在按键已交给 textarea 之后调用——用旧值匹配会永远差一个字符。
// 条目源是 slashRegistry（编译期常量），过滤是纯内存操作，每次按键跑一遍无成本。
func (t *TUI) refilterSlashItems() {
	tok, ok := slashToken(t.ta.Value())
	if !ok {
		t.slashPopup.switchVisibility(false)
		return
	}

	var matched []slashCmd
	for _, sc := range slashRegistry {
		if strings.HasPrefix(sc.cmd, tok) {
			matched = append(matched, sc)
		}
	}
	if len(matched) == 0 {
		t.slashPopup.switchVisibility(false)
		return
	}
	t.slashPopup.setItems(matched) //内部重置 index=0，新清单从首项开始选中
	t.slashPopup.switchVisibility(true)
}
