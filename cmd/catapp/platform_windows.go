//go:build windows

package main

// Phase 2 makes common mode/window flow compile-ready without claiming Windows
// desktop parity. Phase 4 replaces these deliberately narrow no-op adapters.
func hydratePlatformEnvironment() {}
func installMenu(desktopWindow)   {}
func bindPlatformBridges(desktopWindow) error {
	return nil
}
