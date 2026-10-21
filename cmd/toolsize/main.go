// toolsize — 从真实 session.db 统计工具结果的 token 尺寸分布，
// 为 compaction Pass-1 预算选择提供实测依据（与生产同一 tiktoken 计数器）。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model/tiktoken"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "./.HackerTeam/session.db", "session.db 路径")
	modelName := flag.String("model", "glm-5.3", "模型名（决定 tiktoken 词表回退，与生产一致）")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath+"?mode=ro")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	counter, err := tiktoken.New(*modelName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tiktoken:", err)
		os.Exit(1)
	}

	rows, err := db.Query(`SELECT event FROM session_events ORDER BY id`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()

	ctx := context.Background()
	var sizes []int
	var toolNames = map[string][]int{} // toolName -> sizes
	var total int
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			continue
		}
		if ev.Response == nil {
			continue
		}
		for _, ch := range ev.Response.Choices {
			m := ch.Message
			if m.ToolID == "" {
				continue
			}
			n, err := counter.CountTokens(ctx, m)
			if err != nil {
				continue
			}
			sizes = append(sizes, n)
			toolNames[m.ToolName] = append(toolNames[m.ToolName], n)
			total++
		}
	}
	if len(sizes) == 0 {
		fmt.Println("未找到工具结果事件")
		return
	}
	sort.Ints(sizes)

	pct := func(p float64) int { return sizes[int(float64(len(sizes)-1)*p)] }
	fmt.Printf("真实会话工具结果尺寸分布（n=%d，tiktoken est）:\n", len(sizes))
	fmt.Printf("  min=%d  p25=%d  p50=%d  p75=%d  p90=%d  p95=%d  p99=%d  max=%d\n",
		sizes[0], pct(.25), pct(.5), pct(.75), pct(.9), pct(.95), pct(.99), sizes[len(sizes)-1])

	fmt.Println("\n预算覆盖表（该预算下完整存活的工具结果比例）:")
	for _, b := range []int{1024, 2048, 4096, 8192, 16384, 32768, 65536} {
		cnt := 0
		tok := 0
		for _, s := range sizes {
			if s <= b {
				cnt++
				tok += s
			}
		}
		fmt.Printf("  budget=%6d: 存活 %5.1f%%  (保留 %d/%d)\n", b, 100*float64(cnt)/float64(len(sizes)), cnt, len(sizes))
	}
	// 占用侧：全部保留 vs 各预算保留的总 token
	sum := 0
	for _, s := range sizes {
		sum += s
	}
	fmt.Printf("\n全部工具结果合计 %d tokens (%.0fK)。会话总事件 token 估算另计。\n", sum, float64(sum)/1000)

	fmt.Println("\n按工具名 top（次数/均值/p90）:")
	type tn struct {
		name string
		n    int
		sum  int
	}
	var tns []tn
	for name, ss := range toolNames {
		sort.Ints(ss)
		tns = append(tns, tn{name, len(ss), sumOf(ss)})
	}
	sort.Slice(tns, func(i, j int) bool { return tns[i].sum > tns[j].sum })
	for i, t := range tns {
		if i >= 12 {
			break
		}
		ss := toolNames[t.name]
		sort.Ints(ss)
		fmt.Printf("  %-20s n=%-3d 均值=%-6d p90=%-6d 合计=%dK\n", t.name, t.n, t.sum/t.n, ss[(len(ss)-1)*9/10], t.sum/1000)
	}
}

func sumOf(s []int) int {
	n := 0
	for _, v := range s {
		n += v
	}
	return n
}
