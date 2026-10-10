package tui

import (
	tea "charm.land/bubbletea/v2"
)

// keyMsgHandler 处理按键消息（demo 范式 + HyperBot 语义）。
//
// 按键优先级从高到低：
//  1. 终态等键模式（fatal.waitKey）——任意键退出
//  2. 帮助浮层打开——ctrl+k/esc/ctrl+c 关浮层，up/down 选条目，enter 插入指令
//  3. ctrl+c 退出、enter 提交、ctrl+k 开浮层、esc 中断（仅运行态）
//  4. 其余转发给 textarea 自行处理
func (t *TUI) keyMsgHandler(m tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	input := m.String()

	// 终态 waitKey：消息已上屏，任意键退出（必须最先判，不再让按键进输入框）。
	if t.waitingKey {
		return t, tea.Quit
	}

	if t.skills.isVisible() {
		if input == "ctrl+k" || input == "esc" || input == "ctrl+c" {
			t.skills.toggleVisibility()
			return t, nil

		} else if input == "down" {
			t.skills.moveDown()
			return t, nil

		} else if input == "up" {
			t.skills.moveUp()
			return t, nil

		} else if input == "enter" {
			t.ta.InsertString(t.skills.items[t.skills.index][0] + " ") //把当前选中的命令写入到textarea里面去
			t.skills.toggleVisibility()
			t.recalcComponentSize()
			return t, nil

		} else {
			return t, nil
		}

	} else {
		if input == "ctrl+c" {
			return t, tea.Quit

		} else if input == "enter" {
			//获取输入文本，主动提交给引擎（submitInput 内部 select-default，
			//非阻塞）。引擎忙（无人接收）时提交失败、保留输入框内容，用户输入
			//不丢失。空输入引擎侧会忽略（engineRun 的 checkprompt == "" 分支），
			//无需特判。
			text := t.ta.Value()
			if t.submitInput(text) {
				t.ta.Reset()
			}
			t.recalcComponentSize()
			return t, nil

		} else if input == "ctrl+k" {
			t.skills.Update(t.fetchSkillItems()) // 每次打开时刷新，确保 skills 等动态项可见
			t.skills.toggleVisibility()
			return t, nil

		} else if input == "esc" {
			// esc 优先级：浮层开着上面已处理；运行期 → 中断 agent。
			// 非运行期无操作（放行给 textarea 也无绑定，等价 no-op）。
			if t.running {
				t.interrupt()
			}
			return t, nil

		} else {
			var cmd tea.Cmd
			t.ta, cmd = t.ta.Update(m) //只需把消息转发就行，textarea内部会自己处理
			t.recalcComponentSize()
			return t, cmd
		}
	}

}
