package main

import (
	"HackerTeam/tui"
	"HackerTeam/tui/global_object"
)

func main() {
	tui.TuiInit()
	if err := global_object.App_p.Run(); err != nil {
		panic(err)
	}
}
