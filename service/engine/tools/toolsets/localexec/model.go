package localexec

import (
	"bytes"
	"os"

	"io"

	"os/exec"

	"sync"
	"time"
)

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusKilled  = "killed"
)

type SubmitOptions struct {
	Command string
	Args    []string
}

type OutputOptions struct {
	Stream string // stdout 或 stderr
}

type StatusInfo struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	PID       int       `json:"pid,omitempty"`
	ExitCode  int       `json:"exitCode"`
	Error     string    `json:"error,omitempty"`
	Command   string    `json:"command"`
	Shell     string    `json:"shell"`
	CreatedAt time.Time `json:"createdAt"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
}

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

// Manager 管理所有命令任务的生命周期与输出缓存。
type Manager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}
