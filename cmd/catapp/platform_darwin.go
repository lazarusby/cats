//go:build darwin

package main

// Darwin retains the common synchronous local launcher flow. Windows owns a
// persistent asynchronous starting/repair window around WSL startup.
func runPlatformLocal(appConfig) bool { return false }

func initPlatformDescriptor(w desktopWindow, nativeClipboard bool) {
	value := "false"
	if nativeClipboard {
		value = "true"
	}
	w.Init(`window.catsDesktop=Object.freeze({platform:"macos",nativeClipboard:` + value + `});`)
}
