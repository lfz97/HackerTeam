package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// keyMsgHandler 处理按键消息，优先级从高到低：
//  1. 终态等键模式（fatal.waitKey）——任意键退出
//  2. 斜杠命令浮层打开——esc/ctrl+c 关浮层，up/down 选条目，enter 补全指令
//  3. /resume 弹框打开——up/down 选会话，enter 恢复，esc 取消，其余键吞掉
//  4. ctrl+c 退出、enter 提交（斜杠命令先经 dispatchSlash 分发）、esc 中断（仅运行态）
//  5. 其余转发给 textarea，再按输入框内容重算浮层
//
// 浮层无独立打开快捷键：它是输入框内容的派生状态（输入 "/" 起前缀匹配），
// 失去斜杠前缀自动收起——可见性只有一个决定点（refilterSlashItems）。
func (t *TUI) keyMsgHandler(m tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	input := m.String()

	// 终态等任意键退出，必须最先判，不再让按键进输入框。
	if t.waitingKey {
		return t, tea.Quit
	}

	if t.slashPopup.isVisible() {
		if input == "esc" || input == "ctrl+c" {
			t.slashPopup.switchVisibility(false)
			return t, nil

		} else if input == "down" {
			t.slashPopup.moveDown()
			return t, nil

		} else if input == "up" {
			t.slashPopup.moveUp()
			return t, nil

		} else if input == "enter" {
			//清单为空时无指令可补全，浮层保持打开（esc 仍能关）
			sel, ok := t.slashPopup.selectedCmd()
			if !ok {
				return t, nil
			}
			//整段替换而非 InsertString：输入框里留着半截 "/re"，插入会拼成
			//"/re/resume "。补全后带空格，光标停在命令末尾等用户接参数。
			t.ta.SetValue(sel + " ")
			t.slashPopup.switchVisibility(false)
			t.recalcComponentSize()
			return t, nil

		} else {
			var cmd tea.Cmd
			t.ta, cmd = t.ta.Update(m)
			t.refilterSlashItems() //浮层开着时继续打字也要重新过滤，否则清单永远停在第一次的结果
			t.recalcComponentSize()
			return t, cmd
		}

	} else if t.resume.isVisible() {
		// /resume 弹框：方向键选择、enter 确认、esc 取消；其余键一律吞掉，
		// 不让按键漏进 textarea。
		if input == "ctrl+c" {
			return t, tea.Quit
		} else if input == "esc" {
			t.resume.visible = false //取消是纯前端状态，不经引擎
			return t, nil
		} else if input == "down" {
			t.resume.moveDown()
			return t, nil
		} else if input == "up" {
			t.resume.moveUp()
			return t, nil
		} else if input == "enter" {
			//成功才关框：失败（turn 进行中 / 会话已不存在）时弹框保持，用户可重选。
			//失败原因引擎侧已自行上屏（appendTyped + notice），此处无需处理。
			if id, ok := t.resume.selectedID(); ok && t.checkoutSession(id) {
				t.resume.visible = false
			}
			return t, nil
		}
		return t, nil

	} else {
		if input == "ctrl+c" {
			return t, tea.Quit

		} else if input == "enter" {
			//斜杠命令在前端分发；未知命令按普通输入发给引擎兜底。
			//引擎忙时提交失败、保留输入框内容，用户输入不丢失。
			text := strings.TrimSpace(t.ta.Value())
			if text == "" { //空输入就地忽略，不占一条记录
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

		} else if input == "esc" {
			// 浮层已在上面处理；运行期 → 中断 agent，非运行期 no-op。
			if t.running {
				t.interrupt()
			}
			return t, nil

		} else {
			var cmd tea.Cmd
			t.ta, cmd = t.ta.Update(m)
			t.refilterSlashItems()
			t.recalcComponentSize()
			return t, cmd
		}
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
