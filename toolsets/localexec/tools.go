package localexec

import (
	"context"
	"errors"

	"encoding/json"
	"strconv"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 工具：提交命令
func submit_command(ctx context.Context, req struct {
	Process string   `json:"process" jsonschema:"description:要执行的程序名。"`
	Args    []string `json:"args" jsonschema:"description:程序的执行参数。"`
}) (map[string]string, error) {

	if req.Process == "" {
		return nil, errors.New("`Process` cannot be empty")
	}

	id := manager.Submit(SubmitOptions{
		Command: req.Process,
		Args:    req.Args,
	})
	st := manager.Status(id)

	result := map[string]string{
		"id":      id,
		"status":  st.Status,
		"message": "Command submitted successfully. Use `start_command` to execute",
	}
	return result, nil
}

// 工具：执行命令
func start_command(ctx context.Context, req struct {
	Id string `json:"id" jsonschema:"description:命令ID"`
}) (map[string]string, error) {

	if req.Id == "" {
		return nil, errors.New("`id` cannot be empty")
	}
	err := manager.Start(req.Id)
	if err != nil {
		return nil, err
	}
	st := manager.Status(req.Id)
	return map[string]string{
		"id":      req.Id,
		"status":  st.Status,
		"message": "Command started successfully. Use `get_status` to check running status and `get_output` to retrieve output",
	}, nil
}

// 工具：查看命令状态（可选ID）
func get_status(ctx context.Context, req struct {
	Id string `json:"id" jsonschema:"description:命令ID"`
}) (map[string]string, error) {

	if req.Id != "" {
		st := manager.Status(req.Id)
		PID_s := strconv.Itoa(st.PID)
		ExitCode_s := strconv.Itoa(st.ExitCode)
		return map[string]string{
			"id":       st.ID,
			"status":   st.Status,
			"pid":      PID_s,
			"error":    st.Error,
			"exitCode": ExitCode_s,
		}, nil
	}
	list := manager.StatusAll()
	return map[string]string{
		"status_all": marshalJson(list),
	}, nil
}

// 工具：获取输出
func get_output(ctx context.Context, req struct {
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
	data, err := manager.Output(req.Id, OutputOptions{Window: req.Window, Stream: req.Stream})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"id":     req.Id,
		"output": string(data),
	}, nil
}

// 工具：干预命令（stdin 或 signal）
func intervene_command(ctx context.Context, req struct {
	Id     string `json:"id" jsonschema:"description:命令ID"`
	Input  string `json:"input" jsonschema:"description:可选：写入到stdin的字符串"`
	Signal string `json:"signal" jsonschema:"description:可选：信号类型，如SIGINT/SIGTERM/SIGKILL(跨平台差异)"`
}) (map[string]string, error) {
	if req.Id == "" {
		return nil, errors.New("`id` cannot be empty")
	}
	if req.Input != "" {
		err := manager.WriteStdin(req.Id, []byte(req.Input))
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"id":  req.Id,
			"msg": "input written to stdin",
		}, nil
	}
	if req.Signal != "" {
		err := manager.Signal(req.Id, req.Signal)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"id":  req.Id,
			"msg": "signal sent",
		}, nil
	}
	return map[string]string{
		"id":  req.Id,
		"msg": "no action taken; provide `input` or `signal`",
	}, nil
}

// 工具：强制结束
func kill_command(ctx context.Context, req struct {
	Id string `json:"id" jsonschema:"description:命令ID"`
}) (map[string]string, error) {
	if req.Id == "" {
		return nil, errors.New("`id` cannot be empty")
	}
	err := manager.Kill(req.Id)
	if err != nil {
		return nil, err
	}
	st := manager.Status(req.Id)
	return map[string]string{
		"id":     req.Id,
		"status": st.Status,
	}, nil
}

func GetTools() []tool.Tool {
	toolSets := []tool.Tool{}

	submit_commandTool := function.NewFunctionTool(
		submit_command,
		function.WithName("submit_command"),
		function.WithDescription("提交一条命令，返回命令ID与初始状态。命令由`Process`和`Args`组成，支持跨平台的shell命令执行，如`bash -c 'echo Hello World'`或`cmd /c 'echo Hello World'`或`powershell -Command 'echo Hello World'`"),
	)

	start_commandTool := function.NewFunctionTool(
		start_command,
		function.WithName("start_command"),
		function.WithDescription("根据命令ID启动已提交的命令"),
	)

	get_statusTool := function.NewFunctionTool(
		get_status,
		function.WithName("get_status"),
		function.WithDescription("查看命令状态；如不传ID返回全部命令状态"),
	)

	get_outputTool := function.NewFunctionTool(
		get_output,
		function.WithName("get_output"),
		function.WithDescription("获取命令输出；支持窗口大小与选择stdout/stderr"),
	)

	intervene_commandTool := function.NewFunctionTool(
		intervene_command,
		function.WithName("intervene_command"),
		function.WithDescription("向运行中的命令写入stdin或发送信号(Windows仅支持stdin与强制结束)"),
	)

	kill_commandTool := function.NewFunctionTool(
		kill_command,
		function.WithName("kill_command"),
		function.WithDescription("强制结束运行中的命令"),
	)
	toolSets = append(toolSets, submit_commandTool, start_commandTool, get_statusTool, get_outputTool, intervene_commandTool, kill_commandTool)
	return toolSets
}

func marshalJson(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
