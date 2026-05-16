package localexec

import (
	"context"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type LocalExecToolSet struct {
	name string
	mgr  *Manager
}

func (l *LocalExecToolSet) Tools(context.Context) []tool.Tool {
	return getTools(l.mgr)
}

func (l *LocalExecToolSet) Close() error {
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
		name: "LocalExec",
		mgr:  &Manager{jobs: map[string]*Job{}},
	}
}
