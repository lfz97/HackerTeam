package tui

// 工具块的 chroma 语法高亮。配色取 markdownStyle() 的 Chroma 同款（视觉
// 语言一致）。composeView 每帧全量重放 → lexer 与格式化结果均按
// (filename, content) 缓存，带锁（渲染可能多 goroutine 触达）。

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
)

var (
	lexMu    sync.Mutex
	lexCache = map[string]chroma.Lexer{}

	hlMu    sync.Mutex
	hlCache = map[string]string{} // key: filename + "\x00" + source
)

// codeStyle 项目调色板映射到 chroma style（一次性构建）。
var codeStyle = sync.OnceValue(func() *chroma.Style {
	s, _ := chroma.NewStyle("hyperbot", chroma.StyleEntries{
		chroma.Text:          "#BFBCC8",
		chroma.Comment:       "#605F6B",
		chroma.Keyword:       "#00A4FF",
		chroma.KeywordType:   "#4FBEFE",
		chroma.LiteralString: "#E8FE96",
		chroma.LiteralNumber: "#00FFB2",
		chroma.Name:          "#BFBCC8",
		chroma.NameFunction:  "#12C78F",
		chroma.NameClass:     "#6B50FF",
		chroma.NameBuiltin:   "#FF60FF",
		chroma.NameTag:       "#FF60FF",
		chroma.NameAttribute: "#4FBEFE",
		chroma.Operator:      "#BFBCC8",
		chroma.Punctuation:   "#E8FE96",
	})
	return s
})

// lexerFor 按文件名选 lexer（memoize）；匹配不上用内容分析兜底，再兜 Fallback。
func lexerFor(filename, source string) chroma.Lexer {
	lexMu.Lock()
	l, ok := lexCache[filename]
	lexMu.Unlock()
	if ok {
		return l
	}
	l = lexers.Match(filename)
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		l = lexers.Fallback
	}
	lexMu.Lock()
	lexCache[filename] = l
	lexMu.Unlock()
	return l
}

// highlightCode 源码 → 24-bit ANSI 高亮文本（按 filename+source 缓存）。
// 缓存超限（512）整体丢弃重建：会话内工具块数量有限，泄漏兜底优先于淘汰精度。
func highlightCode(source, filename string) string {
	key := filename + "\x00" + source
	hlMu.Lock()
	if v, ok := hlCache[key]; ok {
		hlMu.Unlock()
		return v
	}
	if len(hlCache) > 512 {
		hlCache = map[string]string{}
	}
	hlMu.Unlock()

	l := lexerFor(filename, source)
	// 不用 TokeniseOptions{EnsureLF: true}：chroma v2.27.0 里它会触发 lexer
	// 状态机 "unknown state" panic（追加换行后栈状态错乱），nil options 正常。
	it, err := l.Tokenise(nil, source)
	if err != nil {
		return source
	}
	f := formatters.Get("terminal16m")
	if f == nil {
		f = formatters.Fallback
	}
	var buf bytes.Buffer
	if err := f.Format(&buf, codeStyle(), it); err != nil {
		return source
	}
	out := buf.String()
	hlMu.Lock()
	hlCache[key] = out
	hlMu.Unlock()
	return out
}

// codeBlockLines 高亮代码块：行号列（most-subtle 右对齐）+ chroma 高亮 +
// blockMaxLines 截断与尾注。写入内容展示用（crush toolOutputCodeContent 同型）。
func codeBlockLines(path, content string, avail int) []string {
	if avail < 10 {
		avail = 10
	}
	lines := strings.Split(cleanBlock(content), "\n")
	total := len(lines)
	if total > blockMaxLines {
		lines = lines[:blockMaxLines]
	}
	// 高亮整段再按行拆：块内多行 token（块注释等）颜色上下文连续
	hl := strings.Split(highlightCode(strings.Join(lines, "\n"), path), "\n")
	if len(hl) < len(lines) { // 高亮输出行数异常时兜底原文
		hl = lines
	}
	numW := len(fmt.Sprintf("%d", len(lines)))
	var out []string
	for i, ln := range hl {
		num := colorText(cFgMostSubtle, fmt.Sprintf("%*d", numW, i+1))
		body := ansi.Truncate(ln, avail-numW-2, "…")
		out = append(out, num+"  "+body)
	}
	if total > blockMaxLines {
		note := fmt.Sprintf("… 共 %d 行，显示前 %d 行", total, blockMaxLines)
		out = append(out, colorText(cFgMostSubtle,
			fmt.Sprintf("%*s  %s", numW, "", truncateRunes(note, avail-numW-2))))
	}
	return out
}
