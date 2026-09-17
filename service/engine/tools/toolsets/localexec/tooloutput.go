package localexec

// 工具输出的「超阈值落盘」策略，以及落盘目录的回收。
//
// 背景：命令输出的体量是不可控的，可能远大于上下文能承受的量。策略是——不超过
// inlineMaxBytes 就原样内联返回；超过则把完整内容写到 tool-outputs 目录，只把头部
// previewBytes 作为预览内联返回，并附上文件路径，让 agent 用 ReadFile 的
// Offset/Bytes 分页取用或用 SearchInFile 检索。既不撑爆上下文，也不丢数据。
//
// 注意：落盘是「成功交付」而不是失败，必须作为正常返回值处理，不要用 error 返回
// ——否则上层 `if err != nil { return nil, err }` 会把整个结果丢掉，agent 连文件
// 路径都拿不到，MCP 层还会把这次调用标记为失败。
//
// 这套机制只有 localexec 用，所以是包内私有、不单独成包。functions/file.go 的
// ReadFile 只需要其中的 rune 边界裁剪，那边自带一份同名的 trimPartialRune。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	// inlineMaxBytes 是内联返回的上限，超过则落盘 + 只给预览。
	inlineMaxBytes = 64 << 10 // 64 KB
	// previewBytes 是落盘后内联返回的预览长度（取头部）。
	previewBytes = 2 << 10 // 2 KB
	// spillDirName 是落盘目录名，位于 <exeDir>/output 下。
	spillDirName = "tool-outputs"
	// spillMaxAge 是落盘文件的保留时长，超过即由 GC 回收。
	spillMaxAge = 24 * time.Hour
	// engineOutputDirName 与 engine 的 outputDir 常量（service/engine/init.go）保持一致。
	engineOutputDirName = "output"
	// spillExt 是落盘文件后缀，GC 只认这个后缀。
	spillExt = ".output"
	// spillTimeLayout 是文件名里创建时间戳的格式，与 cronagent store.go 的
	// .fix<时间戳> 保持一致。秒级精度对 24h 的保留期足够。
	spillTimeLayout = "20060102-150405"
)

// gcOnce 保证整个进程生命周期内只自动回收一次，避免每次落盘都扫目录。
var gcOnce sync.Once

// spillSeq 给落盘文件编号，保证同一个 id 的多次落盘互不覆盖。
var spillSeq atomic.Uint64

// spillDir 返回落盘目录，不存在则创建。
//
// 路径与 engine 保持一致：engine 用 filepath.Dir(os.Executable()) 作为 CWD，
// 输出目录是 <CWD>/output。这里独立解析同样的路径，而不是把 engine 的配置层层
// 传进工具集——LocalExec() 是无参构造，且被 engine 与 cronagent 两处调用，
// 改签名的代价远大于在这里重复一次路径推导。
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

// spill 把 data 完整写入 <dir>/<id>.<创建时间戳>.<seq>.output，返回文件路径。
//
// 文件名在 id 之外还要带时间戳与进程内自增序号：同一个 job 的输出会被反复取用
// （run 结束时落一次，之后 output 查 stdout、查 stderr 又各落一次），只用 id
// 命名会让后一次悄悄覆盖前一次，调用方拿着先前返回的路径读到的是另一条流；
// 并发落盘还会共用同一个 .tmp 互相写坏。时间戳供 GC 当创建时间用（见 gcSpillDir），
// 但只有秒级精度，同一秒内的多次落盘靠 seq 区分；id 是随机十六进制，跨进程也不会撞。
//
// 首次调用时顺带执行一次 GC（进程内只扫一次）。
func spill(id string, data []byte) (string, error) {
	gcOnce.Do(func() { _, _ = gcSpillDir(spillMaxAge) })

	dir, err := spillDir()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.%s.%d%s", id, time.Now().Format(spillTimeLayout), spillSeq.Add(1), spillExt)
	path := filepath.Join(dir, name)
	// 原子写：先写 .tmp 再 rename，避免调用方读到写了一半的文件。
	// 崩溃只会留下无害的 .tmp，由 GC 一并回收。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return path, nil
}

// preview 返回 data 的头部预览，长度不超过 previewBytes。
// 截断点对齐到 rune 边界，避免把多字节字符劈成两半产生乱码（中文输出尤其明显）。
func preview(data []byte) []byte {
	if len(data) <= previewBytes {
		return data
	}
	return trimPartialRune(data[:previewBytes])
}

// trimPartialRune 去掉末尾被字节窗口劈开的半个 UTF-8 字符。
// 被去掉的部分由调用方在下次读取时重新取回，不丢数据。
func trimPartialRune(b []byte) []byte {
	// 从末尾往前找最近的 rune 起点；UTF-8 单字符最长 4 字节，回退 4 次足够。
	for i := len(b) - 1; i >= 0 && i >= len(b)-4; i-- {
		if !utf8.RuneStart(b[i]) {
			continue // 10xxxxxx，是延续字节，继续往前找
		}
		// b[i:] 是一个 rune 的开头；不合法说明这个 rune 被截断了，整段丢弃
		if !utf8.Valid(b[i:]) {
			return b[:i]
		}
		return b
	}
	return b
}

// gcSpillDir 删除落盘目录下创建时间早于 maxAge 的文件，返回删除数量。
//
// 创建时间从文件名里解析，不 stat mtime。落盘文件是 deliver 一次性写完的快照，
// 写完就不再改动，mtime 恒等于创建时间，所以文件名里的时间戳是等价信息——但它
// 自描述：ls 一眼看出新旧，回收时省掉每个文件一次 stat，测试也不用 Chtimes 造假时间。
// 顺带也不受外部工具改动 mtime（备份软件、手工 touch）的影响。
//
// 名字解析不出来的文件一律跳过：宁可漏删不可误删。那可能是别的用途的文件，
// 也可能是命名格式变更之前留下的产物。
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
		name := e.Name()
		// .tmp 是原子写崩溃留下的残留，与 .output 同一套命名，一并回收
		stem := strings.TrimSuffix(name, ".tmp")
		if filepath.Ext(stem) != spillExt {
			continue
		}
		created, ok := spillCreatedAt(stem)
		if !ok || created.After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			n++
		}
	}
	return n, nil
}

// spillCreatedAt 从 <id>.<时间戳>.<seq>.output 里解析创建时间。
// 时间戳取倒数第二段：id 目前是不含点的十六进制，但从右往左数不依赖 id 的形状。
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
