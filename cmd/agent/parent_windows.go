//go:build windows

package main

import "golang.org/x/sys/windows"

// watchParent appelle stop quand le processus pid se termine, de quelque facon
// que ce soit, plantage compris. Windows ne tue pas les enfants d'un processus
// qui meurt : sans cette garde, l'agent lance par une application qui plante
// resterait en vie, garderait le port, et la prochaine ouverture de
// l'application tomberait sur lui sans pouvoir le remplacer.
func watchParent(pid int, stop func()) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return err
	}
	go func() {
		defer windows.CloseHandle(h)
		windows.WaitForSingleObject(h, windows.INFINITE)
		stop()
	}()
	return nil
}
