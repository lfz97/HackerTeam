package tui

import (
	"time"

	sp "charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

var (
	defaultDynamicSpinner []string = []string{
		"■■■■⬝⬝⬝⬝",
		"⬝■■■■⬝⬝⬝",
		"⬝⬝■■■■⬝⬝",
		"⬝⬝⬝■■■■⬝",
		"⬝⬝⬝⬝■■■■",
		"■⬝⬝⬝⬝■■■",
		"■■⬝⬝⬝⬝■■",
		"■■■⬝⬝⬝⬝■",
	}
	defaultStaticSpinner string = "⬝⬝⬝⬝⬝⬝⬝⬝"
)

const colorSteps = 36 // 色轮切 36 格,一圈约 3 秒

// spinner 方块彩虹 spinner（demo 同款）：■■■■⬝⬝⬝⬝ 帧动画 + HSV 色轮渐变。
// 运行态（agent 正在跑）时由 applyFrame 调 Start() 起帧链，Stop() 后帧链
// 自然断掉（Update 不再续 tick），空闲时显示静态占位帧。
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
				FPS:    fps, //定义延时，原理：spinner update方法返回的cmd，执行时会sleep指定时长再返回Msg
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

			//通过每帧更新phase，并根据phase渲染spinner.Style，使得色轮往前推进，实现渐变色的效果
			s.phase += 360 / colorSteps
			c := colorful.Hsv(float64(s.phase%360), 0.85, 0.95)
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
