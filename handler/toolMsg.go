package handler

import (
	"HackerTeam/global"
	"HackerTeam/utils/pretty"
	"sync"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type toolmsg struct {
	FunctionName      string
	FunctionArguments []byte
	Result            string
}
type toolMsgBufferStruct struct {
	mu         sync.Mutex
	toolMsgMap map[string]*toolmsg
}

var toolMsgBuffer = toolMsgBufferStruct{
	mu:         sync.Mutex{},
	toolMsgMap: map[string]*toolmsg{},
}

func addToolCallMsg(toolcall model.ToolCall) {
	id := toolcall.ID

	toolMsgBuffer.mu.Lock()
	defer toolMsgBuffer.mu.Unlock()

	toolMsgBuffer.toolMsgMap[id] = &toolmsg{
		FunctionName:      toolcall.Function.Name,
		FunctionArguments: toolcall.Function.Arguments,
	}
}

func addToolResultMsg(toolcallid string, content string) {

	toolMsgBuffer.mu.Lock()
	defer toolMsgBuffer.mu.Unlock()

	msg_p := toolMsgBuffer.toolMsgMap[toolcallid]
	if msg_p != nil {
		(*msg_p).Result = content

		global.PrintToTui(global.AgentMessage, pretty.TToolCompact(
			(*msg_p).FunctionName,
			(*msg_p).FunctionArguments,
			(*msg_p).Result,
		), false)

	}
}
