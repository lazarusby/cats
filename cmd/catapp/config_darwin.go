//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDataDir returns the Darwin launcher settings directory. Daemon XDG state
// remains independent, so app packaging never disturbs sessions.
func appDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "cats"), nil
}
