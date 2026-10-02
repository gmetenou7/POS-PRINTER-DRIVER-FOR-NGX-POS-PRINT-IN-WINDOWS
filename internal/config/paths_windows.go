//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// DefaultDataDir est le dossier de l'agent : journal, certificats, reglages et associations.
// La barre de notification le lit aussi, pour y trouver le jeton local.
func DefaultDataDir() string {
	return filepath.Join(programData(), "PrintBridge")
}

func defaultLogPath() string {
	return filepath.Join(programData(), "PrintBridge", "agent.log")
}

func defaultCertDir() string {
	return filepath.Join(programData(), "PrintBridge", "certs")
}

func programData() string {
	dir := os.Getenv("ProgramData")
	if dir == "" {
		dir = os.TempDir()
	}
	return dir
}
