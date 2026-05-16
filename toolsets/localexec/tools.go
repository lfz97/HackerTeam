package localexec

import (
	"context"
	"errors"

	"encoding/json"
	"strconv"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func getTools(m *Manager) []tool.Tool {
	toolSets := []tool.Tool{}

	submit_commandTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Process string   `json:"process" jsonschema:"description:要执行的程序名。"`
			Args    []string `json:"args" jsonschema:"description:程序的执行参数。"`
		}) (map[string]string, error) {
			if req.Process == "" {
				return nil, errors.New("`Process` cannot be empty")
			}
			id := m.Submit(SubmitOptions{Command: req.Process, Args: req.Args})
			if err := m.Start(id); err != nil {
				return nil, err
			}
			st := m.Status(id)
			return map[string]string{
				"id":      id,
				"status":  st.Status,
				"message": "Command started successfully. Use `get_status` to check running status and `get_output` to retrieve output",
			}, nil
		},
		function.WithName("submit_command"),
		function.WithDescription("异步执行一条命令并立即返回命令ID与运行状态。命令由`Process`和`Args`组成，支持跨平台shell命令执行，如`bash -c 'echo Hello World'`。命令异步运行，必须使用`get_status`检查是否完成，`get_output`获取输出，`intervene_command`写入stdin，`kill_command`强制终止"),
	)

	get_statusTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id string `json:"id" jsonschema:"description:命令ID"`
		}) (map[string]string, error) {
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
		function.WithName("get_status"),
		function.WithDescription("查看命令状态；如不传ID返回全部命令状态"),
	)

	get_outputTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id     string `json:"id" jsonschema:"description:命令ID"`
			Window int    `json:"window" jsonschema:"description:可选：窗口大小(字节)；默认全部"`
			Stream string `json:"stream" jsonschema:"description:可选：输出流类型，stdout或stderr；默认stdout"`
		}) (map[string]string, error) {
			if req.Id == "" {
				return nil, errors.New("`id` cannot be empty")
			}
			if req.Stream == "" {
				req.Stream = "stdout"
			}
			data, err := m.Output(req.Id, OutputOptions{Window: req.Window, Stream: req.Stream})
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"id":     req.Id,
				"output": string(data),
			}, nil
		},
		function.WithName("get_output"),
		function.WithDescription("获取命令输出；支持窗口大小与选择stdout/stderr"),
	)

	intervene_commandTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id     string `json:"id" jsonschema:"description:命令ID"`
			Input  string `json:"input" jsonschema:"description:可选：写入到stdin的字符串"`
			Signal string `json:"signal" jsonschema:"description:可选：信号类型，如SIGINT/SIGTERM/SIGKILL(跨平台差异)"`
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
		function.WithName("intervene_command"),
		function.WithDescription("向运行中的命令写入stdin或发送信号(Windows仅支持stdin与强制结束)"),
	)

	kill_commandTool := function.NewFunctionTool(
		func(ctx context.Context, req struct {
			Id string `json:"id" jsonschema:"description:命令ID"`
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
		function.WithName("kill_command"),
		function.WithDescription("强制结束运行中的命令"),
	)

	toolSets = append(toolSets, submit_commandTool, get_statusTool, get_outputTool, intervene_commandTool, kill_commandTool)
	return toolSets
}

func marshalJson(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
