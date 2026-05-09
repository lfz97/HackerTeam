package functionTools

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func WriteFile(ctx context.Context, req struct {
	Path    string `json:"path" jsonschema:"description:要写入的文件路径。"`
	Content string `json:"content" jsonschema:"description:写入的文件内容"`
}) (map[string]string, error) {

	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}

	fd, err := os.OpenFile(req.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
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
	Window int    `json:"window" jsonschema:"description:读取文件的窗口大小，单位为字节。默认为1024字节。"`
}) (map[string]string, error) {

	if req.Path == "" {
		return nil, errors.New("`Path` cannot be empty")
	}
	if req.Window == 0 {
		req.Window = 1024
	}
	fd, err := os.OpenFile(req.Path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	buf := make([]byte, req.Window) //根据请求的窗口大小创建缓冲区
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
		function.WithDescription("从指定文件读取内容，支持设置读取窗口大小。"),
	)
	return []tool.Tool{wftool, rftool}
}
