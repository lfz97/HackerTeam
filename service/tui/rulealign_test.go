package tui

// 表格规则线对齐回归测试：glamour 排版宽度必须扣掉 msgIndent 的 2 列，
// 否则整行超 viewport 被 wrapViewText 硬断时，无空格的行（表格规则线）
// 触发 reflow 硬断丢行首缩进——规则线 ┼ 与正文行 │ 错位 2 列。
// （历史 bug：renderBody 按 w 排版再叠 msgIndent，超宽 2 列。）

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ruleMisalignMD 现场抓取的触发用例：三列 + ✅ + 中英混合长内容 + 折行单元格。
const ruleMisalignMD = `| 检查项 | 状态 | 结果 |
|---|---|---|
| 操作系统 Operating System | ✅ | 检测到 Linux 环境运行正常，主机名 hostname 为 LIFENGZHETB，默认 shell 为 bash/sh，命令选择策略按 Debian/RHEL 系规则执行 |
| CPU 架构 Architecture | ✅ | CPU 架构确认为 amd64 (x86_64)，与 Linux 二进制发行版兼容 |
| 当前目录 Working Directory | ✅ | 当前工作目录为 /tmp/hbtest，目录存在且可访问，任务基础路径有效 |
| 配置目录 Config Directory | ✅ | 配置目录 /tmp/hbtest/.hyperbot 加载完成，含核心配置、skills 技能目录及运行日志 |`

func TestTableRuleAlignment(t *testing.T) {
	// 生产路径：renderBody（含 msgIndent 预算），applyViewText 不再改写
	final := renderBody(ruleMisalignMD, 116)

	var pos [][]int
	for i, ln := range strings.Split(final, "\n") {
		s := ansi.Strip(ln)
		var cols []int
		w := 0
		for _, r := range s {
			if r == '│' || r == '┼' {
				cols = append(cols, w)
			}
			w += ansi.StringWidth(string(r))
		}
		if len(cols) > 0 {
			pos = append(pos, cols)
			_ = i
		}
	}
	if len(pos) < 3 {
		t.Fatalf("表格未渲染出规则线与数据行: %d 行", len(pos))
	}
	for i, p := range pos {
		if p[0] != pos[0][0] || p[len(p)-1] != pos[0][len(pos[0])-1] {
			t.Errorf("line %d 分隔符 %v 与表头 %v 不对齐（规则线/正文错位）", i, p, pos[0])
		}
	}
	// 行宽不越界
	for i, ln := range strings.Split(final, "\n") {
		if w := ansi.StringWidth(ln); w > 116 {
			t.Errorf("line %d 越界（宽度 %d）", i, w)
		}
	}
}
