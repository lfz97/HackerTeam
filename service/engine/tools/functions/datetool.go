package functionTools

import (
	"context"
	"time"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func DateNow(ctx context.Context, req struct {
}) (map[string]string, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	return map[string]string{
		"datetime": now,
	}, nil
}

func GetDateTools() []tool.Tool {
	dtool := function.NewFunctionTool(
		DateNow,
		function.WithName("date_now"),
		function.WithDescription("获取当前日期和时间，格式为YYYY-MM-DD HH:MM:SS"),
	)
	return []tool.Tool{dtool}
}
