package localexec

import (
	"bytes"
	"os"

	"io"

	"os/exec"

	"sync"
	"time"
)

// 任务状态
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusKilled  = "killed"
)

// 提交选项
type SubmitOptions struct {
	Command string
	Args    []string
}

// 输出选项
type OutputOptions struct {
	Window int    // 最后N字节；0或负数表示全部
	Stream string // stdout 或 stderr
}

// 状态信息
type StatusInfo struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	PID       int       `json:"pid,omitempty"` // 进程PID
	ExitCode  int       `json:"exitCode"`
	Error     string    `json:"error,omitempty"`
	Command   string    `json:"command"`
	Shell     string    `json:"shell"`
	CreatedAt time.Time `json:"createdAt"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
}

// Job结构
type Job struct {
	SubmitOptions
	ID string

	cmd       *exec.Cmd
	pid       int // 进程PID
	stdin     io.WriteCloser
	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer
	ptmx      *os.File // PTY master fd，nil 表示普通 pipe 模式

	status   string
	exitCode int
	errStr   string

	createdAt time.Time
	startedAt time.Time
	endedAt   time.Time

	mu sync.Mutex
}

// Manager 管理所有任务
type Manager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}
