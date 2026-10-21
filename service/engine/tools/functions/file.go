package functionTools

import (
	"HackerTeam/service/engine/tools/modeltext"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pmezard/go-difflib/difflib"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func WriteFile(ctx context.Context, req struct {
	Path    string `json:"path" jsonschema:"description=要写入的文件路径。"`
	Content string `json:"content" jsonschema:"description=写入的文件内容"`
	Append  bool   `json:"append" jsonschema:"description=是否启用追加模式，默认为false即全文覆盖，如果为true则在文件末尾追加写入。"`
}) (map[string]string, error) {

	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	var fd *os.File
	var err error
	if req.Append {
		fd, err = os.OpenFile(req.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
	} else {
		fd, err = os.OpenFile(req.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
	}

	length, err := fd.WriteString(req.Content)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"wrote":   strconv.Itoa(length),
		"content": req.Content,
	}, nil
}

type readFileRequest struct {
	Path   string `json:"path" jsonschema:"description=要读取的文件路径。"`
	Bytes  int    `json:"bytes" jsonschema:"description=读取文件的窗口大小，单位为字节。默认为1024字节。"`
	Offset int    `json:"offset" jsonschema:"description=读取文件的偏移位置，单位为字节。默认为0，即从文件开头开始读取。"`
}

func ReadFile(ctx context.Context, req readFileRequest) (map[string]string, error) {

	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Bytes < 0 {
		return nil, errors.New("`bytes` must >= 0")
	}
	if req.Offset < 0 {
		return nil, errors.New("`offset` must >= 0")
	}
	if req.Bytes == 0 {
		req.Bytes = 1024
	}
	fd, err := os.OpenFile(req.Path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	fi, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	total := fi.Size()
	// bytes 来自模型，设上限防止超大请求直接分配超大缓冲区；之后还会按序列化大小收缩。
	window := req.Bytes
	if window > modeltext.MaxInlineBytes {
		window = modeltext.MaxInlineBytes
	}
	var content []byte
	for {
		// 错误必须检查：Offset 非法时指针会停在 0，静默读回文件开头；窗口收缩重读后也要重新 Seek。
		if _, err := fd.Seek(int64(req.Offset), io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek to offset %d: %w", req.Offset, err)
		}
		buf := make([]byte, window)
		// Read 不保证填满缓冲区（short read 合法），文件被后台写入时会误判成 EOF；ReadFull 保证读满。EOF/ErrUnexpectedEOF 属正常。
		n, err := io.ReadFull(fd, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		// 按实际读取长度截取并对齐 rune 边界，去掉被窗口劈开的半个多字节字符；
		// 被去掉的半个字符下次从 nextOffset 读取时会重新取回，不丢数据。
		content = modeltext.TrimPartialRune(buf[:n])
		// 整个窗口都是二进制时不能再 trim：trim 到空会卡在同一 offset 无限重读，原样返回，进度优先。
		if len(content) == 0 && n > 0 {
			content = buf[:n]
		}
		// 一页的体量按 JSON 序列化后的大小设防（非法字符序列化后会成倍膨胀，见 modeltext）：
		// 超限就窗口减半重读，nextOffset 分页语义不变；减到 1 字节时必不超限，循环必终止。
		if window == 1 || modeltext.Size(string(content)) <= modeltext.MaxInlineBytes {
			break
		}
		window /= 2
	}
	next := req.Offset + len(content)
	return map[string]string{
		"length":     strconv.Itoa(len(content)),
		"nextOffset": strconv.Itoa(next), //分页续读时直接把这个值传给 offset，无需自己计算
		"totalSize":  strconv.Itoa(int(total)),
		"eof":        strconv.FormatBool(int64(next) >= total),
		"content":    string(content),
	}, nil
}

// EditFile：编辑指定文件中的内容，支持替换指定的旧内容为新内容。默认仅允许唯一匹配时替换（多处匹配会报错），设置replace_all为true则全量替换。
func EditFile(ctx context.Context, req struct {
	Path       string `json:"path" jsonschema:"description=要编辑的文件路径。"`
	Old        string `json:"old" jsonschema:"description=要替换的旧内容。"`
	New        string `json:"new" jsonschema:"description=要替换的新内容。"`
	ReplaceAll bool   `json:"replace_all" jsonschema:"description=是否替换文件中所有匹配的字符串，默认为false仅允许唯一匹配（多处匹配会报错）。设置为true则全量替换。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Old == "" {
		return nil, errors.New("`Old` cannot be empty")
	}
	oldBytes, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, err
	}
	oldContent := string(oldBytes)

	Indexes := []int{}
	offset := 0
	for {
		idx := strings.Index(oldContent[offset:], req.Old)
		if idx == -1 {
			break
		}
		offset += idx
		Indexes = append(Indexes, offset)
		offset += len(req.Old)
	}
	if len(Indexes) == 0 {
		return nil, errors.New("oldContent not found in file")
	}
	if !req.ReplaceAll && len(Indexes) > 1 {
		return nil, errors.New("multiple matches found for oldContent, but replace_all is set to false")
	}

	newContent := strings.ReplaceAll(oldContent, req.Old, req.New)
	err = os.WriteFile(req.Path, []byte(newContent), 0644)
	if err != nil {
		return nil, err
	}

	// 生成 unified diff，方便 LLM 验证编辑结果
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: req.Path,
		ToFile:   req.Path,
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"diff": text,
	}, nil
}

type matchInfo struct {
	StartlineNum int    `json:"start_line_num"`
	EndlineNum   int    `json:"end_line_num"`
	MatchContent string `json:"match_content"`
}

// searchInFileRequest 是 Grep 工具的请求；正则约束（RE2 语法等）见工具的 WithDescription。
type searchInFileRequest struct {
	Path  string `json:"path" jsonschema:"description=要搜索的文件路径。"`
	Regex string `json:"regex" jsonschema:"description=要搜索的正则表达式。"`
}

func SearchInFile(ctx context.Context, req searchInFileRequest) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Regex == "" {
		return nil, errors.New("`Regex` cannot be empty")
	}
	re_p, err := regexp.Compile(req.Regex) //不用MustCompile：失败时Panic，Compile返回error
	if err != nil {
		return nil, errors.New("invalid regex pattern")
	}
	contentBytes, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	matches := re_p.FindAllIndex(contentBytes, -1)
	// 匹配列表按 inline 预算设防：防止宽松正则把整个文件（序列化后放大数倍）塞进模型上下文，超限即截断并告知总数。
	index := 0
	total := 0
	truncated := false
	for _, match := range matches {
		startOffset := match[0]
		endOffset := match[1]

		startlineNum := strings.Count(string(contentBytes[:startOffset]), "\n") + 1
		endlineNum := strings.Count(string(contentBytes[:endOffset]), "\n") + 1

		matchContent_b := contentBytes[startOffset:endOffset]
		info := matchInfo{
			StartlineNum: startlineNum,
			EndlineNum:   endlineNum,
			MatchContent: string(matchContent_b),
		}
		infoBytes, err := json.Marshal(info)
		if err != nil {
			return nil, err
		}
		// 条目开销按最终序列化形式度量（值 + `"N":` 键 + 逗号），预算按完整开销累计，保证结果不超限。
		cost := modeltext.Size(strconv.Itoa(index)) + 2 + modeltext.Size(string(infoBytes))
		if total+cost > modeltext.MaxInlineBytes {
			truncated = true
			break
		}
		m[strconv.Itoa(index)] = string(infoBytes)
		total += cost
		index++
	}
	if truncated {
		m["truncated"] = "true"
		m["totalMatches"] = strconv.Itoa(len(matches))
		m["note"] = fmt.Sprintf(
			"Result truncated at the %d-byte inline budget after %d matches; %d matches in total. "+
				"Narrow the regex or read the file with `read` instead.",
			modeltext.MaxInlineBytes, index, len(matches))
	}
	return m, nil
}

func DeleteFile(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description=要删除的文件路径。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	err := os.RemoveAll(req.Path)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"deleted": req.Path,
	}, nil
}

func FileInfo(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description=要获取信息的文件路径。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	info, err := os.Stat(req.Path)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"name":     info.Name(),
		"size":     strconv.FormatInt(info.Size(), 10),
		"is_dir":   strconv.FormatBool(info.IsDir()),
		"mode":     info.Mode().String(),
		"mod_time": info.ModTime().String(),
	}, nil
}

func Diff(ctx context.Context, req struct {
	Src string `json:"src" jsonschema:"description=要比较的第一个文件路径。"`
	Dst string `json:"dst" jsonschema:"description=要比较的第二个文件路径。"`
}) (map[string]string, error) {
	if req.Src == "" || req.Dst == "" {
		return nil, errors.New("`PathA` and `PathB` cannot be empty")
	}
	if req.Src == req.Dst {
		return map[string]string{
			"message": "The two paths are the same, no differences.",
		}, nil
	}
	fileA_bytes, err := os.ReadFile(req.Src)
	if err != nil {
		return nil, err
	}
	fileB_bytes, err := os.ReadFile(req.Dst)
	if err != nil {
		return nil, err
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(fileA_bytes)),
		B:        difflib.SplitLines(string(fileB_bytes)),
		FromFile: filepath.Base(req.Src),
		ToFile:   filepath.Base(req.Dst),
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"diff": text,
	}, nil
}

// 获取文件操作工具集合（write/read/edit/Grep/delete/stat/diff）。
const (
	writeFileToolName  string = "write"
	readFileToolName   string = "read"
	editFileToolName   string = "edit"
	grepToolName       string = "Grep"
	deleteFileToolName string = "delete"
	fileStatToolName   string = "stat"
	diffToolName       string = "diff"
)

func GetFileOperationsTools() []tool.Tool {
	wftool := function.NewFunctionTool(
		WriteFile,
		function.WithName(writeFileToolName),
		function.WithDescription("将内容写入指定文件，如果文件不存在则创建，已存在则覆盖。"),
	)
	rftool := function.NewFunctionTool(
		ReadFile,
		function.WithName(readFileToolName),
		function.WithDescription("从指定文件读取内容，支持设置读取窗口大小和偏移量。页大小受内联预算约束：请求的窗口经 JSON 序列化后超预算（如二进制或控制字符）时窗口会收缩，返回的一页比请求的小，用 nextOffset 续读。"),
	)
	eftool := function.NewFunctionTool(
		EditFile,
		function.WithName(editFileToolName),
		function.WithDescription("编辑指定文件中的内容，支持替换指定的旧内容为新内容。默认仅允许唯一匹配时替换（多处匹配会报错），设置replace_all为true则全量替换。"),
	)
	sftool := function.NewFunctionTool(
		SearchInFile,
		function.WithName(grepToolName),
		function.WithDescription("通过正则表达式在指定文件中搜索内容，返回所有匹配项的行号和内容。使用Go RE2语法，不支持lookahead/lookbehind/backreference。`.`默认不匹配换行，跨行匹配用`(?s)`。`^`和`$`默认匹配文本首尾，匹配行首行尾用`(?m)`。结果受内联预算约束：截断时 truncated 为 true，totalMatches 报总匹配数。"),
	)
	dftool := function.NewFunctionTool(
		DeleteFile,
		function.WithName(deleteFileToolName),
		function.WithDescription("删除指定文件或目录，目录会被递归删除，请谨慎使用。"),
	)
	fitool := function.NewFunctionTool(
		FileInfo,
		function.WithName(fileStatToolName),
		function.WithDescription("获取指定文件或目录的信息，包括名称、大小、是否为目录、权限模式和修改时间等。"),
	)
	difftool := function.NewFunctionTool(
		Diff,
		function.WithName(diffToolName),
		function.WithDescription("比较两个文件的差异，返回unified diff格式的结果。"),
	)
	return []tool.Tool{wftool, rftool, eftool, sftool, dftool, fitool, difftool}
}
