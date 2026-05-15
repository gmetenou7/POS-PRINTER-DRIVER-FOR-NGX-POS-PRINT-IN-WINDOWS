//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

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
