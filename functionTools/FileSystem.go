package functionTools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 获取当前工作目录
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

// 列出指定目录下的文件和子目录
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

// 切换当前工作目录
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

// 创建目录
func Mkdir(ctx context.Context, req struct {
	Path    string `json:"path" jsonschema:"description:要创建的目录路径。"`
	Parents bool   `json:"parents" jsonschema:"description:是否自动创建父目录。默认为false。"`
}) (map[string]string, error) {
	if req.Path == "" {
		return nil, errors.New("`path` can't be empty!")
	}
	if req.Parents {
		err := os.MkdirAll(req.Path, 0755)
		if err != nil {
			return nil, err
		}
	} else {
		err := os.Mkdir(req.Path, 0755)
		if err != nil {
			return nil, err
		}
	}
	return map[string]string{
		"CreatedDir": req.Path,
	}, nil
}

func MV(ctx context.Context, req struct {
	OldPath string `json:"oldPath" jsonschema:"description:要移动或重命名的文件或目录的原路径。"`
	NewPath string `json:"newPath" jsonschema:"description:要移动或重命名的文件或目录的新路径。"`
}) (map[string]string, error) {
	if req.OldPath == "" || req.NewPath == "" {
		return nil, errors.New("`oldPath` and `newPath` can't be empty!")
	}
	if req.OldPath == req.NewPath {
		return nil, errors.New("`oldPath` and `newPath` can't be the same!")
	}
	err := os.Rename(req.OldPath, req.NewPath)
	if err == nil {
		return map[string]string{
			"OldPath": req.OldPath,
			"NewPath": req.NewPath,
		}, nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return nil, err
	}
	// 跨设备移动：复制后删除
	srcInfo, err := os.Stat(req.OldPath)
	if err != nil {
		return nil, err
	}
	if srcInfo.IsDir() {
		err = moveDir(req.OldPath, req.NewPath)
	} else {
		err = moveFile(req.OldPath, req.NewPath, srcInfo)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"OldPath": req.OldPath,
		"NewPath": req.NewPath,
	}, nil
}

func moveFile(src, dst string, srcInfo os.FileInfo) error {
	if src == dst {
		return errors.New("source and destination paths are the same")
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("source file was moved but could not be removed: %w", err)
	}
	return nil
}

func moveDir(src, dst string) error {
	// MV 语义：用源替换目标，先删除目标再复制，避免 os.CopyFS 的合并行为
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("failed to remove existing destination: %w", err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return fmt.Errorf("failed to copy directory: %w", err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("source directory was moved but could not be removed: %w", err)
	}
	return nil
}

// 获取文件系统相关工具集合
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
	mkdirTool := function.NewFunctionTool(
		Mkdir,
		function.WithName("Mkdir"),
		function.WithDescription("创建目录，支持递归创建父目录"),
	)
	mvTool := function.NewFunctionTool(
		MV,
		function.WithName("MV"),
		function.WithDescription("移动或重命名文件或目录，支持跨设备移动"),
	)
	return []tool.Tool{pwdtool, cdtool, lstool, mkdirTool, mvTool}
}
