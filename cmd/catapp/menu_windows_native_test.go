//go:build windows && !catapp_windows_nocgo

package main

import (
	"testing"

	webview "github.com/webview/webview_go"
)

func TestWindowsAcceleratorMapping(t *testing.T) {
	tests := []struct {
		name  string
		event webview.WindowsKeyEvent
		want  int
	}{
		{"copy", webview.WindowsKeyEvent{VirtualKey: 'C', Modifiers: webview.WindowsModifierControl | webview.WindowsModifierShift}, winMenuCopy},
		{"paste", webview.WindowsKeyEvent{VirtualKey: 'V', Modifiers: webview.WindowsModifierControl | webview.WindowsModifierShift}, winMenuPaste},
		{"zoom plus", webview.WindowsKeyEvent{VirtualKey: 0xBB, Modifiers: webview.WindowsModifierControl | webview.WindowsModifierShift}, winMenuZoomIn},
		{"zoom minus", webview.WindowsKeyEvent{VirtualKey: 0xBD, Modifiers: webview.WindowsModifierControl}, winMenuZoomOut},
		{"zoom reset", webview.WindowsKeyEvent{VirtualKey: '0', Modifiers: webview.WindowsModifierControl}, winMenuZoomReset},
		{"help", webview.WindowsKeyEvent{VirtualKey: 0x70}, winMenuKeyboardHelp},
		{"plain terminal control", webview.WindowsKeyEvent{VirtualKey: 'C', Modifiers: webview.WindowsModifierControl}, 0},
		{"altgr copy-shaped", webview.WindowsKeyEvent{VirtualKey: 'C', Modifiers: webview.WindowsModifierControl | webview.WindowsModifierAlt | webview.WindowsModifierAltGraph}, 0},
		{"keyup", webview.WindowsKeyEvent{VirtualKey: 0xBB, Kind: 1, Modifiers: webview.WindowsModifierControl}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowsAcceleratorCommand(tc.event); got != tc.want {
				t.Fatalf("command = %d, want %d", got, tc.want)
			}
		})
	}
}
