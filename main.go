package main

import (
	"HackerTeam/global"
	"HackerTeam/tui"
)

func main() {
	tui.TuiInit()
	if err := global.App_p.Run(); err != nil {
		panic(err)
	}
}
