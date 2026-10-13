package engine

// 工具结果旁路通道（对应 crush 的 ResponseMetadata 双通道设计）：
// AfterTool 回调把原始类型化结果按 ToolCallID 记录，emitChoice 写 Role=tool
// 记录时随 wireRecord.Meta 下发给 TUI——模型上下文不受影响，工具零改动。
// MCP 工具的 _meta 经 AfterToolArgs.Meta 协议原生透传。

import (
	"context"
	"encoding/json"
	"sync"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// toolSidecar 工具渲染元数据的旁路存储：即写即消（emitChoice 消费挂载），
// 残留只来自取消/异常路径，容量兜底整体清空。
type toolSidecar struct {
	mu sync.Mutex
	m  map[string]json.RawMessage
}

func newToolSidecar() *toolSidecar {
	return &toolSidecar{m: map[string]json.RawMessage{}}
}

// afterTool 旁路记录，不干预主流程（返回 nil 的 AfterToolResult = 不替换结果）。
func (s *toolSidecar) afterTool() tool.AfterToolCallbackStructured {
	return func(ctx context.Context, args *tool.AfterToolArgs) (*tool.AfterToolResult, error) {
		if args.ToolCallID == "" {
			return nil, nil
		}
		payload := map[string]any{"result": args.Result}
		if len(args.Meta) > 0 {
			payload["meta"] = args.Meta
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, nil // 序列化失败只影响渲染增强，不影响主流程
		}
		s.mu.Lock()
		if len(s.m) > 64 { // 正常流程即写即消，超限即泄漏，整体丢弃重建
			s.m = map[string]json.RawMessage{}
		}
		s.m[args.ToolCallID] = b
		s.mu.Unlock()
		return nil, nil
	}
}

// consume 取走并删除（wireRecord 只挂一次；delta/message 双路径先到先得）。
func (s *toolSidecar) consume(toolCallID string) json.RawMessage {
	if toolCallID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.m[toolCallID]
	delete(s.m, toolCallID)
	return v
}
