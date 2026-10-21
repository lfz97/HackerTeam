package modeltext

import "testing"

// 序列化放大：非法 UTF-8 与控制字符每字节 6 字节转义，合法文本近似 1:1。
func TestSize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 2},        // 只有两端引号
		{"ascii", "hello\n", 9}, // \n 转义成 2 字节
		{"chinese", "中文", 8},    // 3 字节 x2 + 引号，合法 UTF-8 不放大
		{"invalid", "\xB4\xB4", 14},
		{"nul", "\x00\x00", 14},
	}
	for _, tc := range cases {
		if got := Size(tc.in); got != tc.want {
			t.Errorf("%s: Size(%q) = %d, want %d", tc.name, tc.in, got, tc.want)
		}
	}
	// 放大方向性：序列化结果不小于原始长度（ Size >= len + 2）
	for _, s := range []string{"a\nb", "\xB4", "\x00\x00\x00", "中文"} {
		if Size(s) < len(s)+2 {
			t.Errorf("Size(%q) = %d, smaller than raw %d", s, Size(s), len(s))
		}
	}
}

// 窗口劈开的半个 rune 被去掉，完整的 rune（哪怕内容是非法字节）保留。
func TestTrimPartialRune(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean ascii", "abc", "abc"},
		{"clean chinese", "中", "中"},
		{"split rune", string([]byte{0xE4, 0xB8}), ""},   // 「中」的前两个字节
		{"split then clean", "a\xe4\xb8", "a"},           // 劈开的 rune 在末尾
		{"invalid bytes kept", "\xB4\xB4", "\xB4\xB4"},   // 非法但不是被劈开的，不动
		{"invalid rune dropped", "\xE4\xB8\xB4\xB4", ""}, // 末尾整体非法，按被截断的 rune 整段丢弃
	}
	for _, tc := range cases {
		if got := string(TrimPartialRune([]byte(tc.in))); got != tc.want {
			t.Errorf("%s: TrimPartialRune(% x) = % x, want % x", tc.name, tc.in, got, tc.want)
		}
	}
}
