package boot

import (
	"HackerTeam/service/engine"
	"HackerTeam/service/tui"
)

func Boot() {
	tui := tui.GetTuiService()
	go func() {
		e := engine.GetEngineService("HackerTeam", tui)
		e.AgentStart()
	}()
	tui.Run()

}
