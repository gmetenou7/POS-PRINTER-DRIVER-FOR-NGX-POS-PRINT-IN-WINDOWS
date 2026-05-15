//go:build windows

package config

import (
	"os"
	"path/filepath"
)

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
