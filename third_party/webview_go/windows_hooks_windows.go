//go:build windows

package webview

/*
#include <stdint.h>
#include "libs/webview/include/webview.h"

#ifdef __cplusplus
extern "C" {
#endif
void *webview_install_windows_hooks(webview_t w, uintptr_t go_handle,
                                    int *error_code);
void webview_remove_windows_hooks(void *hooks);
#ifdef __cplusplus
}
#endif
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"syscall"
	"unsafe"
)

// WindowsNavigationKind identifies the WebView2 event being decided.
type WindowsNavigationKind int

const (
	WindowsTopLevelNavigation WindowsNavigationKind = iota
	WindowsNewWindow
)

// WindowsNavigationAction is returned synchronously from a navigation handler.
type WindowsNavigationAction int

const (
	WindowsNavigationAllow WindowsNavigationAction = iota
	WindowsNavigationCancel
	WindowsNavigationOpenExternal
)

// WindowsKeyEvent is the subset of a WebView2 accelerator event needed by a
// desktop application. Modifiers are sampled while WebView2 delivers the key.
type WindowsKeyEvent struct {
	VirtualKey uint32
	Kind       uint32
	Modifiers  uint32
}

const (
	WindowsModifierShift = 1 << iota
	WindowsModifierControl
	WindowsModifierAlt
	WindowsModifierWindows
	WindowsModifierAltGraph
)

type windowsHookCallbacks struct {
	navigation  func(string, WindowsNavigationKind) WindowsNavigationAction
	accelerator func(WindowsKeyEvent) bool
}

// WindowsHooks owns native WebView2 event registrations. Close must run on the
// window's UI thread before the WebView is destroyed.
type WindowsHooks struct {
	native unsafe.Pointer
	handle cgo.Handle
}

// InstallWindowsHooks attaches top-level navigation, new-window, and
// accelerator callbacks to the controller already created by webview.New.
func InstallWindowsHooks(
	w WebView,
	navigation func(string, WindowsNavigationKind) WindowsNavigationAction,
	accelerator func(WindowsKeyEvent) bool,
) (*WindowsHooks, error) {
	concrete, ok := w.(*webview)
	if !ok || concrete.w == nil {
		return nil, fmt.Errorf("webview: Windows hooks require a live native WebView")
	}
	handle := cgo.NewHandle(windowsHookCallbacks{navigation: navigation, accelerator: accelerator})
	var code C.int
	native := C.webview_install_windows_hooks(concrete.w, C.uintptr_t(handle), &code)
	if native == nil {
		handle.Delete()
		return nil, fmt.Errorf("webview: install Windows hooks failed (HRESULT 0x%08x)", uint32(code))
	}
	return &WindowsHooks{native: native, handle: handle}, nil
}

// Close detaches callbacks and releases their Go handle. It is idempotent.
func (h *WindowsHooks) Close() {
	if h == nil || h.native == nil {
		return
	}
	C.webview_remove_windows_hooks(h.native)
	h.native = nil
	h.handle.Delete()
}

//export goWebviewWindowsNavigation
func goWebviewWindowsNavigation(rawHandle C.uintptr_t, rawURI *C.uint16_t, kind C.int) (action C.int) {
	action = C.int(WindowsNavigationCancel)
	defer func() { _ = recover() }()
	callbacks := cgo.Handle(rawHandle).Value().(windowsHookCallbacks)
	if callbacks.navigation == nil {
		return C.int(WindowsNavigationAllow)
	}
	return C.int(callbacks.navigation(utf16PointerString(rawURI), WindowsNavigationKind(kind)))
}

//export goWebviewWindowsAccelerator
func goWebviewWindowsAccelerator(rawHandle C.uintptr_t, virtualKey, kind, modifiers C.uint32_t) (handled C.int) {
	defer func() { _ = recover() }()
	callbacks := cgo.Handle(rawHandle).Value().(windowsHookCallbacks)
	if callbacks.accelerator != nil && callbacks.accelerator(WindowsKeyEvent{
		VirtualKey: uint32(virtualKey), Kind: uint32(kind), Modifiers: uint32(modifiers),
	}) {
		return 1
	}
	return 0
}

func utf16PointerString(pointer *C.uint16_t) string {
	if pointer == nil {
		return ""
	}
	const maxURIUnits = 1 << 20
	units := (*[maxURIUnits]uint16)(unsafe.Pointer(pointer))[:]
	for i, unit := range units {
		if unit == 0 {
			return syscall.UTF16ToString(units[:i])
		}
	}
	return ""
}
