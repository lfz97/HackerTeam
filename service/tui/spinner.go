package tui

import (
	"math"
	"strings"
	"time"

	sp "charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

var (
	// 圆点滑窗：8 个实心点沿 12 格轨道循环，空位不画空心点（运行时只有
	// 光点在跑，轨道留白；空闲态由静态暗点轨接管）。字形与输入框的圆角
	// 呼应，12 格（旧版 8 格）让扫动更从容。
	defaultDynamicSpinner []string = []string{
		"••••••••    ",
		" ••••••••   ",
		"  ••••••••  ",
		"   •••••••• ",
		"    ••••••••",
		"•    •••••••",
		"••    ••••••",
		"•••    •••••",
		"••••    ••••",
		"•••••    •••",
		"••••••    ••",
		"•••••••    •",
	}
	// 静态轨：同色相同饱和度、明度压一档的暗紫——空闲时轨道在场但不抢戏
	defaultStaticSpinner = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorful.Hsv(hueCenter, satBase, staticVal).Hex())).
				Render(strings.Repeat("•", 12))
)

const colorSteps = 36 // 色相往返切 36 格，一圈约 2.2s

// 渐变锚定输入框边框的紫（ANSI 256 的 62 号 ≈ Hsv(240°, 0.56, 0.84)）：
// 色相在 240±20（蓝紫↔紫）往返，S/V 贴边框——替代旧版全彩 HSV 色轮，
// 与输入框圆角边框同一色系。
const (
	hueCenter = 240.0
	hueSwing  = 20.0
	satBase   = 0.56
	valBase   = 0.84
	staticVal = 0.55 // 静态轨明度：比输入框边框（0.84）暗一档
)

// borderHue 相位对应的色相：hueCenter ± hueSwing 往返。
func borderHue(phase int) float64 {
	return hueCenter + hueSwing*math.Sin(2*math.Pi*float64(phase%360)/360)
}

// spinner 圆点滑窗 + 紫色渐变：整帧一个颜色逐帧推进（bubbles 的原生模型，
// 帧表是静态字符串）。运行态由 applyFrame 调 Start() 起帧链，Stop() 后
// 不再续 tick，空闲时显示静态占位帧。
type spinner struct {
	spinner   sp.Model
	isRunning bool
	phase     int // 颜色相位,与帧同步推进
	static    string
}

func NewSpinner(frames []string, static string, fps time.Duration) *spinner {
	return &spinner{
		spinner: sp.New(
			sp.WithSpinner(sp.Spinner{
				Frames: frames,
				FPS:    fps,
			}),
			sp.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205")))),
		static: static,
	}
}

// Start 进入运行态并返回启动帧链的 cmd。
func (s *spinner) Start() tea.Cmd {
	s.isRunning = true
	return s.spinner.Tick
}

// Stop 退出运行态。正在排队的最后一帧会被 Update 丢弃（不再续链）。
func (s *spinner) Stop() {
	s.isRunning = false
}

func (s *spinner) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case sp.TickMsg:
		if s.isRunning {
			var cmd tea.Cmd
			s.spinner, cmd = s.spinner.Update(m)

			//每帧推进色相相位：以输入框边框紫为中心 ±20 往返
			s.phase += 360 / colorSteps
			c := colorful.Hsv(borderHue(s.phase), satBase, valBase)
			s.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))

			return cmd
		}
	}

	return nil
}

func (s *spinner) View() string {
	if s.isRunning {
		return s.spinner.View()
	} else {
		return s.static
	}

}
