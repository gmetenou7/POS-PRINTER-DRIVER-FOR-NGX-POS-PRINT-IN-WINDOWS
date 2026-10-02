//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

// DefaultDataDir est le dossier de l'agent : journal, certificats, reglages et associations.
// La barre de notification le lit aussi, pour y trouver le jeton local.
func DefaultDataDir() string {
	return filepath.Join(cacheDir(), "print-bridge")
}

func defaultLogPath() string {
	return filepath.Join(cacheDir(), "print-bridge", "agent.log")
}

func defaultCertDir() string {
	return filepath.Join(cacheDir(), "print-bridge", "certs")
}

func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return os.TempDir()
	}
	return dir
}
