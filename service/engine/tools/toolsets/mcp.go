package toolsets

import (
	"HackerTeam/service/engine/config"
	"context"
	"errors"
	"fmt"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// defaultMCPTimeout 单次 MCP 工具调用超时的默认值。旧值 10s 对任何做实际工作的
// server 都太苛刻（长分析型调用必然 context deadline exceeded），提到 2 分钟；
// 个别 server 用 yaml 的 timeout 字段覆盖（单位/秒，<=0 落到这里）。
const defaultMCPTimeout = 2 * time.Minute

// mcpTimeout 把 yaml 里的秒数换算成 Duration，<=0 视为未配置。
func mcpTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultMCPTimeout
	}
	return time.Duration(seconds) * time.Second
}

// HttpMCP / StdinMCP 返回包了一层超时错误改写的 ToolSet：框架把工具错误的
// err.Error() 原文回给模型，「context deadline exceeded」既没说等多久也没说
// 该怎么办，模型拿到后只会无意义重试（超时按次调用算，重试必然再超时）。
// 改写成带时长与出路的文本，止住无效重试循环。ESC 打断不受影响：那是父 ctx
// 的 cancel（context.Canceled），与本包装处理的 DeadlineExceeded 是两条路。
// 实例生命周期不变：同一实例共享挂给全部 agent，每轮 run 由 Engine 重建前
// Close 旧实例（见 init.go 的 refreshMCPFromConfig）。

func HttpMCP(cfg config.HttpMCP) tool.ToolSet {
	timeout := mcpTimeout(cfg.Timeout)
	inner := mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport:   string(cfg.Type),
			ServerURL:   cfg.Endpoint,
			Timeout:     timeout,
			Headers:     cfg.Headers,
			Description: cfg.Description,
		},
		mcp.WithName(cfg.Name), //决定工具前缀 {Name}_{toolName}，多server必须唯一
		mcp.WithSessionReconnect(3),
	)
	return &friendlyToolSet{inner: inner, timeout: timeout}
}

func StdinMCP(cfg config.StdinMCP) tool.ToolSet {
	timeout := mcpTimeout(cfg.Timeout)
	inner := mcp.NewMCPToolSet(
		mcp.ConnectionConfig{
			Transport:   "stdio",
			Command:     cfg.Command,
			Args:        cfg.Args,
			Timeout:     timeout,
			Description: cfg.Description,
		},
		mcp.WithName(cfg.Name), //决定工具前缀 {Name}_{toolName}，多server必须唯一
		mcp.WithSessionReconnect(3),
	)
	return &friendlyToolSet{inner: inner, timeout: timeout}
}

// ── 超时错误改写装饰器 ─────────────────────────────────────

// friendlyToolSet 转发 ToolSet 三件套，Tools() 逐个包上 friendlyTool。
type friendlyToolSet struct {
	inner   tool.ToolSet
	timeout time.Duration
}

func (t *friendlyToolSet) Tools(ctx context.Context) []tool.Tool {
	var wrapped []tool.Tool
	for _, tl := range t.inner.Tools(ctx) {
		if _, ok := tl.(tool.CallableTool); !ok {
			wrapped = append(wrapped, tl) // 不可调用的声明型工具原样放行
			continue
		}
		wrapped = append(wrapped, &friendlyTool{inner: tl, timeout: t.timeout})
	}
	return wrapped
}

func (t *friendlyToolSet) Close() error { return t.inner.Close() }
func (t *friendlyToolSet) Name() string { return t.inner.Name() }

// friendlyTool 只包装 Call 的错误路径；ToolMetadata 必须转发——mcpTool 实现了
// MetadataProvider，不转发会让框架的断言拿到零值元数据（并发安全位等）。
// StreamableCall 不转发：mcpTool 没有流式形态，本装饰器只用于 MCP 工具集。
type friendlyTool struct {
	inner   tool.Tool
	timeout time.Duration
}

func (t *friendlyTool) Declaration() *tool.Declaration { return t.inner.Declaration() }

func (t *friendlyTool) ToolMetadata() tool.ToolMetadata { return tool.MetadataOf(t.inner) }

func (t *friendlyTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	res, err := t.inner.(tool.CallableTool).Call(ctx, jsonArgs)
	// 框架用 %w 包装超时（failed to call tool X: context deadline exceeded），
	// errors.Is 沿链可判；用户 ESC 产生的是 context.Canceled，不会误伤。
	if errors.Is(err, context.DeadlineExceeded) {
		return res, fmt.Errorf(
			"MCP server call timed out after %s (context deadline exceeded); the operation did not complete. "+
				"Retrying the same call will time out again: use a narrower task, "+
				"or raise the 'timeout' field of this MCP server in HackerTeam.yaml",
			t.timeout)
	}
	return res, err
}
