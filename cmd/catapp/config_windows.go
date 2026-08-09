//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDataDir keeps Windows launcher selection/preferences outside WSL state.
func appDataDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not set")
	}
	return filepath.Join(local, "Cats"), nil
}
