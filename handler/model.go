package handler

type TurnResult struct {
	Code       TurnCode
	Reason     string
	OutputPart string
}
type TurnCode int

const (
	New      TurnCode = 1 //新对话
	Int      TurnCode = 2 //用户中断
	Error    TurnCode = 3 //错误
	Exit     TurnCode = 4 //用户退出
	Continue TurnCode = 5 //继续对话
	Flush    TurnCode = 6 //刷新工具
)
