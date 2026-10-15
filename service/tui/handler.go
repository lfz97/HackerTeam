package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// keyMsgHandler 处理按键消息（demo 范式 + HyperBot 语义）。
//
// 按键优先级从高到低：
//  1. 终态等键模式（fatal.waitKey）——任意键退出
//  2. 帮助浮层打开——ctrl+k/esc/ctrl+c 关浮层，up/down 选条目，enter 插入指令
//  3. /resume 弹框打开——up/down 选会话，enter 恢复，esc 取消，其余键吞掉
//  4. ctrl+c 退出、enter 提交（斜杠命令先经 dispatchSlash 分发）、ctrl+k 开浮层、
//     esc 中断（仅运行态）
//  5. 其余转发给 textarea 自行处理
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
			//清单为空时无指令可插入，浮层保持打开（esc/ctrl+k 仍能关）
			if cmd, ok := t.skills.selectedCmd(); ok {
				t.ta.InsertString(cmd + " ") //把当前选中的命令写入到textarea里面去
				t.skills.toggleVisibility()
				t.recalcComponentSize()
			}
			return t, nil

		} else {
			return t, nil
		}

	} else if t.resume.isVisible() {
		// /resume 弹框：方向键选择、enter 确认、esc 取消；其余键一律吞掉，
		// 不让按键漏进 textarea（弹框期间输入被引擎串行化，不存在并发 turn）。
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
			//斜杠命令在前端分发（注册表自持，后端只暴露能力接口）；未知命令
			//按普通输入发给引擎，保持原先的兜底行为。
			//普通输入主动提交给引擎（submitInput 内部 select-default，非阻塞），
			//引擎忙（无人接收）时提交失败、保留输入框内容，用户输入不丢失。
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

		} else if input == "ctrl+k" {
			t.skills.toggleVisibility() //清单是本地静态注册表，开浮层无需刷新
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
