package engine

// diffMCPtoolsets 的行为验证：同名复用（不 Close、指针原样）、新名字
// 启用新实例、消失/禁用/改名遗留的旧实例关闭。fakeToolset 用来精确
// 观测“哪个实例被关、哪个被复用”——真 ToolSet 是惰性建连的，观测
// 不到生命周期。loadMCPFromConfig 只验证清单构建（真实例惰性，不连）。

import (
	"context"
	"testing"

	"HackerTeam/service/engine/config"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// fakeToolset 记录自己是否被 Close，用于断言“没变的实例绝不被关”。
type fakeToolset struct {
	name   string
	closed bool
}

func (f *fakeToolset) Tools(context.Context) []tool.Tool { return nil }
func (f *fakeToolset) Close() error                      { f.closed = true; return nil }
func (f *fakeToolset) Name() string                      { return f.name }

func fakes(names ...string) []tool.ToolSet {
	out := []tool.ToolSet{}
	for _, n := range names {
		out = append(out, &fakeToolset{name: n})
	}
	return out
}

func namesOf(ts []tool.ToolSet) []string {
	got := []string{}
	for _, t := range ts {
		got = append(got, t.Name())
	}
	return got
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func engineWith(running []tool.ToolSet) *Engine {
	return &Engine{mcpToolsets: running}
}

// 初始加载：空状态 → 清单原样启用。
func TestDiffInitialLoad(t *testing.T) {
	e := engineWith(nil)
	latest := fakes("a", "b")
	e.diffMCPtoolsets(latest)

	if got := namesOf(e.mcpToolsets); !equalStrings(got, []string{"a", "b"}) {
		t.Fatalf("空状态应原样启用清单，得到 %v", got)
	}
	if e.mcpToolsets[0] != latest[0] || e.mcpToolsets[1] != latest[1] {
		t.Fatal("新增条目应启用新造的实例")
	}
}

// 核心契约：同名实例一个都不关、指针原样复用（新造的被丢弃）。
func TestDiffSameNameReusesRunningInstances(t *testing.T) {
	running := fakes("a", "b")
	e := engineWith(running)
	fa := running[0].(*fakeToolset)
	fb := running[1].(*fakeToolset)

	e.diffMCPtoolsets(fakes("a", "b")) //配置没变：新造的同名实例应被丢弃

	if fa.closed || fb.closed {
		t.Fatalf("同名实例不应被 Close: a.closed=%v b.closed=%v", fa.closed, fb.closed)
	}
	if e.mcpToolsets[0] != tool.ToolSet(fa) || e.mcpToolsets[1] != tool.ToolSet(fb) {
		t.Fatal("同名实例应指针原样复用（运行中的实例），而不是换成新造的")
	}
}

// 改名 = 旧实例关闭、新名字用新实例，其余复用。
// 不断言清单顺序：实现允许把新条目排前/排后（功能无关），按名字检索断言契约。
func TestDiffRename(t *testing.T) {
	running := fakes("a", "b")
	e := engineWith(running)
	fa := running[0].(*fakeToolset)
	fb := running[1].(*fakeToolset)

	latest := fakes("b", "c") //a 改名为 c
	e.diffMCPtoolsets(latest)

	if !fa.closed {
		t.Fatal("改名的旧实例（a）应被 Close（stdio 杀子进程）")
	}
	if fb.closed {
		t.Fatal("未受影响的实例（b）不应被 Close")
	}
	if len(e.mcpToolsets) != 2 {
		t.Fatalf("应为 2 个实例，得到 %v", namesOf(e.mcpToolsets))
	}
	byName := map[string]tool.ToolSet{}
	for _, ts := range e.mcpToolsets {
		byName[ts.Name()] = ts
	}
	if byName["b"] != tool.ToolSet(fb) {
		t.Fatal("b 应指针复用（运行中的实例）")
	}
	if byName["c"] != latest[1] {
		t.Fatal("新名字 c 应启用新造的实例")
	}
}

// 消失与禁用同义（禁用条目不会出现在 latest 里）：都走 Close 分支。
func TestDiffRemovalAndDisable(t *testing.T) {
	running := fakes("a", "b")
	e := engineWith(running)
	fa := running[0].(*fakeToolset)
	fb := running[1].(*fakeToolset)

	e.diffMCPtoolsets(nil) //清单清空（a 被删除、b 被禁用都会走到这）

	if !fa.closed || !fb.closed {
		t.Fatalf("消失的实例都应被关闭: a.closed=%v b.closed=%v", fa.closed, fb.closed)
	}
	if len(e.mcpToolsets) != 0 {
		t.Fatal("清单空了应清空 toolsets")
	}
}

// 语义边界的显式记录：同 name 不重启（diff 不看配置内容），
// 配置变更需改名或先禁用再启用才生效。
func TestDiffSameNameMeansReuseOnly(t *testing.T) {
	running := fakes("a")
	e := engineWith(running)
	fa := running[0].(*fakeToolset)

	//配置里 a 的 endpoint 已改，但 loadMCPFromConfig 造出来的还是叫 a
	e.diffMCPtoolsets(fakes("a"))

	if fa.closed || e.mcpToolsets[0] != tool.ToolSet(fa) {
		t.Fatal("精简版按名字匹配：同名一律复用旧实例（这是语义记录，不是缺陷）")
	}
}

// loadMCPFromConfig：只构建 enabled 条目、name 透传、未命名给默认名。
// 新造实例是惰性的，这里不会真正建连。
func TestLoadMCPFromConfig(t *testing.T) {
	e := &Engine{Config_p: &config.Config{
		HttpMcp: []config.HttpMCP{
			{Name: "http1", Enabled: true, Type: config.Streamable_HTTP, Endpoint: "http://x"},
			{Name: "off", Enabled: false, Type: config.Streamable_HTTP, Endpoint: "http://z"},
			{Enabled: true, Type: config.Streamable_HTTP, Endpoint: "http://y"}, //未命名
		},
		StdinMcp: []config.StdinMCP{
			{Name: "stdio1", Enabled: true, Command: "npx", Args: []string{"-y", "mcp"}},
		},
	}}

	latest := e.loadMCPFromConfig()

	if got := namesOf(latest); !equalStrings(got, []string{"http1", "mcp_1", "stdio1"}) {
		t.Fatalf("应构建 http1/mcp_1/stdio1（禁用跳过、未命名给默认名），得到 %v", got)
	}
}
