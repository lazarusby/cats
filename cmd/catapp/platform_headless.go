//go:build catapp_headless && !darwin && !windows

package main

import (
	"context"
	"fmt"
	"os"
)

// This adapter exists only so WSL/Linux CI can execute platform-neutral
// launcher tests. It is never part of a normal catapp build.
func newDesktopWindow(bool) desktopWindow { panic("headless catapp window requested") }
func appDataDir() (string, error)         { return "", fmt.Errorf("headless app data dir") }
func hydratePlatformEnvironment()         {}
func runPlatformLocal(appConfig) bool     { return false }
func installMenu(desktopWindow)           {}
func initPlatformDescriptor(w desktopWindow, nativeClipboard bool) {
	w.Init(`window.catsDesktop=Object.freeze({platform:"headless",nativeClipboard:false});`)
}
func bindPlatformBridges(desktopWindow) error {
	return nil
}
func installSignalHandler()                         {}
func handleLocalBackendError(appConfig, error) bool { return false }
func replaceAppConfigFile(staged, destination string) error {
	return os.Rename(staged, destination)
}
func startLocalBackend(context.Context, appConfig) (localBackend, error) {
	return nil, fmt.Errorf("headless local backend")
}
