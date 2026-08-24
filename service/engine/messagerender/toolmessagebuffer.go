package messagerender

import (
	"sync"
)

type toolMessage struct {
	Id                string
	FunctionName      string
	FunctionArguments []byte
	Result            string
	hasResult         bool //标记是否已存入Result
	printed           bool //标记是否已渲染过
}
type toolMsgBuffer struct {
	mu           sync.Mutex
	toolMessages []*toolMessage
}
