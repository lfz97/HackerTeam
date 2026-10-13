package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tools "HackerTeam/service/engine/tools"
)

// withMappers 测试期替换包级 mapper 索引，结束后恢复（隔离测试）。
func withMappers(t *testing.T, m map[string]*tools.ParamMapper) {
	t.Helper()
	orig := paramMappers
	paramMappers = m
	t.Cleanup(func() { paramMappers = orig })
}

func TestArgRowsMapperOrder(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"edit": {Name: "edit", In: []string{"Path", "Regex"}, Out: []string{"Diff"}},
	})
	rows := argRows("edit", []byte(`{"Regex":"foo","Path":"a.go"}`))
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %#v", len(rows), rows)
	}
	if rows[0].key != "Path" || rows[0].val != "a.go" {
		t.Errorf("row0 = %q %q, want Path a.go", rows[0].key, rows[0].val)
	}
	if rows[1].key != "Regex" || rows[1].val != "foo" {
		t.Errorf("row1 = %q %q, want Regex foo", rows[1].key, rows[1].val)
	}
}

func TestArgRowsFallbackSortedKeys(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	rows := argRows("mcp_unknown", []byte(`{"b":"2","a":"1"}`))
	if len(rows) != 2 || rows[0].key != "a" || rows[1].key != "b" {
		t.Fatalf("want sorted keys a,b, got %#v", rows)
	}
}

func TestArgRowsInvalidJSONFallsBackToRaw(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	rows := argRows("mcp_unknown", []byte(`not json`))
	if len(rows) != 1 || rows[0].key != "args" || rows[0].val != "not json" {
		t.Fatalf("want raw fallback row, got %#v", rows)
	}
}

func TestArgRowsArrayCount(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"todo_write": {Name: "todo_write", In: []string{}, Out: []string{"message"}},
	})
	rows := argRows("todo_write", []byte(`[{"id":1},{"id":2},{"id":3}]`))
	if len(rows) != 1 || rows[0].val != "共 3 项" {
		t.Fatalf("want array count row, got %#v", rows)
	}
}

func TestArgRowsEmptyAndTruncate(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	if rows := argRows("x", []byte(`{}`)); len(rows) != 0 {
		t.Fatalf("want no rows for {}, got %#v", rows)
	}
	if rows := argRows("x", nil); len(rows) != 0 {
		t.Fatalf("want no rows for nil, got %#v", rows)
	}
	rows := argRows("x", []byte(`{"k":"`+strings.Repeat("一", 45)+`"}`))
	if len([]rune(rows[0].val)) > 41 { // 40 rune + 省略号
		t.Fatalf("value not truncated: %q", rows[0].val)
	}
	if !strings.HasSuffix(rows[0].val, "…") {
		t.Fatalf("want ellipsis suffix, got %q", rows[0].val)
	}
}

func TestStringValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{1234.0, "1234"},
		{true, "true"},
		{"s", "s"},
		{[]any{1.0, 2.0}, "[1,2]"},
	}
	for _, c := range cases {
		if got := stringValue(c.in); got != c.want {
			t.Errorf("stringValue(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIndexMappers(t *testing.T) {
	ms := &tools.Mappers{}
	ms.AddMapping("a", []string{}, []string{})
	ms.AddMapping("b", []string{"X"}, []string{})
	idx := indexMappers(ms)
	if idx["a"] == nil || idx["b"] == nil || idx["b"].In[0] != "X" {
		t.Fatalf("index broken: %#v", idx)
	}
}

const testDiff = `--- a/x.go
+++ b/x.go
@@ -1,3 +1,4 @@
 ctx
-old
+new
+new2
`

func TestResultPartsDiffRouting(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"edit": {Name: "edit", In: []string{"Path"}, Out: []string{"Diff"}},
	})
	rows, blocks := resultParts("edit", `{"Diff":`+strconv.Quote(testDiff)+`}`)
	if len(rows) != 0 || len(blocks) != 1 {
		t.Fatalf("want 0 rows 1 block, got %d rows %d blocks", len(rows), len(blocks))
	}
	if !blocks[0].isDiff {
		t.Fatalf("want diff block, got %#v", blocks[0])
	}
}

func TestResultPartsLongTextRouting(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"exec": {Name: "exec", In: []string{"Cmd"}, Out: []string{"Output"}},
	})
	rows, blocks := resultParts("exec", `{"Output":"total 24\ndrwxr-xr-x 5\n-rw-r--r-- 1"}`)
	if len(rows) != 0 || len(blocks) != 1 || blocks[0].isDiff {
		t.Fatalf("want 1 text block, got rows=%#v blocks=%#v", rows, blocks)
	}
}

func TestResultPartsScalarRow(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"write": {Name: "write", In: []string{"Path"}, Out: []string{"BytesWritten"}},
	})
	rows, blocks := resultParts("write", `{"BytesWritten":1234}`)
	if len(rows) != 1 || rows[0].key != "BytesWritten" || rows[0].val != "1234" || len(blocks) != 0 {
		t.Fatalf("want scalar row, got rows=%#v blocks=%#v", rows, blocks)
	}
}

func TestResultPartsNonObjectResult(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	// JSON 字符串整体路由为内容块
	_, blocks := resultParts("raw", `"line1\nline2\nline3"`)
	if len(blocks) != 1 || blocks[0].isDiff {
		t.Fatalf("want text block, got %#v", blocks)
	}
	// 非法 JSON 原文兜底
	rows, _ := resultParts("raw", `plain result`)
	if len(rows) != 1 || rows[0].key != "result" || rows[0].val != "plain result" {
		t.Fatalf("want result fallback row, got %#v", rows)
	}
}

func TestResultPartsEmpty(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	for _, s := range []string{"", "{}", "()", "   "} {
		if rows, blocks := resultParts("x", s); len(rows) != 0 || len(blocks) != 0 {
			t.Fatalf("want nothing for %q, got %#v %#v", s, rows, blocks)
		}
	}
}

func TestDiffLineKind(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"+new", "add"},
		{"+++ b/x.go", "ctx"},
		{"-old", "del"},
		{"--- a/x.go", "ctx"},
		{"@@ -1,3 +1,4 @@", "ctx"},
		{" ctx", "ctx"},
		{"", "ctx"},
	}
	for _, c := range cases {
		if got := diffLineKind(c.line); got != c.want {
			t.Errorf("diffLineKind(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestCleanBlock(t *testing.T) {
	in := "  \x1b[32mgreen\x1b[m\r\nline2\r\n"
	if got := cleanBlock(in); got != "green\nline2" {
		t.Fatalf("cleanBlock = %q, want green\\nline2", got)
	}
}

func TestToolViewAssembly(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"edit": {Name: "edit", In: []string{"Path"}, Out: []string{"Diff"}},
	})
	args := []byte(`{"Path":"service/tui/render.go"}`)
	out := toolView("edit", args, `{"Diff":"--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-old\n+new"}`, 120)
	lines := strings.Split(out, "\n")
	// header + ←Edit 块头（Path 行摘除）+ 空行 + 2 行内容（diff 头行不渲染）
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "edit") {
		t.Errorf("header missing tool name: %q", lines[0])
	}
	// ← Edit 块头携带路径；Path 参数行被摘除（去重）
	if !strings.Contains(ansi.Strip(lines[1]), "← Edit") || !strings.Contains(lines[1], "render.go") {
		t.Errorf("diff header broken: %q", lines[1])
	}
	if strings.Contains(ansi.Strip(lines[1]), "Path") {
		t.Errorf("Path row should be suppressed by diff header: %q", lines[1])
	}
	if strings.TrimSpace(strings.TrimPrefix(ansi.Strip(lines[2]), "│")) != "" {
		t.Errorf("want blank separator line, got %q", lines[2])
	}
	// diff 头行剥离：不出现原始 hunk 语法
	for _, noise := range []string{"--- a/x.go", "+++", "@@"} {
		if strings.Contains(ansi.Strip(out), noise) {
			t.Errorf("raw diff header leaked: %q\n%s", noise, ansi.Strip(out))
		}
	}
	// + 行染绿、- 行染红（语法色分层：前缀承载 diff 色，内容承载 chroma 色。
	// 断言须剥 ANSI：前缀与内容之间隔着 SGR 转义码）
	flat := ansi.Strip(out)
	if !strings.Contains(flat, "+ new") || !strings.Contains(flat, "- old") {
		t.Errorf("diff content missing:\n%s", flat)
	}
}

func TestToolViewKeyAlignment(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"x": {Name: "x", In: []string{"K", "LongerKey"}, Out: []string{"R"}},
	})
	out := toolView("x", []byte(`{"K":"v1","LongerKey":"v2"}`), `{"R":"ok"}`, 120)
	lines := strings.Split(out, "\n")
	if len(lines) != 5 { // header + 2 参数 + 空行 + 1 结果
		t.Fatalf("want 5 lines, got %d:\n%s", len(lines), out)
	}
	// 两行参数的 ANSI 前缀相同（同为 most-subtle 单色），value 起始列必须相等
	if i1, i2 := strings.Index(lines[1], "v1"), strings.Index(lines[2], "v2"); i1 != i2 {
		t.Errorf("value columns misaligned (%d vs %d): %q / %q", i1, i2, lines[1], lines[2])
	}
	if !strings.Contains(lines[2], "LongerKey") {
		t.Errorf("LongerKey row broken: %q", lines[2])
	}
	// 结果行与参数行同列对齐（无符号前缀）——参数/结果配色不同，ANSI 前缀
	// 字节长不同，须剥 ANSI 后按显示列比较
	if i1, i2 := strings.Index(ansi.Strip(lines[1]), "v1"), strings.Index(ansi.Strip(lines[4]), "ok"); i1 != i2 {
		t.Errorf("result value misaligned vs param (%d vs %d)", i1, i2)
	}
}

func TestToolViewSectionBrightness(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"x": {Name: "x", In: []string{"K"}, Out: []string{"R"}},
	})
	out := toolView("x", []byte(`{"K":"v"}`), `{"R":"v"}`, 120)
	lines := strings.Split(out, "\n")
	// 参数 #605F6B（96;95;107）、结果 #858392（133;131;146）——明暗区分
	if !strings.Contains(lines[1], "96;95;107") {
		t.Errorf("param not most-subtle: %q", lines[1])
	}
	if !strings.Contains(lines[3], "133;131;146") {
		t.Errorf("result not muted: %q", lines[3])
	}
}

func TestToolViewBlankLineOnlyBetweenSections(t *testing.T) {
	// 空行判定按视觉：段间空行不带竖线（裸空行）；段内空行带竖线
	visBlanks := func(out string) int {
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(ansi.Strip(l)) == "" {
				n++
			}
		}
		return n
	}
	// 只有参数：无空行
	withMappers(t, map[string]*tools.ParamMapper{
		"p": {Name: "p", In: []string{"K"}, Out: nil},
	})
	if out := toolView("p", []byte(`{"K":"v"}`), "", 120); visBlanks(out) != 0 {
		t.Errorf("unexpected blank line (params only):\n%s", out)
	}
	// 只有结果：无空行
	withMappers(t, map[string]*tools.ParamMapper{
		"r": {Name: "r", In: nil, Out: []string{"R"}},
	})
	if out := toolView("r", nil, `{"R":"v"}`, 120); visBlanks(out) != 0 {
		t.Errorf("unexpected blank line (result only):\n%s", out)
	}
	// 都有：恰一行空行
	withMappers(t, map[string]*tools.ParamMapper{
		"b": {Name: "b", In: []string{"K"}, Out: []string{"R"}},
	})
	if out := toolView("b", []byte(`{"K":"v"}`), `{"R":"v"}`, 120); visBlanks(out) != 1 {
		t.Errorf("want exactly one blank separator:\n%s", out)
	}
}

func TestRenderBlockTruncation(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 45; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	lines := renderBlock(textBlock{text: b.String()}, 120, "")
	if len(lines) != 41 { // 40 行 + 尾注
		t.Fatalf("want 41 lines, got %d", len(lines))
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "共 45 行") || !strings.Contains(last, "前 40 行") {
		t.Fatalf("truncation note wrong: %q", last)
	}
}

func TestRenderBlockWrapsLongLines(t *testing.T) {
	long := strings.Repeat("a", 50)
	lines := renderBlock(textBlock{text: long}, 30, "") // 50 列 → 30 + 20
	if len(lines) != 2 {
		t.Fatalf("want 2 wrapped lines, got %d: %#v", len(lines), lines)
	}
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if joined != "  "+strings.Repeat("a", 30)+"\n  "+strings.Repeat("a", 20) {
		t.Fatalf("wrap mangled content: %q", joined)
	}
}

func TestRenderBlockDiffWrapKeepsKind(t *testing.T) {
	// 40 列的删除行在 20 列限宽下折行，续行保持红色（kind 贯穿整逻辑行）
	l := "-" + strings.Repeat("x", 39)
	lines := renderBlock(textBlock{text: l, isDiff: true}, 30, "")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if !strings.Contains(ln, "38;2;235;66;104") { // cError #EB4268
			t.Errorf("line %d lost del color: %q", i, ln)
		}
	}
}

func TestToolViewRowWrapNarrowWidth(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"x": {Name: "x", In: []string{"K"}, Out: nil},
	})
	// w=20：avail=14，值 41 个全宽字符必然折行；任何行显示宽不得越过
	// w - msgIndentW = 18
	out := toolView("x", []byte(`{"K":"`+strings.Repeat("一", 45)+`"}`), "", 20)
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("value not wrapped:\n%s", out)
	}
	for i, ln := range lines {
		if lw := ansi.StringWidth(ln); lw > 18 {
			t.Errorf("line %d overflows padding (width %d): %q", i, lw, ansi.Strip(ln))
		}
	}
	if n := strings.Count(ansi.Strip(out), "一"); n != 40 { // 截断后 40 个 + …
		t.Errorf("content lost in wrap/truncate: got %d 一, want 40", n)
	}
}

func TestToolViewEmptyArgsResult(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	out := toolView("date_now", nil, "", 120)
	lines := strings.Split(out, "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "date_now") {
		t.Fatalf("want header only, got:\n%s", out)
	}
}

func TestOrderedKeysNumericSort(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{})
	obj := map[string]any{"10": "a", "2": "b", "0": "c", "1": "d"}
	got := orderedKeys("unregistered", obj, false)
	want := []string{"0", "1", "2", "10"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("numeric sort broken: got %v, want %v", got, want)
	}
}

func TestExpandJSONList(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`["a.go","b.go"]`, "a.go\nb.go", true},
		{`[{"A":1},{"B":2}]`, "{\"A\":1}\n{\"B\":2}", true},
		{`[]`, "", false},
		{`{"k":"v"}`, "", false},
		{`plain text`, "", false},
	}
	for _, c := range cases {
		got, ok := expandJSONList(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("expandJSONList(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestResultPartsListExpansion(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"glob": {Name: "glob", In: []string{"Regex"}, Out: []string{"Matches"}},
	})
	rows, blocks := resultParts("glob", `{"Matches":"[\"a.go\",\"b.go\",\"c.go\"]"}`)
	if len(rows) != 0 || len(blocks) != 1 {
		t.Fatalf("want 0 rows 1 block, got rows=%#v blocks=%#v", rows, blocks)
	}
	if blocks[0].isDiff {
		t.Fatalf("list block misdetected as diff")
	}
	lines := strings.Split(cleanBlock(blocks[0].text), "\n")
	if len(lines) != 3 || lines[0] != "a.go" || lines[2] != "c.go" {
		t.Fatalf("list expansion broken: %#v", lines)
	}
}

func TestArgRowsSkipEmptyValues(t *testing.T) {
	withMappers(t, map[string]*tools.ParamMapper{
		"status": {Name: "status", In: []string{"Id", "WaitSeconds"}, Out: nil},
	})
	rows := argRows("status", []byte(`{"Id":"","WaitSeconds":0}`))
	if len(rows) != 1 || rows[0].key != "WaitSeconds" {
		t.Fatalf("want empty-string value skipped, got %#v", rows)
	}
	// 数值 0 不是空值，应保留
	if rows[0].val != "0" {
		t.Fatalf("numeric zero should be kept, got %q", rows[0].val)
	}
}

func TestToolViewSearchInFileFallback(t *testing.T) {
	// search_in_file 形态：Out 无登记（nil）→ 数字键自然排序，每条匹配一行
	withMappers(t, map[string]*tools.ParamMapper{
		"search": {Name: "search", In: []string{"Path", "Regex"}, Out: nil},
	})
	res := `{"10":"m10","2":"m2","0":"m0"}`
	out := toolView("search", []byte(`{"Path":"x.go","Regex":"foo"}`), res, 120)
	lines := strings.Split(out, "\n")
	// header + 2 参数行 + 空行 + 3 结果行
	if len(lines) != 7 {
		t.Fatalf("want 7 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[6], "m10") {
		t.Errorf("numeric order broken, last row: %q", lines[6])
	}
	if !strings.Contains(lines[4], "m0") {
		t.Errorf("numeric order broken, first result row: %q", lines[4])
	}
}

// 回归：显示层 re-marshal 禁用 HTML 转义——默认 json.Marshal 会把 & < > 变成
// \u0026 等，40 字符截断切在转义序列中间显示成 "\u002"（现场：echo '…' && date）。
func TestStringValueNoHTMLEscape(t *testing.T) {
	got := stringValue([]any{"-c", "echo 'Hello from HyperBot!' && date && whoami && hostname"})
	if strings.Contains(got, `\u00`) {
		t.Errorf("HTML escape leaked into display value: %s", got)
	}
	if !strings.Contains(got, "&&") {
		t.Errorf("&& lost: %s", got)
	}
	got = stringValue(map[string]any{"cmd": "a < b > c & d"})
	if strings.Contains(got, `\u00`) || !strings.Contains(got, "<") {
		t.Errorf("HTML escape leaked: %s", got)
	}
}

// 端到端：超长 && 命令行参数经截断后不得出现转义序列残片。
func TestToolViewTruncationNotMidEscape(t *testing.T) {
	args := []byte(`{"Process":"bash","Args":["-c","echo 'Hello from HyperBot!' && date && whoami && hostname"]}`)
	out := toolView("exec", args, `{"Status":"done"}`, 120)
	flat := ansi.Strip(out)
	if strings.Contains(flat, `\u00`) {
		t.Errorf("escape fragment in truncated display:\n%s", flat)
	}
}
