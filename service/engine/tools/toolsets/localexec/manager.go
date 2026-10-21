package localexec

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Submit 创建任务但不启动
func (m *Manager) Submit(opts SubmitOptions) string {
	id := randomID()
	job := &Job{SubmitOptions: opts, ID: id, status: StatusPending, createdAt: time.Now()}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()
	return id
}

// Start 启动任务
func (m *Manager) Start(id string) error {

	// Windows 不支持 PTY，降级使用普通 pipe
	if runtime.GOOS == "windows" {
		err := m.startCmdWithPipe(id)
		return err
	} else {
		err := m.startCmdWithPty(id)
		return err
	}

}

func (m *Manager) startCmdWithPipe(id string) error {
	job := m.get(id)
	if job == nil {
		return errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()

	if job.status != StatusPending {
		return errors.New("job not in pending status")
	}
	cmd := buildCmd(job.SubmitOptions)
	// 用非 *os.File 的 writer 让 exec 自己起拷贝 goroutine，而不是 StdoutPipe +
	// 自建拷贝：StdoutPipe 的读端归 Wait 关，Wait 一返回未排空的输出就被静默丢弃；
	// 这样 Wait 会等拷贝全部结束，「状态为 done 时输出必然完整」由结构保证。
	cmd.Stdout = lockedWriter{mu: &job.mu, buf: &job.stdoutBuf}
	cmd.Stderr = lockedWriter{mu: &job.mu, buf: &job.stderrBuf}
	// WaitDelay 兜底：进程已退出但其派生的后台进程仍攥着写端，EOF 永远不来。
	// 计时器在进程退出后才启动，长跑命令不受影响。
	cmd.WaitDelay = pipeWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		job.status = StatusFailed
		job.errStr = err.Error()
		return err
	}
	job.stdin = stdin
	if err := cmd.Start(); err != nil {
		job.status = StatusFailed
		job.errStr = err.Error()
		return err
	}
	job.cmd = cmd
	job.pid = cmd.Process.Pid
	job.status = StatusRunning
	job.startedAt = time.Now()
	go func() {
		err := cmd.Wait()
		// ErrWaitDelay 含义是「输出没收全」而非「命令失败」（进程失败时退出错误优先），
		// 按成功处理，退出码仍是 0。
		if errors.Is(err, exec.ErrWaitDelay) {
			err = nil
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		job.endedAt = time.Now()
		job.pid = 0
		if err != nil {
			job.status = StatusFailed
			job.errStr = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				job.exitCode = exitErr.ExitCode()
			}
		} else {
			job.status = StatusDone
			job.exitCode = 0
		}
	}()
	return nil
}

func (m *Manager) startCmdWithPty(id string) error {
	job := m.get(id)
	if job == nil {
		return errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()

	if job.status != StatusPending {
		return errors.New("job not in pending status")
	}
	cmd := buildCmd(job.SubmitOptions)
	// Unix: 用 PTY 替代普通 pipe，避免 ssh/sudo 等直接写 /dev/tty 破坏 TUI
	ptmx, err := pty.Start(cmd)
	if err != nil {
		job.status = StatusFailed
		job.errStr = err.Error()
		return err
	}
	job.ptmx = ptmx
	job.stdin = nil // stdin 经 ptmx 写入，不再需要 pipe

	job.cmd = cmd
	job.pid = cmd.Process.Pid
	job.status = StatusRunning
	job.startedAt = time.Now()

	// stdout/stderr 合并在 ptmx 一个 fd 里读
	copyDone := make(chan struct{})
	go func() {
		copyStream(ptmx, &job.stdoutBuf, &job.mu)
		close(copyDone)
	}()

	go func() {
		err := cmd.Wait()
		// 等拷贝 goroutine 把剩余输出写完再置状态，避免调用方看到 done 却读到
		// 截断的输出；若命令派生了持有 tty 的后台进程迟迟不 EOF，超时强制关闭防永久阻塞。
		select {
		case <-copyDone:
		case <-time.After(time.Second):
			ptmx.Close()
			<-copyDone
		}
		ptmx.Close()
		job.mu.Lock()
		defer job.mu.Unlock()
		job.endedAt = time.Now()
		job.pid = 0 // 进程已结束，PID无效
		if err != nil {
			job.status = StatusFailed
			job.errStr = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				job.exitCode = exitErr.ExitCode()
			}
		} else {
			job.status = StatusDone
			job.exitCode = 0
		}
	}()

	return nil
}

// Status 返回某任务状态
func (m *Manager) Status(id string) StatusInfo {
	job := m.get(id)
	if job == nil {
		return StatusInfo{ID: id, Status: "not-found"}
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	return StatusInfo{
		ID:        job.ID,
		Status:    job.status,
		PID:       job.pid,
		ExitCode:  job.exitCode,
		Error:     job.errStr,
		Command:   job.Command,
		CreatedAt: job.createdAt,
		StartedAt: job.startedAt,
		EndedAt:   job.endedAt,
	}
}

// StatusAll 返回全部任务的状态
func (m *Manager) StatusAll() []StatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]StatusInfo, 0, len(m.jobs))
	for _, j := range m.jobs {
		res = append(res, m.Status(j.ID))
	}
	return res
}

// Output 返回输出
func (m *Manager) Output(id string, opts OutputOptions) ([]byte, error) {
	job := m.get(id)
	if job == nil {
		return nil, errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	var buf *bytes.Buffer
	if strings.ToLower(opts.Stream) == "stderr" {
		buf = &job.stderrBuf
	} else {
		buf = &job.stdoutBuf
	}
	data := buf.Bytes()
	// 持锁期间复制一份再返回，避免调用方在锁外使用 buffer 底层数组时与写入竞争
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// WriteStdin 写入stdin
func (m *Manager) WriteStdin(id string, data []byte) error {
	job := m.get(id)
	if job == nil {
		return errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.status != StatusRunning {
		return errors.New("job not running")
	}
	// PTY 模式下通过 master fd 写入，普通 pipe 模式通过 stdin pipe 写入
	if job.ptmx != nil {
		_, err := job.ptmx.Write(data)
		return err
	}
	if job.stdin == nil {
		return errors.New("stdin not available")
	}
	_, err := job.stdin.Write(data)
	return err
}

// signals 仅收录 Windows 的 syscall 包同样定义的信号常量，保证跨平台编译；
// Windows 走 Kill 分支，不会真的用到这些值。
var signals = map[string]syscall.Signal{
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
	"QUIT": syscall.SIGQUIT,
	"ABRT": syscall.SIGABRT,
	"TERM": syscall.SIGTERM,
	"KILL": syscall.SIGKILL,
}

// Signal 发送信号（Windows仅支持Kill作为强制结束）
func (m *Manager) Signal(id, signal string) error {
	job := m.get(id)
	if job == nil {
		return errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.status != StatusRunning {
		return errors.New("job not running")
	}
	if job.cmd == nil || job.cmd.Process == nil {
		return errors.New("process not available")
	}
	if runtime.GOOS == "windows" {
		// 简化：Windows不区分信号，统一Kill
		return job.cmd.Process.Kill()
	}
	// 非Windows：SIGTERM 必须真发 TERM，用 Kill 代替会让进程的 trap/清理逻辑
	// 完全没机会执行，表现与 SIGKILL 无异
	name := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(signal), "SIG"))
	sig, ok := signals[name]
	if !ok {
		return fmt.Errorf("unsupported signal %q; want one of SIGHUP/SIGINT/SIGQUIT/SIGABRT/SIGTERM/SIGKILL", signal)
	}
	return job.cmd.Process.Signal(sig)
}

// Kill 强制结束
func (m *Manager) Kill(id string) error {
	job := m.get(id)
	if job == nil {
		return errors.New("job not found")
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.cmd == nil || job.cmd.Process == nil {
		return errors.New("process not available")
	}
	if err := job.cmd.Process.Kill(); err != nil {
		return err
	}
	if job.ptmx != nil {
		job.ptmx.Close()
	}
	job.status = StatusKilled
	job.endedAt = time.Now()
	job.pid = 0
	return nil
}

// 内部工具
func (m *Manager) get(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[id]
}

// copyStream 分块读取 r 写入 buf，每块写入时短暂持有 mu，与 Output 的读取互斥；
// 不全程持锁，保证命令运行中也能读到已产生的输出。
// 仅 PTY 模式使用：ptmx 由我们自己读；pipe 模式由 exec 经 lockedWriter 写入。
func copyStream(r io.Reader, buf *bytes.Buffer, mu *sync.Mutex) {
	b := make([]byte, 32*1024)
	for {
		n, err := r.Read(b)
		if n > 0 {
			mu.Lock()
			buf.Write(b[:n])
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// pipeWaitDelay 是 pipe 模式下进程退出后等输出收全的上限，超时即关闭管道按成功收尾。
const pipeWaitDelay = 2 * time.Second

// lockedWriter 把 exec 拷贝 goroutine 写来的输出转进 buf，每批数据短暂持有 mu。
// 刻意复用 Job 自己的 mu 与 buf：Output() 在同一对组合上读取，共用一把锁才不竞争。
type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func buildCmd(opts SubmitOptions) *exec.Cmd {
	return exec.Command(opts.Command, opts.Args...)
}
