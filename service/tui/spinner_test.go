package tui

// 圆点滑窗 spinner 的行为契约：
//  1. 帧表所有帧与静态帧等宽——底栏 gap 按帧实时重算（recalcComponentSize），
//     宽度漂移会让 notice 在帧间横跳，这是布局不动的硬前提；
//  2. 色相始终锁定在输入框边框紫 ±20 的摆幅内（渐变与输入框同色系的硬约束）；
//  3. View 运行态渲染动画帧、空闲态渲染静态帧。

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestSpinnerFrameWidthStable(t *testing.T) {
	want := lipgloss.Width(defaultStaticSpinner)
	for i, f := range defaultDynamicSpinner {
		if w := lipgloss.Width(f); w != want {
			t.Fatalf("frame[%d] width=%d, want %d", i, w, want)
		}
	}
}

func TestSpinnerHueAnchoredToBorder(t *testing.T) {
	for p := 0; p < 360; p++ {
		h := borderHue(p)
		if h < hueCenter-hueSwing || h > hueCenter+hueSwing {
			t.Fatalf("phase=%d hue=%v 超出边框紫 ±%v 摆幅", p, h, hueSwing)
		}
	}
}

func TestSpinnerViewState(t *testing.T) {
	s := NewSpinner(defaultDynamicSpinner, defaultStaticSpinner, time.Millisecond)
	if s.View() != defaultStaticSpinner {
		t.Fatal("空闲态应显示静态帧")
	}
	s.isRunning = true // 不经 Start 直接置运行态：View 是纯读
	if s.View() == defaultStaticSpinner {
		t.Fatal("运行态应显示动画帧")
	}
}
