package functionTools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func WriteFile(ctx context.Context, req struct {
	Path    string `json:"path" jsonschema:"description:要写入的文件路径。"`
	Content string `json:"content" jsonschema:"description:写入的文件内容"`
	Append  bool   `json:"append" jsonschema:"description:是否启用追加模式，默认为false即全文覆盖，如果为true则在文件末尾追加写入。"`
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
		"WritePath":   req.Path,
		"WriteLength": strconv.Itoa(length),
		"message":     "Success",
	}, nil
}

func ReadFile(ctx context.Context, req struct {
	Path   string `json:"path" jsonschema:"description:要读取的文件路径。"`
	Bytes  int    `json:"bytes" jsonschema:"description:读取文件的窗口大小，单位为字节。默认为1024字节。"`
	Offset int    `json:"offset" jsonschema:"description:读取文件的偏移位置，单位为字节。默认为0，即从文件开头开始读取。"`
}) (map[string]string, error) {

	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Bytes == 0 {
		req.Bytes = 1024
	}
	fd, err := os.OpenFile(req.Path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	fd.Seek(int64(req.Offset), io.SeekStart) //根据请求的偏移位置调整文件指针位置，默认为0即从文件开头开始读取
	buf := make([]byte, req.Bytes)           //根据请求的窗口大小创建缓冲区
	n, err := fd.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	content := buf[:n] //按实际读取读取的内容长度截取缓冲区，否则如果读取的内容长度小于窗口大小，返回的内容会包含多余的空字节填充
	return map[string]string{
		"ReadPath":   req.Path,
		"ReadLength": strconv.Itoa(len(content)),
		"Content":    string(content),
	}, nil
}

// EditFile：编辑指定文件中的内容，支持替换指定的旧内容为新内容。注意：会替换文件中所有匹配的字符串，非仅第一处。
func EditFile(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description:要编辑的文件路径。"`
	Old  string `json:"old" jsonschema:"description:要替换的旧内容。"`
	New  string `json:"new" jsonschema:"description:要替换的新内容。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Old == "" {
		return nil, errors.New("`Old` cannot be empty")
	}
	contentBytes, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, err
	}
	Now := string(contentBytes)
	if !strings.Contains(Now, req.Old) {
		return nil, errors.New("oldContent not found in file")
	} else {
		err = os.WriteFile(req.Path, []byte(strings.ReplaceAll(Now, req.Old, req.New)), 0644)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"EditPath": req.Path,
			"message":  "Success",
		}, nil

	}
}

type matchInfo struct {
	StartlineNum int    `json:"startLineNum"`
	EndlineNum   int    `json:"endLineNum"`
	MatchContent string `json:"matchContent"`
}

// 通过正则表达式在指定文件中搜索内容，返回所有匹配项的行号和内容。使用Go RE2语法，不支持lookahead/lookbehind/backreference。`.`默认不匹配换行，跨行匹配用`(?s)`。`^`和`$`默认匹配文本首尾，匹配行首行尾用`(?m)`。
func SearchInFile(ctx context.Context, req struct {
	Path  string `json:"path" jsonschema:"description:要搜索的文件路径。"`
	Regex string `json:"regex" jsonschema:"description:要搜索的正则表达式。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Regex == "" {
		return nil, errors.New("`Regex` cannot be empty")
	}
	re_p, err := regexp.Compile(req.Regex) //不使用MustCompile，因为MustCompile失败时会直接Panic，Compile是返回error
	if err != nil {
		return nil, errors.New("invalid regex pattern")
	}
	contentBytes, err := os.ReadFile(req.Path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	matches := re_p.FindAllIndex(contentBytes, -1)
	index := 0
	for _, match := range matches {
		startOffset := match[0]
		endOffset := match[1]

		// \n出现的次数加1即为行号
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
		m[strconv.Itoa(index)] = string(infoBytes)
		index++

	}
	return m, nil
}

func DeleteFile(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description:要删除的文件路径。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	err := os.RemoveAll(req.Path)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"DeletePath": req.Path,
		"message":    "Success",
	}, nil
}

func FileInfo(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description:要获取信息的文件路径。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	info, err := os.Stat(req.Path)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Name":     info.Name(),
		"Size":     strconv.FormatInt(info.Size(), 10),
		"IsDir":    strconv.FormatBool(info.IsDir()),
		"Mode":     info.Mode().String(),
		"ModeTime": info.ModTime().String(),
	}, nil
}

// 获取文件操作工具集合：
// WriteFile：将内容写入指定文件，如果文件不存在则创建，已存在则覆盖。
// ReadFile：从指定文件读取内容，支持设置读取窗口大小。
func GetFileOperationsTools() []tool.Tool {
	wftool := function.NewFunctionTool(
		WriteFile,
		function.WithName("WriteFile"),
		function.WithDescription("将内容写入指定文件，如果文件不存在则创建，已存在则覆盖。"),
	)
	rftool := function.NewFunctionTool(
		ReadFile,
		function.WithName("ReadFile"),
		function.WithDescription("从指定文件读取内容，支持设置读取窗口大小和偏移量。"),
	)
	eftool := function.NewFunctionTool(
		EditFile,
		function.WithName("EditFile"),
		function.WithDescription("编辑指定文件中的内容，支持替换指定的旧内容为新内容。注意：会替换文件中所有匹配的字符串，非仅第一处。"),
	)
	sftool := function.NewFunctionTool(
		SearchInFile,
		function.WithName("SearchInFile"),
		function.WithDescription("通过正则表达式在指定文件中搜索内容，返回所有匹配项的行号和内容。使用Go RE2语法，不支持lookahead/lookbehind/backreference。`.`默认不匹配换行，跨行匹配用`(?s)`。`^`和`$`默认匹配文本首尾，匹配行首行尾用`(?m)`。"),
	)
	dftool := function.NewFunctionTool(
		DeleteFile,
		function.WithName("DeleteFile"),
		function.WithDescription("删除指定文件或目录，目录会被递归删除，请谨慎使用。"),
	)
	fitool := function.NewFunctionTool(
		FileInfo,
		function.WithName("FileStat"),
		function.WithDescription("获取指定文件或目录的信息，包括名称、大小、是否为目录、权限模式和修改时间等。"),
	)
	return []tool.Tool{wftool, rftool, eftool, sftool, dftool, fitool}
}
