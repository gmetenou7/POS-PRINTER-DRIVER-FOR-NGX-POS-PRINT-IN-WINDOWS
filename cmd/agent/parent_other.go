//go:build !windows

package main

import (
	"os"
	"time"
)

// watchParent appelle stop quand le processus pid n'est plus le parent de
// l'agent : a sa mort, l'agent est rattache a un autre processus.
func watchParent(pid int, stop func()) error {
	go func() {
		for os.Getppid() == pid {
			time.Sleep(2 * time.Second)
		}
		stop()
	}()
	return nil
}
