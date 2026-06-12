package bootstrap

import (
	"HackerTeam/global"
	"HackerTeam/handler"
	"HackerTeam/utils/pretty"
	"context"
	"github.com/google/uuid"
)

type RunningStatus string

const END RunningStatus = "end"
const RUN RunningStatus = "run"

func AgentStart() {
	MsgContext := handler.TurnResult{
		Code:       handler.New,
		Reason:     "新对话",
		OutputPart: "",
	}
	RandomStartID()
	for {
		EndTurn_p := handler.AgentRunIteratively(context.Background(), MsgContext)
		if (*EndTurn_p).Code == handler.Exit { //用户主动结束对话，退出程序
			//关闭AgentRunner，释放资源
			(*global.AgentRunner_p).Runner.Close()
			global.ShowMsgAndExitNoTrigger(global.AgentMessage, pretty.TExit("对话已结束，感谢使用！后会有期！"))

		} else if (*EndTurn_p).Code == handler.New { //用户开始新对话，重置global.SessionId, global.RequestId，更新MsgContext为新对话的初始状态
			RandomStartID()
			MsgContext = handler.TurnResult{
				Code:       handler.New,
				Reason:     "新对话",
				OutputPart: "",
			}

		} else if (*EndTurn_p).Code == handler.Flush { //用户主动刷新工具，保持global.SessionId, global.RequestId不变，重新创建Runner
			MsgContext = handler.TurnResult{
				Code:       handler.Flush,
				Reason:     "用户主动刷新工具",
				OutputPart: "",
			}
			LoadConfig()                              //重新加载配置文件，确保工具的最新状态被加载
			(*global.AgentRunner_p).Runner.Close()    //关闭旧的Runner，释放资源
			NewRunner()                               //创建新的agent和Runner，使用最新的工具配置

		} else { //其他情况，继续使用当前的global.SessionId, global.UserId, global.RequestId，更新MsgContext为当前对话的结束状态，供下一轮对话使用
			MsgContext = *EndTurn_p
			continue
		}

	}
}

func RandomStartID() {
	global.SessionId = uuid.New().String()
	global.RequestId = uuid.New().String()
}
