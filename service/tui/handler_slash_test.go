package tui

// keyMsgHandler 层的斜杠补全测试：直接喂 tea.KeyPressMsg 走真实分发路径，
// 验证按键转发与浮层重算的接线（而不只是 refilterSlashItems 本身）。
//
// 原始缺陷就在这一层：浮层打开时的分支既没把按键转发给 textarea（于是拿旧值
// 匹配，清单永远差一个字符），又在末尾 return t, nil 把按键吞掉（输入框根本
// 收不到字），表现为"敲 / 弹出全量、继续打字清单不更新"。

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// printKey 构造一个可打印字符按键；specialKey 构造 enter/esc 等特殊键。
func printKey(ch rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: ch, Text: string(ch)} }

func specialKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func ctrlKey(ch rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ch, Mod: tea.ModCtrl}
}

// press 依次把按键喂给 keyMsgHandler（走真实分发路径，不直接改 textarea）。
func press(t *testing.T, tt *TUI, keys ...tea.KeyPressMsg) *TUI {
	t.Helper()
	for _, k := range keys {
		m, _ := tt.keyMsgHandler(k)
		tt, _ = m.(*TUI)
	}
	return tt
}

func TestKeyHandlerSlashPopupTracksInput(t *testing.T) {
	tt, _ := newSlashTestTUI()

	//① 单敲 "/"：全量清单（注册表的 cmd 一律以 "/" 开头，空前缀匹配全部）
	tt = press(t, tt, printKey('/'))
	if !tt.slashPopup.isVisible() {
		t.Fatalf("after '/': popup not visible")
	}
	if n := len(tt.slashPopup.items); n != len(slashRegistry) {
		t.Errorf("after '/': items=%d, want %d (full list)", n, len(slashRegistry))
	}
	if got := tt.ta.Value(); got != "/" {
		t.Errorf("after '/': textarea=%q, want %q（按键必须转发给 textarea）", got, "/")
	}

	//② 继续打字：清单必须跟着收窄——这一条就是原始缺陷的回归测试
	tt = press(t, tt, printKey('r'))
	if got := tt.ta.Value(); got != "/r" {
		t.Errorf("after '/r': textarea=%q, want %q", got, "/r")
	}
	if !tt.slashPopup.isVisible() {
		t.Fatalf("after '/r': popup not visible")
	}
	if n := len(tt.slashPopup.items); n != 1 {
		t.Errorf("after '/r': items=%d, want 1（清单没跟着输入重算）", n)
	}
	if sel, _ := tt.slashPopup.selectedCmd(); sel != "/resume" {
		t.Errorf("after '/r': selected=%q, want /resume", sel)
	}

	//③ 退格回到 "/"：清单重新展开
	tt = press(t, tt, specialKey(tea.KeyBackspace))
	if got := tt.ta.Value(); got != "/" {
		t.Fatalf("after backspace: textarea=%q, want %q", got, "/")
	}
	if n := len(tt.slashPopup.items); n != len(slashRegistry) {
		t.Errorf("after backspace: items=%d, want %d", n, len(slashRegistry))
	}

	//④ 打到无匹配：浮层收起
	tt = press(t, tt, printKey('z'), printKey('z'))
	if tt.slashPopup.isVisible() {
		t.Error("after '/zz': popup should be hidden (no match)")
	}
}

func TestKeyHandlerPopupDismissAndCompletion(t *testing.T) {
	tt, _ := newSlashTestTUI()

	tt = press(t, tt, printKey('/'), printKey('r'), printKey('e'))
	if !tt.slashPopup.isVisible() {
		t.Fatalf("popup should be visible for '/re'")
	}

	//esc 收起浮层，但输入框内容保留
	tt = press(t, tt, specialKey(tea.KeyEsc))
	if tt.slashPopup.isVisible() {
		t.Error("esc should hide the popup")
	}
	if got := tt.ta.Value(); got != "/re" {
		t.Errorf("esc should keep the textarea content, got %q", got)
	}

	//已知瑕疵（与 HyperBot 同型，照原样继承不修）：esc 只改 visible，输入框里
	//"/re" 还在，下一个可打印按键经 refilterSlashItems 又把浮层弹回来。
	tt = press(t, tt, printKey('s'))
	if !tt.slashPopup.isVisible() {
		t.Error("known wart: a printable key re-opens the popup while the token is still in the box")
	}

	//enter 补全：整段替换而非插入，否则 "/re" + "/resume " 会拼成 "/re/resume "
	tt = press(t, tt, specialKey(tea.KeyEnter))
	if got := tt.ta.Value(); got != "/resume " {
		t.Errorf("enter completion: textarea=%q, want %q", got, "/resume ")
	}
	if tt.slashPopup.isVisible() {
		t.Error("enter completion should hide the popup")
	}

	//已知瑕疵：补全后带尾空格 → slashToken 判为"在写参数" → 浮层不再弹出，
	//且这一次 enter 只做补全、没有提交，得再按一次 enter 才真正分发。
	tt = press(t, tt, specialKey(tea.KeyEnter))
	if got := tt.ta.Value(); got != "" {
		t.Errorf("second enter should dispatch and reset the textarea, got %q", got)
	}
}

func TestKeyHandlerPlainChatKeepsPopupClosed(t *testing.T) {
	tt, e := newSlashTestTUI()

	tt = press(t, tt, printKey('h'), printKey('i'))
	if tt.slashPopup.isVisible() {
		t.Error("plain chat must not open the popup")
	}
	if got := tt.ta.Value(); got != "hi" {
		t.Errorf("textarea=%q, want %q", got, "hi")
	}

	//enter 提交普通输入：走 submitInput，不走 dispatchSlash
	tt = press(t, tt, specialKey(tea.KeyEnter))
	if e.submitted != "hi" {
		t.Errorf("submitted=%q, want %q", e.submitted, "hi")
	}
	if got := tt.ta.Value(); got != "" {
		t.Errorf("textarea should reset after submit, got %q", got)
	}
}

// ctrl+k 已从按键表里彻底移除：它现在只是一个普通按键，会落进 textarea。
func TestKeyHandlerCtrlKIsGone(t *testing.T) {
	tt, _ := newSlashTestTUI()
	tt = press(t, tt, ctrlKey('k'))
	if tt.slashPopup.isVisible() {
		t.Error("ctrl+k must no longer open the popup")
	}
}

// 已知瑕疵（与 HyperBot 同型，照原样继承不修）：浮层分支把 ctrl+c 与 esc
// 并列，所以浮层开着时 ctrl+c 只关浮层、不退出，用户得再按一次才真退出。
func TestKeyHandlerCtrlCWhilePopupOpen(t *testing.T) {
	tt, _ := newSlashTestTUI()
	tt = press(t, tt, printKey('/'))

	_, cmd := tt.keyMsgHandler(ctrlKey('c'))
	if cmd != nil {
		t.Errorf("ctrl+c with popup open returned a cmd (%v), want nil（known wart: 只关浮层）", cmd)
	}
	if tt.slashPopup.isVisible() {
		t.Error("ctrl+c with popup open should at least close the popup")
	}

	//浮层关掉之后，ctrl+c 恢复退出语义
	if _, cmd := tt.keyMsgHandler(ctrlKey('c')); cmd == nil {
		t.Error("ctrl+c with popup closed should return tea.Quit")
	}
}
