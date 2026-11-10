package main

import (
	"fmt"
	"os"

	"HackerTeam/boot"
)

func main() {
	if err := boot.Boot(); err != nil {
		fmt.Fprintln(os.Stderr, "hackerteam:", err)
		os.Exit(1)
	}
}
