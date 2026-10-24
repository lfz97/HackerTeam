package tui

// 斜杠命令注册表、分发与补全浮层的行为测试。
//
// 浮层的显隐是输入框内容的派生结果（无独立开关、无打开快捷键），这条不变量
// 曾经被破坏过——按键没转发给 textarea 就用旧值匹配、浮层开着时的分支直接
// return nil 吞掉按键，两者叠加的表现是"敲 / 弹全量、继续打字清单不更新"。
// 下面的用例把它钉住：清单必须随输入实时重算，且失去斜杠前缀自动收起。

import (
	"testing"
)

// recordingView 在 fakeView（composebench_test.go）之上加调用记录，用来断言
// dispatchSlash 确实把命令送到了对应的引擎能力接口、普通输入确实走了 submitInput。
type recordingView struct {
	fakeView
	shutdown, newConv, clear int
	submitted                string
}

func (r *recordingView) Shutdown()             { r.shutdown++ }
func (r *recordingView) NewConversation() bool { r.newConv++; return true }
func (r *recordingView) ClearRecords() bool    { r.clear++; return true }
func (r *recordingView) SubmitInput(line string) bool {
	r.submitted = line
	return true
}

// newSlashTestTUI 组一个够跑按键路径的最小 TUI：width/height 留 0，
// recalcComponentSize 会在开头早退，因此不需要 bottom/spinner 等组件。
func newSlashTestTUI() (*TUI, *recordingView) {
	e := &recordingView{}
	ta, _ := NewPrettyTextArea(0, 1, 0)
	return &TUI{
		engine:     e,
		ta:         ta,
		vp:         NewPrettyViewport(0, 1),
		slashPopup: newSlashPopup(),
		resume:     newResumeList(),
		expand:     newExpandState(),
	}, e
}

func TestSlashToken(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/", true}, {"/re", true}, {"/resume", true},
		{"", false}, {"re", false}, {"hello", false}, {"a/b", false},
		//含空白 = 用户已经在写命令参数（或本就是普通消息），补全没有意义
		{"/resume ", false}, {"/re sume", false}, {"/resume x", false},
	}
	for _, c := range cases {
		if _, ok := slashToken(c.in); ok != c.want {
			t.Errorf("slashToken(%q) ok=%v, want %v", c.in, ok, c.want)
		}
	}
}

func TestSlashPopupVisibilityIsDerived(t *testing.T) {
	tt, _ := newSlashTestTUI()

	cases := []struct {
		input       string
		wantVisible bool
		wantCount   int
		wantFirst   string
	}{
		{"/", true, len(slashRegistry), slashRegistry[0].cmd}, //单敲 / = 全量清单
		{"/re", true, 1, "/resume"},                           //前缀过滤
		{"/n", true, 1, "/new"},
		{"/c", true, 1, "/clear"},
		{"/e", true, 1, "/exit"},
		{"/zzz", false, 0, ""},     //无匹配 → 收起
		{"hello", false, 0, ""},    //普通聊天 → 收起（且不触发过滤）
		{"/resume ", false, 0, ""}, //已在写参数 → 收起
		{"", false, 0, ""},
	}
	for _, c := range cases {
		tt.ta.SetValue(c.input)
		tt.refilterSlashItems()
		if got := tt.slashPopup.isVisible(); got != c.wantVisible {
			t.Errorf("input=%q visible=%v, want %v", c.input, got, c.wantVisible)
			continue
		}
		if !c.wantVisible {
			continue
		}
		if n := len(tt.slashPopup.items); n != c.wantCount {
			t.Errorf("input=%q items=%d, want %d", c.input, n, c.wantCount)
		}
		if sel, ok := tt.slashPopup.selectedCmd(); !ok || sel != c.wantFirst {
			t.Errorf("input=%q selected=%q (ok=%v), want %q", c.input, sel, ok, c.wantFirst)
		}
	}
}

// 清单每次重算都必须把选中项归零，否则上一次的选择会残留在新清单上
// （新清单更短时还会让 selectedCmd 越界返回 ok=false，enter 变成空操作）。
func TestSlashPopupSetItemsResetsIndex(t *testing.T) {
	tt, _ := newSlashTestTUI()
	tt.ta.SetValue("/")
	tt.refilterSlashItems()
	tt.slashPopup.moveDown()
	tt.slashPopup.moveDown()
	if tt.slashPopup.index != 2 {
		t.Fatalf("index=%d, want 2", tt.slashPopup.index)
	}
	tt.ta.SetValue("/e")
	tt.refilterSlashItems()
	if tt.slashPopup.index != 0 {
		t.Errorf("index after refilter=%d, want 0", tt.slashPopup.index)
	}
	if sel, _ := tt.slashPopup.selectedCmd(); sel != "/exit" {
		t.Errorf("selected=%q, want /exit", sel)
	}
}

// moveUp/moveDown 的取模除数是 len(items)、selectedCmd 是裸索引：
// 空清单必须早退，否则 integer divide by zero / index out of range。
func TestSlashPopupEmptyListGuards(t *testing.T) {
	s := newSlashPopup()
	s.moveUp()
	s.moveDown()
	if _, ok := s.selectedCmd(); ok {
		t.Error("selectedCmd on empty popup should be ok=false")
	}
	s.setItems(nil)
	s.moveUp()
	s.moveDown()
	if _, ok := s.selectedCmd(); ok {
		t.Error("selectedCmd after setItems(nil) should be ok=false")
	}
}

func TestDispatchSlash(t *testing.T) {
	tt, e := newSlashTestTUI()

	if cmd, handled := tt.dispatchSlash("/new"); !handled || cmd != nil {
		t.Errorf("/new handled=%v cmd=%v, want true/nil", handled, cmd)
	}
	if e.newConv != 1 {
		t.Errorf("NewConversation calls=%d, want 1", e.newConv)
	}

	if cmd, handled := tt.dispatchSlash("/clear"); !handled || cmd != nil {
		t.Errorf("/clear handled=%v cmd=%v, want true/nil", handled, cmd)
	}
	if e.clear != 1 {
		t.Errorf("ClearRecords calls=%d, want 1", e.clear)
	}

	///resume 无候选（fakeView 的 ListResumeItems 返回 "[]"）时不开弹框
	if cmd, handled := tt.dispatchSlash("/resume"); !handled || cmd != nil {
		t.Errorf("/resume handled=%v cmd=%v, want true/nil", handled, cmd)
	}
	if tt.resume.isVisible() {
		t.Error("/resume with empty candidate list should not open the dialog")
	}

	if cmd, handled := tt.dispatchSlash("/exit"); !handled || cmd == nil {
		t.Errorf("/exit handled=%v cmd=%v, want true/non-nil(tea.Quit)", handled, cmd)
	}
	if e.shutdown != 1 {
		t.Errorf("Shutdown calls=%d, want 1", e.shutdown)
	}

	//未登记的输入必须 handled=false，由调用方按普通输入兜底提交给引擎。
	//绝不能因为"以 / 开头"就当命令吞掉。
	for _, in := range []string{"/bogus", "/resumeX", "/new2", "hello", "/", ""} {
		if _, handled := tt.dispatchSlash(in); handled {
			t.Errorf("dispatchSlash(%q) handled=true, want false", in)
		}
	}
}

// 一张表两种用途的核心不变量：浮层里能选到的每一条，dispatchSlash 都必须认，
// 且都挂着可执行的 run。将来若有人把清单与分发拆回两处维护，这条会先失败。
func TestSlashRegistryCoversDispatch(t *testing.T) {
	tt, _ := newSlashTestTUI()
	seen := map[string]bool{}
	for _, sc := range slashRegistry {
		if seen[sc.cmd] {
			t.Errorf("duplicate registry entry %q", sc.cmd)
		}
		seen[sc.cmd] = true
		if sc.run == nil {
			t.Errorf("registry entry %q has nil run", sc.cmd)
		}
		if sc.desc == "" {
			t.Errorf("registry entry %q has empty desc（浮层会是空白行）", sc.cmd)
		}
		if _, handled := tt.dispatchSlash(sc.cmd); !handled {
			t.Errorf("registry entry %q not handled by dispatchSlash", sc.cmd)
		}
	}
	if len(seen) == 0 {
		t.Fatal("slashRegistry is empty — 浮层将永远打不开")
	}
}
