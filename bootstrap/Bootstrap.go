package bootstrap

import (
	"HackerTeam/handler"
	"HackerTeam/tui/global_object"
	"HackerTeam/utils/pretty"
	"context"
	"fmt"
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
	sessionID, requestID := RandomStartID()
	for {
		EndTurn_p := handler.AgentRunIteratively(context.Background(), AgentRunner, sessionID, AgentRunner.UserId, requestID, MsgContext)
		if (*EndTurn_p).Code == handler.Exit { //用户主动结束对话，退出程序
			//关闭AgentRunner，释放资源
			done := make(chan struct{})
			AgentRunner.Runner.Close()
			global_object.App_p.QueueUpdateDraw(func() {
				fmt.Fprint(global_object.AgentMessageView_p, pretty.TExit("对话已结束，感谢使用！后会有期！"))
				global_object.AgentMessageView_p.ScrollToEnd()
				global_object.App_p.Stop()
			})
			<-done

		} else if (*EndTurn_p).Code == handler.New { //用户开始新对话，重置sessionID,  requestID，更新MsgContext为新对话的初始状态
			sessionID, requestID = RandomStartID()
			MsgContext = handler.TurnResult{
				Code:       handler.New,
				Reason:     "新对话",
				OutputPart: "",
			}

		} else if (*EndTurn_p).Code == handler.Flush { //用户主动刷新工具，保持sessionID, requestID不变，重新创建Runner
			MsgContext = handler.TurnResult{
				Code:       handler.Flush,
				Reason:     "用户主动刷新工具",
				OutputPart: "",
			}
			LoadConfig()               //重新加载配置文件，确保工具的最新状态被加载
			AgentRunner.Runner.Close() //关闭旧的Runner，释放资源
			AgentRunner = NewRunner()  //创建新的agent和Runner，使用最新的工具配置

		} else { //其他情况，继续使用当前的sessionID, userID, requestID，更新MsgContext为当前对话的结束状态，供下一轮对话使用
			MsgContext = *EndTurn_p
			continue
		}

	}
}

func RandomStartID() (string, string) {
	sessionID := uuid.New().String()
	requestID := uuid.New().String()
	return sessionID, requestID
}
