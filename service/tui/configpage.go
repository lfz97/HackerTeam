package tui

// 模型配置页：模态表单，收集 model 块的四个核心字段后提交引擎。
// 当前由首次启动流程驱动开页（引擎写回 yaml 并 flush 重跑装配）；
// 组件本身与首次启动解耦（常驻构造 + open 灌值），后续 /config 之类的
// 编辑入口可复用，仅需替换提交目标。按键语义在 handler.go（模态期间
// 全量接管），本文件只管结构、渲染与聚焦。
//
// 输入组件用 textinput（单行，crush 同款选型）而非 textarea：
// 没有软换行/MaxHeight/Prompt 一类多行概念需要压制，粘贴自动把换行、
// Tab 清洗成空格。显示策略：内部宽度远大于内容（窗口永不横滚），
// 渲染时由 viewValue 截断到列宽——于是"显示恒为头部 + 光标逻辑在尾部
// （退格从尾部删）"可以同时成立，因为光标本就不可见。

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	configBoxWidth  = 72 //盒子总宽（含边框、padding）
	configBoxHeight = 16 //盒子总高上限（实际随内容收缩；单行表单很矮）
	configMinTextW  = 30 //终端极窄时的兜底文本宽，防止负宽 panic
	configTitle     = "First Startup · Model Config"
	configHint      = "←/→ apitype · tab/↑/↓ switch · enter save & start · esc quit"

	configLabelW    = 9   //label 列宽（最长 Base URL/API Type=8，右补空格对齐输入列）
	configFieldW    = 2   //聚焦标记列宽（"▶ "，失焦用等宽空格占位）
	configValueW    = 42  //输入值列宽（收窄于文本区，整块内容才能在盒内居中）
	configInternalW = 512 //输入框内部宽度：窗口永不横滚，显示截断由 viewValue 负责
	configCharLimit = 512 //与内部宽度一致，保证任何合法值都能整体放进窗口
)

// configFieldSpecs 表单字段定义（顺序即 Tab 焦点顺序）。
// label 是展示名（与 yaml 键解耦）；choices 非空 = 二选一项：无输入框，←/→ 切换；
// placeholder 是输入框清空后的暗色示例提示。
var configFieldSpecs = []struct {
	key         string
	label       string
	placeholder string
	choices     []string
}{
	{"model", "Model", "deepseek-reasoner", nil},
	{"baseurl", "Base URL", "https://api.deepseek.com", nil},
	{"apikey", "API Key", "sk-...", nil},
	{"apitype", "API Type", "", []string{"openai", "anthropic"}},
}

// configField 单个表单项：自由文本项（input）或二选一项（choices）。
// 二选一项的 input 恒为零值，一切输入框方法须经 isChoice 判别后调用。
type configField struct {
	key         string
	label       string
	placeholder string
	input       textinput.Model
	choices     []string
	choiceIdx   int
}

func (f *configField) isChoice() bool { return len(f.choices) > 0 }

// moveChoice 二选一项切换候选项（循环）。
func (f *configField) moveChoice(delta int) {
	f.choiceIdx = (f.choiceIdx + delta + len(f.choices)) % len(f.choices)
}

// value 字段当前值（提交形状与 yaml 字段一致）。
func (f *configField) value() string {
	if f.isChoice() {
		return f.choices[f.choiceIdx]
	}
	return f.input.Value()
}

// configPage 模型配置页（结构对齐 resume 弹框：自持状态 + View 渲染）。
type configPage struct {
	fields  []*configField
	focus   int
	errText string
	visible bool
}

// newConfigPage 常驻组件（NewTui 时构造，visible=false）。
// 输入框留空；开页时经 open 灌预填值——建页与"首次启动弹页"解耦，
// 以后 /config 之类的编辑入口可直接复用本组件。
func newConfigPage() *configPage {
	p := &configPage{visible: false}
	for _, spec := range configFieldSpecs {
		f := &configField{key: spec.key, label: spec.label, placeholder: spec.placeholder, choices: spec.choices}
		if !f.isChoice() {
			input := textinput.New()
			input.SetVirtualCursor(false) //去掉聚焦行尾部的光标竖线（焦点指示已有 ▶ + 配色）
			input.Prompt = ""             //默认行首 "> " 提示符，与左右排版冲突，清掉
			input.Placeholder = ""        //占位提示由 viewValue 自渲染，统一走截断管道
			input.CharLimit = configCharLimit
			input.SetStyles(textinput.Styles{}) //全零值=纯文本，颜色由 viewValue 统一上色
			f.input = input
		}
		p.fields = append(p.fields, f)
	}
	return p
}

// open 开页并灌入预填值：清空旧输入、二选一项按预填选中、焦点归首项。
// SetValue 后光标自动落在尾部（退格可直接删尾部）。
func (p *configPage) open(prefill map[string]string) {
	p.errText = ""
	p.focus = 0
	for _, f := range p.fields {
		if f.isChoice() {
			//预填值命中候选项则选中之，未命中取第一项
			f.choiceIdx = 0
			for i, c := range f.choices {
				if c == prefill[f.key] {
					f.choiceIdx = i
					break
				}
			}
			continue
		}
		f.input.Reset()
		f.input.SetValue(prefill[f.key])
		f.input.Blur()
	}
	if f := p.fields[0]; !f.isChoice() {
		f.input.Focus() //cmd 丢弃，不上屏光标
	}
	p.visible = true
}

// isVisible nil 安全：cfgPage 虽已改为常驻构造，保留 nil 容忍以免调用方
// 关心构造时序（与 slashPopup/resume 的查询方式完全一致）。
func (p *configPage) isVisible() bool { return p != nil && p.visible }

// moveFocus 切换焦点：旧项失焦，新项聚焦。
// 刻意丢弃 Focus() 返回的 cmd（光标使能/闪烁链）：表单不想要终端光标，
// 焦点指示由 "▶ " + 配色承担。二选一项无输入框，跳过 blur/focus。
func (p *configPage) moveFocus(delta int) {
	if len(p.fields) == 0 {
		return
	}
	if old := p.fields[p.focus]; !old.isChoice() {
		old.input.Blur()
	}
	p.focus = (p.focus + delta + len(p.fields)) % len(p.fields)
	if f := p.fields[p.focus]; !f.isChoice() {
		f.input.Focus() //cmd 丢弃，不上屏光标
	}
}

// values 收集全部字段（key → 原文，未 trim：引擎侧统一处理）。
func (p *configPage) values() map[string]string {
	vals := make(map[string]string, len(p.fields))
	for _, f := range p.fields {
		vals[f.key] = f.value()
	}
	return vals
}

// View 渲染表单内容：label 列 + 输入值列同行左右排布，整块在盒内水平居中。
func (p *configPage) View(termWidth, termHeight int) string {
	boxW := min(configBoxWidth, termWidth)
	st := overlayBoxStyle.Width(boxW)
	textW := max(boxW-st.GetHorizontalFrameSize(), configMinTextW)
	valueW := max(min(configValueW, textW-configFieldW-configLabelW-1), 10)

	lines := []string{configTitle, ""}
	for i, f := range p.fields {
		//内部宽度恒定放大：textinput 的横滚窗口永不触发，显示恒为头部。
		//二选一项无输入框（零值 textinput），不可触碰
		if !f.isChoice() {
			f.input.SetWidth(configInternalW)
		}
		label := fmt.Sprintf("%-*s", configLabelW, f.label)
		if i == p.focus {
			label = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("▶ " + label)
		} else {
			label = lipgloss.NewStyle().Foreground(cFgMuted).Render(strings.Repeat(" ", configFieldW) + label)
		}
		lines = append(lines, label+" "+f.viewValue(i == p.focus, valueW))
	}
	if p.errText != "" {
		errLine, _ := truncateByWidth("✗ "+p.errText, textW)
		lines = append(lines, "", lipgloss.NewStyle().Foreground(cError).Render(errLine))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(cFgMuted).Render(configHint))

	//整块居中：按最宽行算左移量，逐行补齐
	maxW := 0
	for _, ln := range lines {
		maxW = max(maxW, lipgloss.Width(ln))
	}
	pad := strings.Repeat(" ", max(0, (textW-maxW)/2))
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(pad + ln + "\n")
	}
	return b.String()
}

// viewValue 渲染值区：文本项截断到列宽（显示恒为头部，尾部不展示——
// 光标不可见，故与"光标在尾部"不冲突）；二选一项渲染候选项行（选中 ● 高亮）。
// 上色在截断之后整段进行：先截断再染色，避免切开 ANSI 序列。
func (f *configField) viewValue(focused bool, valueW int) string {
	if f.isChoice() {
		parts := make([]string, 0, len(f.choices))
		for i, c := range f.choices {
			if i == f.choiceIdx {
				style := lipgloss.NewStyle().Foreground(cSuccess)
				if focused {
					style = style.Bold(true)
				}
				parts = append(parts, style.Render("● "+c))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(cFgMuted).Render("○ "+c))
			}
		}
		return strings.Join(parts, "   ")
	}

	//空值 → 暗色占位提示
	if strings.TrimSpace(f.input.Value()) == "" {
		if f.placeholder == "" {
			return ""
		}
		ph, _ := truncateByWidth(f.placeholder, valueW)
		return lipgloss.NewStyle().Foreground(cFgMostSubtle).Render(ph)
	}

	raw := strings.TrimRight(f.input.View(), " ")
	text, _ := truncateByWidth(raw, valueW)
	color := cFgSubtle
	if focused {
		color = cFgBase
	}
	return lipgloss.NewStyle().Foreground(color).Render(text)
}

// configPageKeyHandler 配置页模态按键：esc 退出应用（首次启动没有"取消"语义），
// enter 提交（失败页内显示错误、保持可重填），tab/方向键切换焦点，
// 其余键交给聚焦项（文本项进输入框，二选一项 ←/→ 切换）。
// 全量接管：任何按键不漏进主输入框。
func (t *TUI) configPageKeyHandler(m tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	input := m.String()
	page := t.cfgPage

	switch input {
	case "ctrl+c", "esc":
		//模态期无回合在跑，与 /exit 同款收尾安全：释放 runner/工具集再退出
		t.engine.Shutdown()
		return t, tea.Quit
	case "tab", "down":
		page.moveFocus(1)
		return t, nil
	case "shift+tab", "up":
		page.moveFocus(-1)
		return t, nil
	case "enter":
		if err := t.submitFirstStartupConfig(page.values()); err != nil {
			page.errText = err.Error()
			return t, nil
		}
		page.visible = false
		t.vp.GotoBottom()
		t.recalcComponentSize()
		return t, nil
	default:
		f := page.fields[page.focus]
		if f.isChoice() {
			switch input {
			case "left":
				f.moveChoice(-1)
			case "right", " ":
				f.moveChoice(1)
			}
			return t, nil
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(m)
		return t, cmd
	}
}

// configPageView 配置页浮层渲染（draw 的叠加层）；异常态（nil/不可见）返回空串。
func (t *TUI) configPageView() string {
	if t.cfgPage == nil || !t.cfgPage.isVisible() {
		return ""
	}
	return t.cfgPage.View(t.width, t.height)
}
