package localexec

import (
	"context"
	"errors"

	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"HackerTeam/service/engine/tools/modeltext"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// deliver 把工具输出写进结果 map：不超过内联上限就原样返回；超过则完整落盘，
// 只内联头部预览并附文件路径。落盘走正常返回值而非 error——这是一次成功交付，
// 用 error 返回会让上层丢掉整个结果。
func deliver(res map[string]string, data []byte, id string) error {
	// 按序列化后大小判定而非原始字节数：JSON 序列化对非法 UTF-8/控制字符有
	// 最高 6 倍的放大（见 modeltext）。内容原样返回不转换。
	text := string(data)
	if modeltext.Size(text) <= inlineMaxBytes {
		res["output"] = text
		return nil
	}
	return deliverSpilled(res, data, id)
}

// deliverSpilled 把完整输出落盘，只内联头部预览并附上文件路径与提示。
// totalBytes/尾部偏移报的是文件真实字节数，read 分页读的就是原始文件。
func deliverSpilled(res map[string]string, data []byte, id string) error {
	path, err := spill(id, data)
	if err != nil {
		return err
	}
	head := preview(data)
	// 尾部偏移直接算好给 agent，省得它自己算错
	tailOffset := len(data) - 4096
	if tailOffset < 0 {
		tailOffset = 0
	}
	res["output"] = string(head)
	res["outputFile"] = path
	res["totalBytes"] = strconv.Itoa(len(data))
	res["note"] = fmt.Sprintf(
		"Output is %d bytes, over the %d-byte inline limit; the full content has been saved to outputFile. "+
			"The output above is only a %d-byte head preview and does not represent the whole output. "+
			"Failure reasons, test summaries and stack traces are usually at the end: pass offset=%d to `read` to read the tail directly, "+
			"or use Grep to look for keywords.",
		len(data), inlineMaxBytes, len(head), tailOffset,
	)
	return nil
}

func getTools(m *Manager) []tool.Tool {
	toolSets := []tool.Tool{}

	runTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Process string   `json:"process" jsonschema:"description=Name of the program to execute."`
			Args    []string `json:"args" jsonschema:"description=Arguments passed to the program."`
		}) (map[string]string, error) {
			if req.Process == "" {
				return nil, errors.New("`process` cannot be empty")
			}
			id := m.Submit(SubmitOptions{Command: req.Process, Args: req.Args})
			if err := m.Start(id); err != nil {
				return nil, err
			}
			isFinished := func(st StatusInfo) bool {
				return st.Status == StatusDone || st.Status == StatusFailed || st.Status == StatusKilled
			}
			// 失败时合并 stdout+stderr（PTY 下 stderr 合流进 stdout，只有
			// Windows pipe 模式才有独立 stderr）
			buildResult := func(st StatusInfo, id string) (map[string]string, error) {
				out, err := m.Output(id, OutputOptions{Stream: "stdout"})
				if err != nil {
					return nil, err
				}
				res := map[string]string{
					"id":       id,
					"status":   st.Status,
					"exitCode": strconv.Itoa(st.ExitCode),
				}
				if st.Status == StatusFailed {
					res["error"] = st.Error
					errOut, err := m.Output(id, OutputOptions{Stream: "stderr"})
					if err != nil {
						return nil, err
					}
					// 合并后再做落盘判断，否则两条流各落一个文件；
					// out 是 Output() 的副本，append 不会写坏 job 的 buffer
					out = append(out, errOut...)
				}
				if err := deliver(res, out, id); err != nil {
					return nil, err
				}
				return res, nil
			}

			// 先立即检查一次，避免瞬间完成的命令还要等满首个 tick
			if st := m.Status(id); isFinished(st) {
				return buildResult(st, id)
			}

			deadline := time.After(20 * time.Second)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-deadline:
					st := m.Status(id)
					return map[string]string{
						"id":      id,
						"status":  st.Status,
						"message": "Command has been running for 20 seconds and has been switched to background execution. Please use `status` to check its running status and `output` to retrieve its output",
					}, nil
				case <-ticker.C:
					if st := m.Status(id); isFinished(st) {
						return buildResult(st, id)
					}
				}
			}
		},
		function.WithName(runToolName),
		function.WithDescription("Execute a command built from `process` (program name) and `args` (argument array). Pick the shell for the operating system: Windows only supports `powershell` or `cmd` (e.g. `args=[\"-Command\",\"echo hello\"]`), while Unix/Linux/macOS use `bash`/`sh` (e.g. `args=[\"-c\",\"echo hello\"]`). Blocks for at most 20 seconds: if the command finishes within that window it returns the status (`status`), exit code (`exitCode`) and output (`output`) directly; if it is still running after 20 seconds it is switched to background execution and the command ID is returned, after which `CommandStatus` checks its state, `CommandOutput` fetches its output, `CommandIntervene` writes stdin or sends signals and `CommandKill` terminates it forcefully. Output larger than the inline limit is saved to a file and only a head preview is returned inline; see `outputFile` and `note` in the result."),
	)

	statusTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id           string `json:"id" jsonschema:"description=Optional: command ID. When omitted the status of every command is returned."`
			Wait_seconds int    `json:"wait_seconds" jsonschema:"description=Optional: wait up to n seconds for the command to finish before returning its status; polls once per second and returns early as soon as the command completes. Defaults to 0 which returns the current status immediately without waiting."`
		}) (map[string]string, error) {
			if req.Wait_seconds < 0 {
				return nil, errors.New("`wait_seconds` must be >= 0")
			}
			if req.Id != "" && req.Wait_seconds > 0 {
				deadline := time.After(time.Duration(req.Wait_seconds) * time.Second)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-deadline:
						st := m.Status(req.Id)
						return map[string]string{
							"id":       st.ID,
							"status":   st.Status,
							"pid":      strconv.Itoa(st.PID),
							"error":    st.Error,
							"exitCode": strconv.Itoa(st.ExitCode),
							"msg":      "wait timed out, command still running",
						}, nil
					case <-ticker.C:
						st := m.Status(req.Id)
						if st.Status == "done" || st.Status == "failed" || st.Status == "killed" {
							return map[string]string{
								"id":       st.ID,
								"status":   st.Status,
								"pid":      strconv.Itoa(st.PID),
								"error":    st.Error,
								"exitCode": strconv.Itoa(st.ExitCode),
							}, nil
						}
					}
				}
			}
			if req.Id != "" {
				st := m.Status(req.Id)
				return map[string]string{
					"id":       st.ID,
					"status":   st.Status,
					"pid":      strconv.Itoa(st.PID),
					"error":    st.Error,
					"exitCode": strconv.Itoa(st.ExitCode),
				}, nil
			}
			list := m.StatusAll()
			return map[string]string{
				"status_all": marshalJson(list),
			}, nil
		},
		function.WithName(statusToolName),
		function.WithDescription("Check command status; returns the status of every command when no ID is given. With wait_seconds it blocks until the command completes (polling every second and returning as soon as it does), or returns the current status on timeout; suited to long-running async tasks and avoids repeated polling."),
	)

	outputTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id     string `json:"id" jsonschema:"description=Command ID."`
			Stream string `json:"stream" jsonschema:"description=Optional: output stream; stdout or stderr. Defaults to stdout."`
		}) (map[string]string, error) {
			if req.Id == "" {
				return nil, errors.New("`id` cannot be empty")
			}
			if req.Stream == "" {
				req.Stream = "stdout"
			}
			data, err := m.Output(req.Id, OutputOptions{Stream: req.Stream})
			if err != nil {
				return nil, err
			}
			res := map[string]string{"id": req.Id}
			if err := deliver(res, data, req.Id); err != nil {
				return nil, err
			}
			return res, nil
		},
		function.WithName(outputToolName),
		function.WithDescription("Fetch command output; choose stdout or stderr. Output larger than the inline limit is saved to a file and only a head preview is returned inline; see `outputFile` and `note` in the result."),
	)

	interveneTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id     string `json:"id" jsonschema:"description=Command ID."`
			Input  string `json:"input" jsonschema:"description=Optional: string written to stdin."`
			Signal string `json:"signal" jsonschema:"description=Optional: signal type such as SIGINT/SIGTERM/SIGKILL (support varies across platforms)."`
		}) (map[string]string, error) {
			if req.Id == "" {
				return nil, errors.New("`id` cannot be empty")
			}
			if req.Input != "" {
				if err := m.WriteStdin(req.Id, []byte(req.Input)); err != nil {
					return nil, err
				}
				return map[string]string{"id": req.Id, "msg": "input written to stdin"}, nil
			}
			if req.Signal != "" {
				if err := m.Signal(req.Id, req.Signal); err != nil {
					return nil, err
				}
				return map[string]string{"id": req.Id, "msg": "signal sent"}, nil
			}
			return map[string]string{"id": req.Id, "msg": "no action taken; provide `input` or `signal`"}, nil
		},
		function.WithName(interveneToolName),
		function.WithDescription("Write to stdin or send a signal to a running command. Windows only supports stdin and forced termination, so use `CommandKill` when you need to terminate a command."),
	)

	killTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id string `json:"id" jsonschema:"description=Command ID."`
		}) (map[string]string, error) {
			if req.Id == "" {
				return nil, errors.New("`id` cannot be empty")
			}
			if err := m.Kill(req.Id); err != nil {
				return nil, err
			}
			st := m.Status(req.Id)
			return map[string]string{"id": req.Id, "status": st.Status}, nil
		},
		function.WithName(killToolName),
		function.WithDescription("Forcefully terminate a running command."),
	)

	toolSets = append(toolSets, runTool, statusTool, outputTool, interveneTool, killTool)
	return toolSets
}

func marshalJson(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
