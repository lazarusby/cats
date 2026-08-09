//go:build windows && !catapp_windows_nocgo

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
)

var shellExecuteProc = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

func openWindowsPath(path string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	target, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	result, _, callErr := shellExecuteProc.Call(0, uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("ShellExecuteW result %d: %w", result, callErr)
	}
	return nil
}

func bindPlatformBridges(window desktopWindow) error {
	native, ok := window.(*nativeWindow)
	if !ok {
		// Unit fakes exercise the binding names without owning a native controller.
		return bindWindowsClipboard(window)
	}
	// Any controller-hook failure must leave network navigation disabled. The
	// caller may still render a local error/setup document with SetHtml.
	native.prepareNavigation = func(string) error {
		return fmt.Errorf("native WebView2 navigation guard is unavailable")
	}
	policy := &windowsNavigationPolicy{}
	hooks, err := webview.InstallWindowsHooks(native.w,
		func(uri string, kind webview.WindowsNavigationKind) webview.WindowsNavigationAction {
			action := policy.decide(uri, kind == webview.WindowsNewWindow)
			switch action {
			case navigationAllow:
				return webview.WindowsNavigationAllow
			case navigationExternal:
				return webview.WindowsNavigationOpenExternal
			default:
				return webview.WindowsNavigationCancel
			}
		},
		func(event webview.WindowsKeyEvent) bool {
			command := windowsAcceleratorCommand(event)
			if command == 0 {
				return false
			}
			runWindowsMenuCommand(native, command)
			return true
		})
	if err != nil {
		return err
	}
	native.platformClosers = append(native.platformClosers, hooks.Close)
	native.prepareNavigation = func(rawURL string) error {
		if err := policy.trust(rawURL); err != nil {
			return fmt.Errorf("refuse untrusted navigation target: %w", err)
		}
		return nil
	}
	if err := bindWindowsClipboard(window); err != nil {
		hooks.Close()
		native.platformClosers = native.platformClosers[:len(native.platformClosers)-1]
		return err
	}
	return nil
}
