package tui

// 工具块的 chroma 语法高亮，配色与 markdownStyle() 一致。无缓存：高亮
// 成本毫秒级不可感知，渲染管线保持无全局可变状态。

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
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

// lexerFor 按文件名选 lexer；匹配不上用内容分析兜底，再兜 Fallback。
func lexerFor(filename, source string) chroma.Lexer {
	l := lexers.Match(filename)
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		l = lexers.Fallback
	}
	return l
}

// highlightCode 源码 → 24-bit ANSI 高亮。bg 非 nil 时每个 token 的 SGR
// 自带底色（diff 条纹用）——底色跟着 token 走，chroma 的逐 token reset 冲不掉。
func highlightCode(source, filename string, bg color.Color) string {
	l := lexerFor(filename, source)
	// 不用 TokeniseOptions{EnsureLF: true}：chroma v2.27.0 会触发 lexer
	// "unknown state" panic，nil options 正常。
	it, err := l.Tokenise(nil, source)
	if err != nil {
		return source
	}
	var buf bytes.Buffer
	if err := chromaFormatter(bg).Format(&buf, codeStyle(), it); err != nil {
		return source
	}
	return buf.String()
}

// chromaFormatter bg==nil 走 terminal16m；否则逐 token 用 lipgloss 渲染，
// 语法前景色 + 强制底色（crush internal/ui/xchroma 同款分层）。
func chromaFormatter(bg color.Color) chroma.Formatter {
	if bg == nil {
		if f := formatters.Get("terminal16m"); f != nil {
			return f
		}
		return formatters.Fallback
	}
	return chroma.FormatterFunc(func(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
		for token := it(); token != chroma.EOF; token = it() {
			entry := style.Get(token.Type)
			s := lipgloss.NewStyle().Background(bg)
			if entry.Bold == chroma.Yes {
				s = s.Bold(true)
			}
			if entry.Underline == chroma.Yes {
				s = s.Underline(true)
			}
			if entry.Italic == chroma.Yes {
				s = s.Italic(true)
			}
			if entry.Colour.IsSet() {
				s = s.Foreground(lipgloss.Color(entry.Colour.String()))
			}
			if _, err := io.WriteString(w, s.Render(token.Value)); err != nil {
				return err
			}
		}
		return nil
	})
}

// codeBlockLines 高亮代码块：行号列（most-subtle 右对齐）+ chroma 高亮 +
// blockMaxLines 截断与尾注。
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
	hl := strings.Split(highlightCode(strings.Join(lines, "\n"), path, nil), "\n")
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
		note := fmt.Sprintf("… %d lines total, showing first %d", total, blockMaxLines)
		out = append(out, colorText(cFgMostSubtle,
			fmt.Sprintf("%*s  %s", numW, "", truncateRunes(note, avail-numW-2))))
	}
	return out
}
