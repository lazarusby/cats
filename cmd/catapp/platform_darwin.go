//go:build darwin

package main

import "fmt"

// Darwin retains the common synchronous local launcher flow. Windows owns a
// persistent asynchronous starting/repair window around WSL startup.
func runPlatformLocal(appConfig) bool { return false }

func runInstallSmoke(appConfig) error {
	return fmt.Errorf("install smoke is available only in the Windows launcher")
}

func initPlatformDescriptor(w desktopWindow, nativeClipboard bool) {
	value := "false"
	if nativeClipboard {
		value = "true"
	}
	w.Init(`window.catsDesktop=Object.freeze({platform:"macos",nativeClipboard:` + value + `});`)
}
