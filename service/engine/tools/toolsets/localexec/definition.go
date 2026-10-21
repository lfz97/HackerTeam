package localexec

import (
	"context"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// LocalExecToolSet 的 name 为空串（见 LocalExec）：框架对空名工具集不加
// 前缀，工具以 mapper.go 里登记的名字原样暴露给模型。
// MCP 工具集保留前缀，不会与本工具集裸名冲突。
type LocalExecToolSet struct {
	name string
	mgr  *Manager
}

func (l *LocalExecToolSet) Tools(context.Context) []tool.Tool {
	return getTools(l.mgr)
}

// Close 先kill所有未结束的命令（释放占用的端口/进程），再清空注册表。
// Kill 自身持 job 锁，此处先拷贝ID列表再逐个 Kill，避免锁序问题。
func (l *LocalExecToolSet) Close() error {
	l.mgr.mu.RLock()
	ids := make([]string, 0, len(l.mgr.jobs))
	for id := range l.mgr.jobs {
		ids = append(ids, id)
	}
	l.mgr.mu.RUnlock()
	for _, id := range ids {
		l.mgr.Kill(id) //已结束的任务返回错误，忽略
	}
	l.mgr.mu.Lock()
	defer l.mgr.mu.Unlock()
	l.mgr.jobs = map[string]*Job{}
	return nil
}

func (l *LocalExecToolSet) Name() string {
	return l.name
}

func LocalExec() tool.ToolSet {
	return &LocalExecToolSet{
		name: "",
		mgr:  &Manager{jobs: map[string]*Job{}},
	}
}
