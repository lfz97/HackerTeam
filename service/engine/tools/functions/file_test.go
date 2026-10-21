package functionTools

// 模型侧体量预算的回归测试：结果经 JSON 序列化后会被放大，ReadFile 的窗口与
// SearchInFile 的匹配列表都按 modeltext.Size 度量设防。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"HackerTeam/service/engine/tools/modeltext"
)

// inlineJSONSize 返回工具结果经 JSON 序列化后的大小，即模型实际收到的字节数。
func inlineJSONSize(t *testing.T, res map[string]string) int {
	t.Helper()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return len(b)
}

// 请求超大窗口时页体量按序列化后大小收缩到预算内；nextOffset 保持分页语义，数据不丢。
func TestReadFileShrinksWindowToBudget(t *testing.T) {
	dir := t.TempDir()

	// 64KB 的 'a'：序列化 1:1，64KB 窗口恰好超限 2 字节（引号），减半到 32KB。
	text := make([]byte, modeltext.MaxInlineBytes)
	for i := range text {
		text[i] = 'a'
	}
	textPath := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(textPath, text, 0644); err != nil {
		t.Fatal(err)
	}
	res, err := ReadFile(context.Background(), readFileRequest{Path: textPath, Bytes: 1 << 20})
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if size := inlineJSONSize(t, res); size > modeltext.MaxInlineBytes+64 {
		t.Fatalf("text page serialized to %d bytes, over budget %d", size, modeltext.MaxInlineBytes)
	}
	if res["length"] == strconv.Itoa(modeltext.MaxInlineBytes) {
		t.Fatal("window should have been shrunk for the over-budget page")
	}

	// 分页语义：从 nextOffset 连续续读（trim 掉的截断字节下一页补回）直至 EOF，
	// 每一页都在预算内、页偏移严格前进。
	binPath := filepath.Join(dir, "bin.dat")
	bin := make([]byte, 16<<10)
	for i := range bin {
		bin[i] = 0xFF
	}
	if err := os.WriteFile(binPath, bin, 0644); err != nil {
		t.Fatal(err)
	}
	offset := 0
	prevNext := -1
	for page := 0; ; page++ {
		if page > 10 {
			t.Fatal("pagination did not reach EOF within 10 pages")
		}
		res2, err := ReadFile(context.Background(), readFileRequest{Path: binPath, Bytes: modeltext.MaxInlineBytes, Offset: offset})
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if size := inlineJSONSize(t, res2); size > modeltext.MaxInlineBytes+64 {
			t.Fatalf("binary page serialized to %d bytes, over budget %d", size, modeltext.MaxInlineBytes)
		}
		if n, _ := strconv.Atoi(res2["length"]); n >= modeltext.MaxInlineBytes {
			t.Fatalf("binary page should be far smaller than the window, got %d bytes", n)
		}
		next, _ := strconv.Atoi(res2["nextOffset"])
		if next <= prevNext {
			t.Fatalf("nextOffset did not advance: %d after %d", next, prevNext)
		}
		prevNext = next
		if res2["eof"] == "true" {
			if next != len(bin) {
				t.Fatalf("EOF at nextOffset %d, want %d", next, len(bin))
			}
			break
		}
		offset = next
	}
}

// 匹配列表超预算时截断，并附 truncated/totalMatches/note。
func TestSearchInFileTruncatesToBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "many.txt")
	// 2000 行 "match"：每条匹配序列化后 ~60 字节，总量远超 64KB 预算；
	// 首行放一个唯一的 token 供「少量匹配」用例使用。
	content := "unique first line\n"
	for i := 0; i < 2000; i++ {
		content += "match something\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := SearchInFile(context.Background(), searchInFileRequest{Path: path, Regex: "match"})
	if err != nil {
		t.Fatalf("SearchInFile: %v", err)
	}
	if size := inlineJSONSize(t, res); size > modeltext.MaxInlineBytes+4096 {
		t.Fatalf("search result serialized to %d bytes, over budget %d", size, modeltext.MaxInlineBytes)
	}
	if res["truncated"] != "true" {
		t.Fatal("expected truncated = true")
	}
	if total, _ := strconv.Atoi(res["totalMatches"]); total != 2000 {
		t.Fatalf("totalMatches = %q, want 2000", res["totalMatches"])
	}

	// 少量匹配不受影响：无 truncated 标记。
	res2, err := SearchInFile(context.Background(), searchInFileRequest{Path: path, Regex: "unique"})
	if err != nil {
		t.Fatalf("SearchInFile: %v", err)
	}
	if _, ok := res2["truncated"]; ok {
		t.Fatal("small result should not be truncated")
	}
}
