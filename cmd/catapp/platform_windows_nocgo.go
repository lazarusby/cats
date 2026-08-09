//go:build windows && catapp_windows_nocgo

package main

import "fmt"

func installMenu(desktopWindow) {}

func bindPlatformBridges(desktopWindow) error { return nil }

func openWindowsPath(string) error { return fmt.Errorf("native Windows shell is unavailable") }
