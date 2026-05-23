package functionTools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/otiai10/copy"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// 复制文件或目录
func Copy(ctx context.Context, req struct {
	Src string `json:"src" jsonschema:"description:源文件或目录路径。"`
	Dst string `json:"dst" jsonschema:"description:目标文件或目录路径。"`
}) (map[string]string, error) {
	if req.Src == "" || req.Dst == "" {
		return nil, errors.New("`src` and `dst` can't be empty!")
	}
	if req.Src == req.Dst {
		return nil, errors.New("`src` and `dst` can't be the same!")
	}
	if _, err := os.Stat(req.Src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("source path does not exist: %w", err)
		}
		return nil, fmt.Errorf("failed to stat source: %w", err)
	}
	if err := copy.Copy(req.Src, req.Dst); err != nil {
		return nil, fmt.Errorf("failed to copy: %w", err)
	}
	return map[string]string{
		"Src": req.Src,
		"Dst": req.Dst,
	}, nil
}

// 移动或重命名文件或目录
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
	// 同设备直接重命名
	if err := os.Rename(req.OldPath, req.NewPath); err == nil {
		return map[string]string{
			"OldPath": req.OldPath,
			"NewPath": req.NewPath,
		}, nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return nil, err
	}
	// 跨设备移动：先删目标（避免合并），复制源，再删源
	if err := os.RemoveAll(req.NewPath); err != nil {
		return nil, fmt.Errorf("failed to remove existing destination: %w", err)
	}
	if err := copy.Copy(req.OldPath, req.NewPath); err != nil {
		return nil, fmt.Errorf("failed to copy across devices: %w", err)
	}
	if err := os.RemoveAll(req.OldPath); err != nil {
		return nil, fmt.Errorf("source was moved but could not be removed: %w", err)
	}
	return map[string]string{
		"OldPath": req.OldPath,
		"NewPath": req.NewPath,
	}, nil
}

func Glob(ctx context.Context, req struct {
	Regex string `json:"regex" jsonschema:"description:要搜索的正则表达式。"`
	Root  string `json:"root" jsonschema:"description:要搜索的起始路径。默认为当前目录。"`
	Depth int    `json:"depth" jsonschema:"description:搜索深度，默认为0表示同目录，如果传入-1，则无深度限制。"`
}) (map[string]string, error) {
	if req.Depth < -1 {
		return nil, errors.New("`depth` must be -1 (for unlimited) or a non-negative integer")
	}
	if req.Root == "" {
		req.Root = "."
	}
	_, err := os.Stat(req.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("path does not exist: %w", err)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}
	regex_p, err := regexp.Compile(req.Regex)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	mathesFiles := []string{}
	err = filepath.WalkDir(req.Root, func(path string, d os.DirEntry, err error) error {
		rel, err := filepath.Rel(req.Root, path)
		if err != nil {
			return err
		}
		if rel != "." {
			// 深度 = 相对路径中用分隔符分隔的组件数
			depth := len(strings.Split(rel, string(os.PathSeparator))) - 1
			if depth > req.Depth && req.Depth >= 0 {
				return filepath.SkipDir
			}
		}
		if !d.IsDir() {
			if regex_p.MatchString(d.Name()) {
				mathesFiles = append(mathesFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	jsonBytes, err := json.Marshal(mathesFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}
	return map[string]string{
		"matches": string(jsonBytes),
	}, nil
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
	copyTool := function.NewFunctionTool(
		Copy,
		function.WithName("CP"),
		function.WithDescription("复制文件或目录，支持跨设备复制"),
	)
	mvTool := function.NewFunctionTool(
		MV,
		function.WithName("MV"),
		function.WithDescription("移动或重命名文件或目录，支持跨设备移动"),
	)
	globTool := function.NewFunctionTool(
		Glob,
		function.WithName("Glob"),
		function.WithDescription("按正则表达式搜索文件名，支持指定根目录和搜索深度"),
	)
	return []tool.Tool{pwdtool, cdtool, lstool, mkdirTool, copyTool, mvTool, globTool}
}
