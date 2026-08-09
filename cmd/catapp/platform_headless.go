//go:build catapp_headless && !darwin && !windows

package main

import (
	"context"
	"fmt"
)

// This adapter exists only so WSL/Linux CI can execute platform-neutral
// launcher tests. It is never part of a normal catapp build.
func newDesktopWindow(bool) desktopWindow { panic("headless catapp window requested") }
func appDataDir() (string, error)         { return "", fmt.Errorf("headless app data dir") }
func hydratePlatformEnvironment()         {}
func installMenu(desktopWindow)           {}
func bindPlatformBridges(desktopWindow) error {
	return nil
}
func installSignalHandler() {}
func startLocalBackend(context.Context, appConfig) (localBackend, error) {
	return nil, fmt.Errorf("headless local backend")
}
