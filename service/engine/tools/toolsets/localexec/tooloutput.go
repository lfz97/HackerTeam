package localexec

// 工具输出超过内联预算时整体落盘到 tool-outputs 目录，只内联头部预览并附上
// 文件路径，由 agent 用 ReadFile/SearchInFile 分页取用。落盘是成功交付，
// 必须走正常返回值——用 error 返回会让上层丢掉整个结果。
// 预算与序列化度量来自 modeltext 包（ReadFile、SearchInFile 共用同一套）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"HackerTeam/service/engine/tools/modeltext"
)

const (
	inlineMaxBytes = modeltext.MaxInlineBytes
	previewBytes   = 2 << 10 // 2 KB
	spillDirName   = "tool-outputs"
	spillMaxAge    = 24 * time.Hour
	// 与 engine 的 outputDir 常量（service/engine/init.go）保持一致。
	engineOutputDirName = "output"
	spillExt            = ".output" // GC 只认这个后缀
	spillTimeLayout     = "20060102-150405"
)

// gcOnce 进程内只自动 GC 一次，避免每次落盘都扫目录。
var gcOnce sync.Once

var spillSeq atomic.Uint64

// spillDir 返回落盘目录，不存在则创建。与 engine 的输出目录保持同一推导，
// 但独立解析：LocalExec() 是无参构造，不值得为此层层传配置。
func spillDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}
	dir := filepath.Join(filepath.Dir(exePath), engineOutputDirName, spillDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create spill directory %s: %w", dir, err)
	}
	return dir, nil
}

// spill 把 data 完整写入落盘目录，返回文件路径。文件名 = id.时间戳.序号：
// 同一个 id 会落盘多次（run 结束、查 stdout、查 stderr），只按 id 命名会互相
// 覆盖；时间戳供 GC 判断新旧，秒级以内的多次落盘靠序号区分。
// 首次调用顺带 GC 一次。
func spill(id string, data []byte) (string, error) {
	gcOnce.Do(func() { _, _ = gcSpillDir(spillMaxAge) })

	dir, err := spillDir()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.%s.%d%s", id, time.Now().Format(spillTimeLayout), spillSeq.Add(1), spillExt)
	path := filepath.Join(dir, name)
	// 原子写：崩溃只会留下无害的 .tmp，由 GC 一并回收。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return path, nil
}

// preview 返回 data 的头部预览，截断点对齐到 rune 边界，避免劈开多字节字符。
func preview(data []byte) []byte {
	if len(data) <= previewBytes {
		return data
	}
	return modeltext.TrimPartialRune(data[:previewBytes])
}

// gcSpillDir 删除创建时间早于 maxAge 的落盘文件，返回删除数量。
// 时间从文件名解析而非 stat mtime：ls 一眼可辨新旧，也免受外部工具改 mtime
// 的影响。名字解析不出来的文件跳过——宁可漏删不可误删。
func gcSpillDir(maxAge time.Duration) (int, error) {
	dir, err := spillDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", dir, err)
	}
	cutoff := time.Now().Add(-maxAge)
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// .tmp 是原子写崩溃的残留，与 .output 一并回收
		stem := strings.TrimSuffix(e.Name(), ".tmp")
		if filepath.Ext(stem) != spillExt {
			continue
		}
		created, ok := spillCreatedAt(stem)
		if !ok || created.After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			n++
		}
	}
	return n, nil
}

// spillCreatedAt 从 <id>.<时间戳>.<seq>.output 解析创建时间。
// 时间戳取倒数第二段，不依赖 id 的形状。
func spillCreatedAt(stem string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSuffix(stem, spillExt), ".")
	if len(parts) < 3 {
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation(spillTimeLayout, parts[len(parts)-2], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}
