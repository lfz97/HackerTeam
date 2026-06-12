package main

import (
	"HackerTeam/bootstrap"
	"HackerTeam/global"
)

func main() {
	global.TuiInit(
		func() { bootstrap.Init("HackerTeam") },
		func() { bootstrap.AgentStart() },
	)
	global.TuiRun()
}
