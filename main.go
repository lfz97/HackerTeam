package main

import (
	"HackerTeam/bootstrap"
	"HackerTeam/global"
)

func main() {
	global.Frontendinit()
	global.Backendinit(func() { bootstrap.Init("HackerTeam") },
		func() { bootstrap.AgentStart() })

	global.TuiRun()
}
