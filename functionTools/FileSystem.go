package functionTools

import (
	"context"
	"encoding/json"
	"os"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func PWD(ctx context.Context, req struct {
}) (map[string]string, error) {
	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"PWD": path,
	}, nil
}

type fileInfo struct {
	Name     string      `json:"name"`
	Size     int64       `json:"size"`
	IsDir    bool        `json:"isDir"`
	Mode     os.FileMode `json:"mode"`
	ModeTime string      `json:"modTime"`
}

func LS(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description:要列出文件的目录路径。默认为当前目录。"`
}) (map[string]string, error) {
	if req.Path == "" {
		req.Path = "."
	}
	files, err := os.ReadDir(req.Path)
	if err != nil {
		return nil, err
	}
	fileInfos := make([]fileInfo, 0, len(files))
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			return nil, err
		}
		f := fileInfo{
			Name:     info.Name(),
			Size:     info.Size(),
			IsDir:    info.IsDir(),
			Mode:     info.Mode(),
			ModeTime: info.ModTime().String(),
		}
		fileInfos = append(fileInfos, f)
	}
	jsonBytes, err := json.Marshal(fileInfos)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Files": string(jsonBytes),
	}, nil
}

func CD(ctx context.Context, req struct {
	Path string `json:"path" jsonschema:"description:要切换到的目录路径。"`
}) (map[string]string, error) {
	if req.Path == "" {
		req.Path = "."
	}
	err := os.Chdir(req.Path)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"CWD now": cwd,
	}, nil
}

// 获取文件系统相关工具集合
// PWD：获取当前工作目录
// CD：切换当前工作目录
// LS：列出指定目录下的文件和子目录
func GetFileSystemTools() []tool.Tool {
	pwdtool := function.NewFunctionTool(
		PWD,
		function.WithName("PWD"),
		function.WithDescription("获取当前工作目录"),
	)
	cdtool := function.NewFunctionTool(
		CD,
		function.WithName("CD"),
		function.WithDescription("切换当前工作目录"),
	)
	lstool := function.NewFunctionTool(
		LS,
		function.WithName("LS"),
		function.WithDescription("列出指定目录下的文件和子目录"),
	)
	return []tool.Tool{pwdtool, cdtool, lstool}
}
