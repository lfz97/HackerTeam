package engine

import (
	stdlog "log"
	"os"
	"path/filepath"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/log"
)

// 日志文件打不开（只读目录 / ENOTDIR）时必须降级为黑洞而非静默放弃：
// 静默放弃会让框架日志回落到 stdout/stderr，污染 TUI 终端
// （复现：exe 放在 C:\Program Files (x86) 下，OpenFile 拒绝访问）。
func TestRedirectFrameworkLogFallsBackToDiscard(t *testing.T) {
	oldDefault, oldContext, oldStd := log.Default, log.ContextDefault, stdlog.Writer()
	defer func() {
		log.Default, log.ContextDefault = oldDefault, oldContext
		stdlog.SetOutput(oldStd)
	}()

	// 父路径是普通文件 → OpenFile 必失败（ENOTDIR）。
	// 不用"只读目录"构造：root 下权限拒绝不可复现，ENOTDIR 必现。
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &BootEnv{ConfigFolderPath: parent}
	env.redirectFrameworkLog()

	if env.LogFile != nil {
		t.Fatalf("LogFile 应保持 nil（打开失败时无句柄可持有），got %v", env.LogFile)
	}
	if len(env.Notices) != 1 || env.Notices[0].Kind != NoticeWarning {
		t.Fatalf("打开失败应推送 1 条 warning notice，got %+v", env.Notices)
	}
	// 标准库 log 的默认输出口是 os.Stderr：仍是它 = 走了旧的静默 return 路径
	if stdlog.Writer() == os.Stderr {
		t.Fatal("标准库 log 未被重定向（仍指向 os.Stderr）：静默放弃路径复活了")
	}
	if log.Default == nil || log.ContextDefault == nil {
		t.Fatal("框架 logger 被置空：丢弃路径也必须给出可用的 logger")
	}
}
