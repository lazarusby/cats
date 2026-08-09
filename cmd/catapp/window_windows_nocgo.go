//go:build windows && catapp_windows_nocgo

package main

// This adapter exists only for compile-checking Windows launcher/process logic
// without the native cgo/WebView2 toolchain. It is never a product build.
func newDesktopWindow(bool) desktopWindow {
	panic("catapp_windows_nocgo cannot create a desktop window")
}
