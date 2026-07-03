package main

import (
	"HackerTeam/bootstrap"
	"HackerTeam/global"
)

func main() {
	global.PageCreate()
	global.AgentEngineRun(func() { bootstrap.Init("HackerTeam") },
		func() { bootstrap.AgentStart() })

	global.TuiRun()
}
