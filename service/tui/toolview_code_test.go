package tui

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tools "HackerTeam/service/engine/tools"
)

func TestLexerFor(t *testing.T) {
	l1 := lexerFor("x.go", "")
	if l1 == nil || l1.Config() == nil {
		t.Fatalf("want go lexer for .go file")
	}
	if n := l1.Config().Name; n != "Go" && n != "go" {
		t.Errorf(".go lexer name = %q", n)
	}
	// memoize：同名文件返回同一实例
	if l2 := lexerFor("x.go", ""); l2 != l1 {
		t.Errorf("lexer not memoized")
	}
	// 无扩展名：Analyse 兜底（内容像 Go）
	l3 := lexerFor("", "package main\nfunc main() {}\n")
	if l3 == nil || l3.Config() == nil {
		t.Fatalf("want analysed lexer for plain name")
	}
}

func TestHighlightCode(t *testing.T) {
	src := "func main() { return 42 }"
	out := highlightCode(src, "x.go", nil)
	if out == src {
		t.Fatalf("want ANSI highlighted output, got plain")
	}
	// 剥 ANSI 后文本内容保留（chroma 可能补行尾，trim 比较）
	if got := strings.TrimSpace(ansi.Strip(out)); got != src {
		t.Errorf("content mangled: %q", got)
	}
	// 确定性：同输入返回同输出
	if out2 := highlightCode(src, "x.go", nil); out2 != out {
		t.Errorf("non-deterministic output")
	}
	// 非 Go 文件不应产出相同高亮
	out3 := highlightCode(src, "x.py", nil)
	if out3 == out {
		t.Errorf("lexer not selected by filename")
	}
	// bg 非 nil：底色 SGR 进每个 token（48;2; = 24-bit 背景色）
	bg := highlightCode(src, "x.go", cDiffAddBg)
	if !strings.Contains(bg, "48;2;") {
		t.Errorf("bg not stamped into tokens: %q", bg)
	}
	if ansi.Strip(bg) != ansi.Strip(out) {
		t.Errorf("bg variant changed text content")
	}
}

func TestHighlightCodeConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = highlightCode("var x = 1", "x.go", nil)
			_ = highlightCode("print('hi')", "x.py", nil)
		}(i)
	}
	wg.Wait()
}

func TestCodeBlockLines(t *testing.T) {
	content := strings.Repeat("line\n", 45) // 45 行 > blockMaxLines(40)
	lines := codeBlockLines("x.go", content, 120)
	if len(lines) != 41 { // 40 行 + 尾注
		t.Fatalf("want 41 lines, got %d", len(lines))
	}
	// 行号列：前导行号 1..40
	if !strings.Contains(ansi.Strip(lines[0]), " 1") {
		t.Errorf("missing line number: %q", lines[0])
	}
	last := ansi.Strip(lines[40])
	if !strings.Contains(last, "45 lines total") || !strings.Contains(last, "showing first 40") {
		t.Errorf("truncation note wrong: %q", last)
	}
	// 不足 40 行全显：无尾注
	short := codeBlockLines("x.go", strings.Repeat("l\n", 5), 120)
	if len(short) != 5 {
		t.Fatalf("short content should show all lines, got %d", len(short))
	}
	// 高亮存在：Go 内容应带 ANSI
	if lines[0] == ansi.Strip(lines[0]) {
		t.Errorf("no syntax highlighting applied")
	}
}

func TestToolViewWriteFileArgsContentSuppressed(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"write": {Name: "write", In: []string{"path", "content"}, Out: []string{"wrote"}},
	})
	// 响应无 content（如出错路径）：参数 Content 行仍摘除（避免与响应
	// content 代码块重复），参数侧不再渲染兜底块——响应为准
	args := []byte(`{"path":"x.go","content":"func main() {\n\treturn 42\n}\n"}`)
	out := toolView("write", args, `{"wrote":44}`, 120)
	flat := ansi.Strip(out)
	if strings.Contains(flat, "func main()") {
		t.Errorf("args-side content fallback should be gone:\n%s", flat)
	}
	if strings.Contains(flat, "Content") {
		t.Errorf("Content value row should be suppressed:\n%s", flat)
	}
	if !strings.Contains(flat, "path") || !strings.Contains(flat, "wrote") {
		t.Errorf("Path/BytesWritten rows missing:\n%s", flat)
	}
}

func TestRenderDiffBlockLineNumbers(t *testing.T) {
	diff := "--- a/x.go\n+++ b/x.go\n@@ -10,3 +10,4 @@\n ctx\n-old\n+new\n+new2\n"
	lines := renderDiffBlock(textBlock{text: diff, isDiff: true}, "x.go", 120)
	// 4 行内容（---/+++/@@ 头行不渲染）
	if len(lines) != 4 {
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	flat := ansi.Strip(strings.Join(lines, "\n"))
	// 原始 diff 头不出现
	for _, noise := range []string{"--- a/x.go", "+++", "@@"} {
		if strings.Contains(flat, noise) {
			t.Errorf("raw diff header leaked: %q\n%s", noise, flat)
		}
	}
	// 单列行号：ctx = 新 10；-old = 旧 11；+new = 新 11；+new2 = 新 12
	for _, want := range []string{"10", "11", "12"} {
		if !strings.Contains(flat, want) {
			t.Errorf("line number %q missing:\n%s", want, flat)
		}
	}
	// 标记列：行号 + 空格 + +/- + 空格 + 内容
	if !strings.Contains(flat, "11 - old") || !strings.Contains(flat, "11 + new") {
		t.Errorf("diff content missing:\n%s", flat)
	}
	if strings.Count(strings.Join(lines, "\n"), "\x1b[38;2;") == 0 {
		t.Errorf("no 24-bit color in diff block")
	}
}

func TestRenderDiffBlockHunkGap(t *testing.T) {
	// 两个 hunk：中间插入弱化 … 行示意行号跳变
	diff := "@@ -1,2 +1,2 @@\n a\n-b\n@@ -10,2 +10,2 @@\n c\n+d\n"
	lines := renderDiffBlock(textBlock{text: diff, isDiff: true}, "x.go", 120)
	flat := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Count(flat, "…") != 1 {
		t.Errorf("want exactly one hunk gap:\n%s", flat)
	}
	if !strings.Contains(flat, "- b") || !strings.Contains(flat, "+ d") {
		t.Errorf("hunk content missing:\n%s", flat)
	}
}

func TestRenderDiffBlockBgStripes(t *testing.T) {
	diff := "--- a/x.go\n+++ b/x.go\n@@ -1,3 +1,3 @@\n ctx line\n-old line\n+new line\n"
	lines := renderDiffBlock(textBlock{text: diff, isDiff: true}, "x.go", 40)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	// add 底 #303a30、del 底 #3a3030（24-bit 背景 SGR 48;2;R;G;B）
	if !strings.Contains(lines[1], "48;2;58;48;48") {
		t.Errorf("del line missing red stripe: %q", lines[1])
	}
	if !strings.Contains(lines[2], "48;2;48;58;48") {
		t.Errorf("add line missing green stripe: %q", lines[2])
	}
	// ctx 行保持透明（无背景 SGR）
	if strings.Contains(lines[0], "48;2;") {
		t.Errorf("ctx line should carry no background: %q", lines[0])
	}
	// 条纹铺满整行：add/del 行显示宽 == avail
	for i := 1; i <= 2; i++ {
		if w := ansi.StringWidth(lines[i]); w != 40 {
			t.Errorf("line %d stripe not filled to avail (width %d): %q", i, w, ansi.Strip(lines[i]))
		}
	}
}

func TestRenderDiffBlockStripeWrapKeepsBg(t *testing.T) {
	// 长 add 行折行：Hardwrap 不跨行重发活跃 SGR，续行须手动补发底色
	l := "+" + strings.Repeat("x", 60)
	lines := renderDiffBlock(textBlock{text: l, isDiff: true}, "x.go", 40)
	if len(lines) < 2 {
		t.Fatalf("want wrapped lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if !strings.Contains(ln, "48;2;48;58;48") {
			t.Errorf("line %d lost add bg: %q", i, ln)
		}
		if w := ansi.StringWidth(ln); w != 40 {
			t.Errorf("line %d stripe not filled (width %d)", i, w)
		}
	}
}

func TestRenderDiffBlockTruncationCountsContent(t *testing.T) {
	// 3 头行 + 45 内容行：截断计数只算内容行（头行不计入"X lines total"）
	var b strings.Builder
	b.WriteString("--- a/x.go\n+++ b/x.go\n@@ -1,45 +1,45 @@\n")
	for i := 0; i < 45; i++ {
		b.WriteString(" ctx\n")
	}
	lines := renderDiffBlock(textBlock{text: b.String(), isDiff: true}, "x.go", 120)
	if len(lines) != 41 { // 40 内容行 + 尾注
		t.Fatalf("want 41 lines, got %d", len(lines))
	}
	last := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(last, "45 lines total") {
		t.Errorf("truncation note should count content lines only: %q", last)
	}
}

func TestToolViewWriteFileResultContent(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"write": {Name: "write", In: []string{"path", "content"}, Out: []string{"wrote"}},
	})
	// 响应携带 content（用户改造后的 WriteFile）：渲染为代码块，
	// 结果行不重复展示 content 截断值
	args := []byte(`{"Path":"x.py","content":"print(1)\nprint(2)\n"}`)
	result := `{"wrote":20,"content":"print(1)\nprint(2)\n"}`
	out := toolView("write", args, result, 120)
	flat := ansi.Strip(out)
	if !strings.Contains(flat, "print(1)") || !strings.Contains(flat, "print(2)") {
		t.Errorf("result content not rendered as code block:\n%s", flat)
	}
	if !strings.Contains(flat, "wrote") || !strings.Contains(flat, "20") {
		t.Errorf("BytesWritten row missing:\n%s", flat)
	}
	// content 只出现一次（代码块），不出现第二次（值行）
	if n := strings.Count(flat, "print(1)"); n != 1 {
		t.Errorf("content duplicated (%d occurrences)", n)
	}
}

func TestToolViewBodySectionLines(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"CD": {Name: "CD", In: []string{"Path"}, Out: []string{"CwdNow"}},
	})
	out := toolView("CD", []byte(`{"Path":"/tmp"}`), `{"CwdNow":"/home/x"}`, 120)
	lines := strings.Split(out, "\n")
	// 头部行无竖线
	if strings.HasPrefix(ansi.Strip(lines[0]), "│") {
		t.Errorf("header should not carry section line: %q", lines[0])
	}
	if len(lines) != 4 { // header + Path + 段间空行 + CwdNow
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), out)
	}
	// 参数区行带暗灰线（#605F6B = 96;95;107）
	if !strings.HasPrefix(lines[1], "\x1b[38;2;96;95;107m│") {
		t.Errorf("param line missing dark-gray line: %q", lines[1])
	}
	// 段间空行不带竖线（两段竖线在此断开）
	if strings.Contains(lines[2], "│") {
		t.Errorf("section separator should not carry line: %q", lines[2])
	}
	// 结果区行带淡绿线（#12C78F = 18;199;143）
	if !strings.HasPrefix(lines[3], "\x1b[38;2;18;199;143m│") {
		t.Errorf("result line missing soft-green line: %q", lines[3])
	}
}

func TestToolViewReadFileContent(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"read": {Name: "read", In: []string{"path"}, Out: []string{"length"}},
	})
	// ReadFile 响应的 Content（实际读出的内容）渲染为高亮代码块
	result := `{"length":28,"content":"func main() {\n\treturn 42\n}\n"}`
	out := toolView("read", []byte(`{"Path":"x.go","Bytes":1024}`), result, 120)
	flat := ansi.Strip(out)
	if !strings.Contains(flat, "func main()") || !strings.Contains(flat, "return 42") {
		t.Errorf("read content not rendered as code block:\n%s", flat)
	}
	if !strings.Contains(flat, "length") {
		t.Errorf("length row missing:\n%s", flat)
	}
	if strings.Count(flat, "func main()") != 1 {
		t.Errorf("content duplicated (%d)", strings.Count(flat, "func main()"))
	}
}
