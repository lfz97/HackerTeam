package tui

// 工具块渲染（设计文档：docs/superpowers/specs/2026-10-13-tui-tool-block-rendering-design.md）。
// 纯函数模块：composeView 每帧全量重放，无状态、无副作用。
//
// 数据流：args/result JSON → ParamMapper 字段提取（无登记走兜底）→
// 参数渲染 key value 行、结果按内容类型路由（diff 双色块 / 多行文本块 / 标量行）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tools "HackerTeam/service/engine/tools"
)

const (
	argValueMaxRunes = 80 // 参数/标量值单行截断
	blockMaxLines    = 40 // diff/长文本/代码块默认显示行数（用户指定：概览多看一点）

	// tabSpaces tab 展开宽度：ANSI 宽度测量把 \t 计 0 列而终端展开到
	// 8 列制表位，不展开则含 tab 的行"测量达标、实际越界"（crush
	// diffview 的 replaceTabs 同款处理，取 4 列保持紧凑）。
	tabSpaces = "    "
)

// codeArgFields 工具参数中的"代码内容"字段：仅从参数值行中摘除（避免与
// 响应 content 代码块重复展示，响应为准——参数侧不再渲染兜底块）。
// 渲染知识留在 UI 层（同 crush per-tool renderer 思路），工具本身零改动。
var codeArgFields = map[string]string{
	"write": "content",
}

// codeResultFields 工具结果中的"代码内容"字段：渲染为高亮代码块——
// 实际写入/读出的内容。WriteFile 响应携带小写 content；ReadFile 携带 Content。
var codeResultFields = map[string]string{
	"write": "content",
	"read":  "content",
}

// argPath 工具调用参数中的目标文件路径（选 lexer 用）。
func argPath(obj map[string]any) string {
	for _, k := range []string{"Path", "path", "PathA", "src", "File", "FilePath"} {
		if s, ok := obj[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// resultObjectOf 结果对象解析，非对象返回 nil。
func resultObjectOf(result string) map[string]any {
	obj, _ := decodeJSONObject([]byte(strings.TrimSpace(result)))
	return obj
}

// paramMappers 工具名 → 字段登记，包级一次性索引（查询 O(1)）。
// 测试可整体替换后恢复。
var paramMappers = indexMappers(tools.GetParamMapper())

// indexMappers 线性登记表建按名索引。
func indexMappers(ms *tools.Mappers) map[string]*tools.ParamMapper {
	idx := make(map[string]*tools.ParamMapper, len(*ms))
	for _, m := range *ms {
		idx[m.Name] = m
	}
	return idx
}

// kvRow 工具块内的一行键值（参数字段或结果标量字段）。
type kvRow struct{ key, val string }

// textBlock 结果里的多行内容块（diff 或长文本）。
type textBlock struct {
	text   string
	isDiff bool
}

// ── 参数行 ──────────────────────────────────────────────

// argRows 参数字段行：按 mapper.In 登记顺序提取；无登记遍历顶层键（字典序）；
// JSON 数组显示项数（todo_write 的整份清单）；非法 JSON 退化为原文单行截断。
func argRows(name string, args []byte) []kvRow {
	if len(args) == 0 {
		return nil
	}
	if obj, ok := decodeJSONObject(args); ok {
		return rowsFromKeys(obj, orderedKeys(name, obj, true))
	}
	if n, isArray := decodeJSONArray(args); isArray {
		return []kvRow{{key: "args", val: fmt.Sprintf("共 %d 项", n)}}
	}
	// 非法 JSON：截断原文兜底，宁可朴素不可消失
	s := truncateRunes(compactLine(string(args)), argValueMaxRunes)
	if s == "" || s == "{}" || s == "()" {
		return nil
	}
	return []kvRow{{key: "args", val: s}}
}

// rowsFromKeys 从 JSON 对象按键序提取键值行（值折叠多行、截断；空字符串值
// 跳过——传了但为空的字段（如 Id:""）显示成空尾巴是噪音）。
func rowsFromKeys(obj map[string]any, keys []string) []kvRow {
	var rows []kvRow
	for _, k := range keys {
		v, ok := obj[k]
		if !ok || v == nil {
			continue
		}
		s := truncateRunes(compactLine(stringValue(v)), argValueMaxRunes)
		if s == "" {
			continue
		}
		rows = append(rows, kvRow{key: k, val: s})
	}
	return rows
}

// orderedKeys 键序：有 mapper 登记按登记顺序（登记为空切片则无行），
// 无登记（MCP/未知工具）字典序兜底；纯数字键（search_in_file 的 "0"~"N"）
// 按数值排序——字典序会把 "10" 排在 "2" 前面。
func orderedKeys(name string, obj map[string]any, in bool) []string {
	var registered []string
	if m := paramMappers[name]; m != nil {
		if in {
			registered = m.In
		} else {
			registered = m.Out
		}
	}
	if registered != nil {
		return registered
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if allDigits(keys) {
		sort.Slice(keys, func(i, j int) bool {
			a, _ := strconv.Atoi(keys[i])
			b, _ := strconv.Atoi(keys[j])
			return a < b
		})
	}
	return keys
}

// allDigits 全部键均为纯数字（≥1 个）。
func allDigits(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		if _, err := strconv.Atoi(k); err != nil {
			return false
		}
	}
	return true
}

// ── 值与 JSON 工具函数 ───────────────────────────────────

// stringValue 标量值转显示文本：字符串原样，其余紧凑 JSON。
// marshalCompact 值 → 紧凑 JSON（显示用）。必须 SetEscapeHTML(false)：
// 默认 marshal 会把 & < > 转成 \u0026/\u003c/\u003e，argValueMaxRunes 截断
// 恰好切在转义序列中间时显示成 "\u002" 这样的乱码（真实现场：
// echo 'Hello from HyperBot!' && date 的命令行参数行）。
func marshalCompact(v any) (string, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", false
	}
	return strings.TrimRight(buf.String(), "\n"), true
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if s, ok := marshalCompact(v); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// decodeJSONObject 严格解析 JSON 对象（空输入/非对象/非法返回 false）。
func decodeJSONObject(data []byte) (map[string]any, bool) {
	if len(data) == 0 {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return nil, false
	}
	return obj, true
}

// decodeJSONArray 解析 JSON 数组，返回元素数与是否为数组。
func decodeJSONArray(data []byte) (int, bool) {
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		return 0, false
	}
	return len(arr), true
}

// truncateRunes 按 rune 截断（多字节安全），截断时追加省略号。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// padRight rune 宽度右填充（key 列对齐）。
func padRight(s string, w int) string {
	if gap := w - len([]rune(s)); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// ── 结果路由 ────────────────────────────────────────────

// resultParts 结果解析：JSON 对象按 mapper.Out 顺序提取字段——标量成行、
// 多行内容成块；非对象结果（JSON 字符串或原文）整体按内容类型路由。
// 空/()/{} 无输出。
func resultParts(name string, result string) ([]kvRow, []textBlock) {
	s := strings.TrimSpace(result)
	if s == "" || s == "{}" || s == "()" {
		return nil, nil
	}
	if obj, ok := decodeJSONObject([]byte(s)); ok {
		skip := codeResultFields[name] // 代码内容字段由 toolView 渲染为代码块，不重复
		var rows []kvRow
		var blocks []textBlock
		for _, k := range orderedKeys(name, obj, false) {
			if skip != "" && k == skip {
				continue
			}
			v, exists := obj[k]
			if !exists || v == nil {
				continue
			}
			str := stringValue(v)
			if expanded, ok := expandJSONList(str); ok {
				// 值本身是 JSON 数组（ls/glob 的"JSON 套 JSON"形态）：展开成
				// 每元素一行，走多行块渲染
				blocks = append(blocks, textBlock{text: expanded})
				continue
			}
			if isMultiline(str) {
				blocks = append(blocks, textBlock{text: str, isDiff: isDiff(str)})
			} else {
				s := truncateRunes(compactLine(str), argValueMaxRunes)
				if s != "" { // 空字符串值（如 Error:""）不出行
					rows = append(rows, kvRow{key: k, val: s})
				}
			}
		}
		return rows, blocks
	}
	// 非对象：JSON 字符串解开引号，否则当原文
	var str string
	if err := json.Unmarshal([]byte(s), &str); err != nil {
		str = s
	}
	if isMultiline(str) {
		return nil, []textBlock{{text: str, isDiff: isDiff(str)}}
	}
	return []kvRow{{key: "result", val: truncateRunes(compactLine(str), argValueMaxRunes)}}, nil
}

// isMultiline 清洗后含真实换行（≥2 行）即多行内容。
func isMultiline(s string) bool {
	return strings.Contains(strings.TrimSpace(s), "\n")
}

// isDiff 统一 diff 识别：首行 --- / diff --git / @@ 起头。
func isDiff(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "--- ") ||
		strings.HasPrefix(t, "diff --git") ||
		strings.HasPrefix(t, "@@ ")
}

// expandJSONList 结果值本身是 JSON 数组字符串（工具返回 map[string]string 装
// 序列化好的列表——ls 的 Files、glob 的 Matches）时展开成每元素一行的文本：
// 标量元素取裸值（glob 的路径列表去引号），其余紧凑序列化（ls 的文件对象）。
// 非数组/空数组/解析失败返回 false，走原渲染路径。
func expandJSONList(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len(t) < 2 || t[0] != '[' {
		return "", false
	}
	var arr []any
	if err := json.Unmarshal([]byte(t), &arr); err != nil || len(arr) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(arr))
	for _, e := range arr {
		if str, ok := e.(string); ok {
			parts = append(parts, str)
		} else if b, ok := marshalCompact(e); ok {
			parts = append(parts, b)
		} else {
			return "", false
		}
	}
	return strings.Join(parts, "\n"), true
}

// diffLineKind diff 行分类：add(+)/del(-)/ctx(其余，含 --- +++ @@ 与上下文)。
func diffLineKind(l string) string {
	switch {
	case strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++"):
		return "add"
	case strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---"):
		return "del"
	default:
		return "ctx"
	}
}

// cleanBlock 多行内容清洗：剥 ANSI 色码（PTY 输出防串色）、\r\n 归一、tab
// 展开为 4 空格、去首尾空白。与 compactLine 的区别：保留换行结构。
// tab 必须展开：ANSI 宽度测量把 \t 计为 0 列而终端展开到 8 列制表位，
// 含 tab 的行会"测量达标、实际超宽"，越过 padding 被终端硬换行。
func cleanBlock(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", tabSpaces)
	return strings.TrimSpace(s)
}

// ── 组装与块渲染 ────────────────────────────────────────

// lineWrap 体区行加左侧竖线前缀（与思考块/用户消息同款视觉语言）：
// 参数区/结果区竖线异色区分段（颜色承载段语义，正文保持明暗层级）。
// 竖线占 2 显示列，内容折行预算（avail = w - msgIndentW - 4）已含冗余，
// 加前缀后总宽仍 ≤ w。
func lineWrap(line string, c color.Color) string {
	return colorText(c, "│ ") + line
}

// diffHeaderVerbs 有 diff 结果的工具的块头动词（"← Edit <path>" 形式）。
// 块头承载路径后，参数区的 Path 行与之重复，toolView 会摘除。
var diffHeaderVerbs = map[string]string{
	"edit": "Edit",
	"diff": "Diff",
}

// toolView 工具块总装：头部行（✓ name）→ 参数字段行（most-subtle，最暗）→
// 空行分段 → 结果标量行 + 多行内容块（muted，亮一档）。参数与结果行同列
// 对齐，仅靠空行与明暗区分段。w 为块可用总宽（msgIndent 的缩进已扣）。
func toolView(name string, args []byte, result string, w int) string {
	avail := w - msgIndentW - 4 // 扣 msgIndent(2) 与块内容 4 空格缩进
	if avail < 10 {
		avail = 10
	}

	argsObj, _ := decodeJSONObject(args)
	path := argPath(argsObj)

	argRowList := argRows(name, args)
	resRows, blocks := resultParts(name, result)

	// 代码内容：响应侧字段（WriteFile/ReadFile 的 content/Content）渲染为
	// 高亮代码块——实际写入/读出的内容，响应为准。参数侧同名字段不再渲染
	// 兜底，仅在下方从参数值行摘除（同一数据不重复展示）。
	var codeLines []string
	if resCode := codeResultFields[name]; resCode != "" {
		if obj := resultObjectOf(result); obj != nil {
			if s, ok := obj[resCode].(string); ok && strings.TrimSpace(s) != "" {
				codeLines = codeBlockLines(path, s, avail)
			}
		}
	}
	if codeArg := codeArgFields[name]; codeArg != "" {
		rows := argRowList[:0]
		for _, r := range argRowList {
			if r.key != codeArg {
				rows = append(rows, r)
			}
		}
		argRowList = rows
	}

	all := make([]kvRow, 0, len(argRowList)+len(resRows))
	all = append(all, argRowList...)
	all = append(all, resRows...)
	if len(all) == 0 && len(blocks) == 0 && len(codeLines) == 0 {
		return toolHeader(name, "")
	}

	kw := 0
	for _, r := range all {
		if n := len([]rune(r.key)); n > kw {
			kw = n
		}
	}

	// diff 块头（← Edit <path>）：有 diff 结果且路径已知时渲染，参数区的
	// Path 行与块头重复，一并摘除
	diffHeader := ""
	if verb := diffHeaderVerbs[name]; verb != "" && path != "" {
		for _, b := range blocks {
			if b.isDiff {
				diffHeader = colorText(cInfo, "← "+verb) + " " + colorText(cFgSubtle, path)
				rows := argRowList[:0]
				for _, r := range argRowList {
					if r.key != "Path" {
						rows = append(rows, r)
					}
				}
				argRowList = rows
				break
			}
		}
	}

	// 体区（头部行之外）：参数区行（暗灰竖线）→ 空行 → 结果区行（淡绿竖线）。
	// 代码块（响应侧 content）归属结果区，接在标量行/内容块之后。
	var inLines, outLines []string
	for _, r := range argRowList {
		inLines = append(inLines, kvRowLines(r, kw, avail, cFgMostSubtle)...)
	}
	for _, r := range resRows {
		outLines = append(outLines, kvRowLines(r, kw, avail, cFgMuted)...)
	}
	if diffHeader != "" {
		outLines = append(outLines, diffHeader, "")
	}
	for _, b := range blocks {
		outLines = append(outLines, renderBlock(b, avail, path)...)
	}
	outLines = append(outLines, codeLines...)

	lines := make([]string, 0, len(inLines)+len(outLines)+2)
	lines = append(lines, toolHeader(name, ""))
	for _, l := range inLines {
		lines = append(lines, lineWrap(l, cToolLineIn))
	}
	// 段间空行不带竖线：参数段/结果段的竖线在此断开、各自成段
	// （段内空行——如参数行与代码块之间——仍带竖线保持段内连续）
	if len(inLines) > 0 && len(outLines) > 0 {
		lines = append(lines, "")
	}
	for _, l := range outLines {
		lines = append(lines, lineWrap(l, cToolLineOut))
	}
	return strings.Join(lines, "\n")
}

// kvRowLines 键值行：值超宽按列硬折行（续行对齐 value 起始列，不越过
// padding），整行统一配色。值无 ANSI（已截断的纯文本），Hardwrap 安全。
func kvRowLines(r kvRow, kw, avail int, c color.Color) []string {
	prefix := "  " + padRight(r.key, kw) + "  "
	limit := avail - len(prefix) // prefix 全为窄字符，字节数即显示列数
	if limit < 4 {
		limit = 4
	}
	chunks := hardWrapIf(r.val, limit)
	lines := make([]string, len(chunks))
	for i, chunk := range chunks {
		p := prefix
		if i > 0 {
			p = strings.Repeat(" ", len(prefix))
		}
		lines[i] = colorText(c, p+chunk)
	}
	return lines
}

// renderBlock 多行内容块：每行 4 空格缩进、muted 色；diff 块分流到
// renderDiffBlock（行号列 + 行内语法高亮）；行内容按 avail 硬折行；
// 超过 blockMaxLines 截断并加最暗色尾注。path 为编辑目标文件路径（diff
// 块选 lexer 用，空则不高亮）。
func renderBlock(b textBlock, avail int, path string) []string {
	if b.isDiff {
		return renderDiffBlock(b, path, avail)
	}
	if avail < 10 {
		avail = 10
	}
	all := strings.Split(cleanBlock(b.text), "\n")
	total := len(all)
	if total > blockMaxLines {
		all = all[:blockMaxLines]
	}
	out := make([]string, 0, len(all)+1)
	for _, l := range all {
		for _, chunk := range hardWrapIf(l, avail) {
			out = append(out, "  "+colorText(cFgMuted, chunk))
		}
	}
	if total > blockMaxLines {
		note := fmt.Sprintf("… 共 %d 行，显示前 %d 行", total, blockMaxLines)
		for _, chunk := range hardWrapIf(note, avail) {
			out = append(out, colorText(cFgMostSubtle, "  "+chunk))
		}
	}
	return out
}

// diffHunkHeader 匹配 unified diff hunk 头，提取 old/new 起始行号。
var diffHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// renderDiffBlock unified diff 块：行号列（@@ 头解析并逐行推进，add/del 行
// 各自只占一侧）+ 行内语法高亮（lexer 取自 path）+ +/- 前缀染色。
// add/del 行整行铺底色条纹（crush Insert/DeleteLine 同型）：行号列底色
// 深一档、内容列底色进每个语法 token，行尾补齐铺满；ctx 行保持透明。
// 语法色与 diff 色分层：内容承载 chroma 语法色，条纹与标记承载 kind 色。
// 原始 diff 头行（---/+++/@@）不渲染——行号与染色已是完整信息，原始
// hunk 语法是噪音；多 hunk 之间以弱化 … 行示意行号跳变；截断只计内容行。
func renderDiffBlock(b textBlock, path string, avail int) []string {
	if avail < 10 {
		avail = 10
	}
	all := strings.Split(cleanBlock(b.text), "\n")

	// 第一遍：解析为内容条目。头行只推进行号计数不产出。
	type diffEntry struct {
		num     int    // 单列行号：上下文/新增取新文件行号，删除取旧文件行号
		kind    string // add | del | ctx | gap
		content string
	}
	var entries []diffEntry
	oldLine, newLine := 0, 0
	for _, l := range all {
		if m := diffHunkHeader.FindStringSubmatch(l); m != nil {
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[2])
			if len(entries) > 0 {
				entries = append(entries, diffEntry{kind: "gap"})
			}
			continue
		}
		if strings.HasPrefix(l, "---") || strings.HasPrefix(l, "+++") ||
			strings.HasPrefix(l, "diff --git") {
			continue
		}
		switch {
		case strings.HasPrefix(l, "-"):
			entries = append(entries, diffEntry{num: oldLine, kind: "del", content: l[1:]})
			oldLine++
		case strings.HasPrefix(l, "+"):
			entries = append(entries, diffEntry{num: newLine, kind: "add", content: l[1:]})
			newLine++
		case strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\\"):
			entries = append(entries, diffEntry{num: newLine, kind: "ctx", content: strings.TrimPrefix(l, " ")})
			oldLine++
			newLine++
		}
	}

	// 截断：只计内容行（gap 不计），保留前 blockMaxLines 个内容行及其伴随 gap
	contentTotal := 0
	for _, e := range entries {
		if e.kind != "gap" {
			contentTotal++
		}
	}
	shown := entries
	if contentTotal > blockMaxLines {
		kept, n := make([]diffEntry, 0, len(entries)), 0
		for _, e := range entries {
			if e.kind != "gap" {
				if n == blockMaxLines {
					break
				}
				n++
			}
			kept = append(kept, e)
		}
		shown = kept
	}

	// 单列行号宽 + 标记列（num + " " + marker + " "，合计 numCol）
	maxNum := 1
	for _, e := range shown {
		if e.num > maxNum {
			maxNum = e.num
		}
	}
	numW := len(fmt.Sprintf("%d", maxNum))
	numCol := numW + 3

	renderNum := func(n int) string {
		if n == 0 {
			return colorText(cFgMostSubtle, strings.Repeat(" ", numW))
		}
		return colorText(cFgMostSubtle, fmt.Sprintf("%*d", numW, n))
	}
	numText := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", numW)
		}
		return fmt.Sprintf("%*d", numW, n)
	}
	// 标记列承载 diff kind 色（+ 淡绿 / - 红 / 上下空白）
	markerChar := func(kind string) string {
		switch kind {
		case "add":
			return "+"
		case "del":
			return "-"
		default:
			return " "
		}
	}
	diffMarker := func(kind string) string {
		switch kind {
		case "add":
			return colorText(cSuccessSubtle, "+")
		case "del":
			return colorText(cError, "-")
		default:
			return " "
		}
	}
	blankCol := colorText(cFgMostSubtle, strings.Repeat(" ", numCol))

	limit := avail - numCol
	if limit < 4 {
		limit = 4
	}
	out := make([]string, 0, len(shown)+1)
	for _, e := range shown {
		if e.kind == "gap" {
			out = append(out, blankCol+colorText(cFgMostSubtle, "…"))
			continue
		}
		// add/del 行整行铺条纹：gutter 两档底色、内容列底色进每个语法
		// token（有 path）或整格承载（无 path）、行尾补齐铺满 limit；
		// ctx 行无底色保持透明。
		gutBg, conBg := diffKindBg(e.kind)
		kindColor := diffKindColor(e.kind)
		bgSeq := ""
		if conBg != nil {
			bgSeq = bgSGRSeq(conBg)
		}

		var chunks []string
		if path == "" {
			// 无文件上下文（如 Diff 工具的裸 diff）：内容整段按 kind 染色，
			// 折行后每个 chunk 独立染色（kind 贯穿整逻辑行）
			chunks = hardWrapIf(e.content, limit)
		} else {
			// 有文件路径：语法色打底、底色随 formatter 进每个 token
			chunks = hardWrapIf(highlightCode(e.content, path, conBg), limit)
		}
		for i, chunk := range chunks {
			var line string
			switch {
			case conBg != nil && i == 0:
				line = diffCell(kindColor, gutBg, numText(e.num)) +
					diffCell(kindColor, conBg, " "+markerChar(e.kind)+" ")
			case conBg != nil:
				line = bgCell(gutBg, strings.Repeat(" ", numW)) + bgCell(conBg, "   ")
			case i == 0:
				line = renderNum(e.num) + " " + diffMarker(e.kind) + " "
			default:
				line = blankCol
			}
			switch {
			case path == "":
				line += diffCell(kindColor, conBg, chunk)
			case i > 0:
				// Hardwrap 不跨行重发活跃 SGR：续行手动补发底色
				line += bgSeq + chunk
			default:
				line += chunk
			}
			if conBg != nil {
				if pad := limit - ansi.StringWidth(chunk); pad > 0 {
					line += bgCell(conBg, strings.Repeat(" ", pad))
				}
			}
			out = append(out, line)
		}
	}
	if contentTotal > blockMaxLines {
		note := fmt.Sprintf("… 共 %d 行，显示前 %d 行", contentTotal, blockMaxLines)
		for _, chunk := range hardWrapIf(note, limit) {
			out = append(out, blankCol+colorText(cFgMostSubtle, chunk))
		}
	}
	return out
}

// diffKindColor diff 行分类配色。
// diffKindColor diff 行分类配色。
func diffKindColor(kind string) color.Color {
	switch kind {
	case "add":
		return cSuccessSubtle
	case "del":
		return cError
	default:
		return cFgMuted
	}
}

// hardWrapIf 超宽才按列硬折行（ANSI 感知、保显示列对齐），否则原样单行。
func hardWrapIf(s string, limit int) []string {
	if limit < 1 || ansi.StringWidth(s) <= limit {
		return []string{s}
	}
	return strings.Split(ansi.Hardwrap(s, limit, false), "\n")
}
