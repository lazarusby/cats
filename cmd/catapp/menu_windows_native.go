//go:build windows && !catapp_windows_nocgo

package main

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
)

const (
	winMenuAbout = 1001 + iota
	winMenuQuit
	winMenuUndo
	winMenuRedo
	winMenuCut
	winMenuCopy
	winMenuPaste
	winMenuSelectAll
	winMenuZoomIn
	winMenuZoomOut
	winMenuZoomReset
	winMenuKeyboardHelp
)

const (
	mfString          = 0x0000
	mfPopup           = 0x0010
	mfSeparator       = 0x0800
	gwlpWndProc       = ^uintptr(3) // -4
	wmCommand         = 0x0111
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	mbOK              = 0x00000000
	mbIconInformation = 0x00000040
)

var (
	menuUser32       = syscall.NewLazyDLL("user32.dll")
	createMenuProc   = menuUser32.NewProc("CreateMenu")
	createPopupProc  = menuUser32.NewProc("CreatePopupMenu")
	appendMenuProc   = menuUser32.NewProc("AppendMenuW")
	setMenuProc      = menuUser32.NewProc("SetMenu")
	drawMenuBarProc  = menuUser32.NewProc("DrawMenuBar")
	destroyMenuProc  = menuUser32.NewProc("DestroyMenu")
	setWndProc       = menuUser32.NewProc("SetWindowLongPtrW")
	callWndProc      = menuUser32.NewProc("CallWindowProcW")
	postMessageProc  = menuUser32.NewProc("PostMessageW")
	sendMessageProc  = menuUser32.NewProc("SendMessageW")
	messageBoxProc   = menuUser32.NewProc("MessageBoxW")
	getMenuProc      = menuUser32.NewProc("GetMenu")
	showWindowProc   = menuUser32.NewProc("ShowWindow")
	windowsMenuState sync.Map
	windowsWndProc   = syscall.NewCallback(windowsWindowProcedure)
)

type windowsMenuRegistration struct {
	window   *nativeWindow
	hwnd     uintptr
	menu     uintptr
	original uintptr
}

func installMenu(window desktopWindow) {
	native, ok := window.(*nativeWindow)
	if !ok {
		return
	}
	registration, err := installWindowsMenu(native)
	if err != nil {
		return
	}
	native.platformClosers = append(native.platformClosers, registration.close)
}

func installWindowsMenu(window *nativeWindow) (*windowsMenuRegistration, error) {
	hwnd := uintptr(window.w.Window())
	if hwnd == 0 {
		return nil, fmt.Errorf("native window handle is unavailable")
	}
	menu, _, callErr := createMenuProc.Call()
	if menu == 0 {
		return nil, fmt.Errorf("CreateMenu: %w", callErr)
	}
	ok := false
	defer func() {
		if !ok {
			destroyMenuProc.Call(menu)
		}
	}()
	fileMenu, _, _ := createPopupProc.Call()
	editMenu, _, _ := createPopupProc.Call()
	viewMenu, _, _ := createPopupProc.Call()
	if fileMenu == 0 || editMenu == 0 || viewMenu == 0 {
		return nil, fmt.Errorf("CreatePopupMenu failed")
	}

	for _, item := range []struct {
		menu  uintptr
		flags uintptr
		id    uintptr
		label string
	}{
		{fileMenu, mfString, winMenuAbout, "&About Cats"},
		{fileMenu, mfSeparator, 0, ""},
		{fileMenu, mfString, winMenuQuit, "E&xit\tAlt+F4"},
		{editMenu, mfString, winMenuUndo, "&Undo\tCtrl+Shift+Z"},
		{editMenu, mfString, winMenuRedo, "&Redo\tCtrl+Shift+Y"},
		{editMenu, mfSeparator, 0, ""},
		{editMenu, mfString, winMenuCut, "Cu&t\tCtrl+Shift+X"},
		{editMenu, mfString, winMenuCopy, "&Copy\tCtrl+Shift+C"},
		{editMenu, mfString, winMenuPaste, "&Paste\tCtrl+Shift+V"},
		{editMenu, mfString, winMenuSelectAll, "Select &All\tCtrl+Shift+A"},
		{viewMenu, mfString, winMenuZoomIn, "&Bigger Text\tCtrl++"},
		{viewMenu, mfString, winMenuZoomOut, "&Smaller Text\tCtrl+-"},
		{viewMenu, mfString, winMenuZoomReset, "&Default Text Size\tCtrl+0"},
		{viewMenu, mfSeparator, 0, ""},
		{viewMenu, mfString, winMenuKeyboardHelp, "&Keyboard Shortcuts\tF1"},
	} {
		if err := appendWindowsMenuItem(item.menu, item.flags, item.id, item.label); err != nil {
			return nil, err
		}
	}
	for _, popup := range []struct {
		handle uintptr
		label  string
	}{{fileMenu, "&File"}, {editMenu, "&Edit"}, {viewMenu, "&View"}} {
		if err := appendWindowsMenuItem(menu, mfPopup, popup.handle, popup.label); err != nil {
			return nil, err
		}
	}
	if result, _, callErr := setMenuProc.Call(hwnd, menu); result == 0 {
		return nil, fmt.Errorf("SetMenu: %w", callErr)
	}
	drawMenuBarProc.Call(hwnd)
	original, _, callErr := setWndProc.Call(hwnd, gwlpWndProc, windowsWndProc)
	if original == 0 {
		setMenuProc.Call(hwnd, 0)
		return nil, fmt.Errorf("SetWindowLongPtrW: %w", callErr)
	}
	registration := &windowsMenuRegistration{window: window, hwnd: hwnd, menu: menu, original: original}
	windowsMenuState.Store(hwnd, registration)
	ok = true
	return registration, nil
}

func appendWindowsMenuItem(menu, flags, id uintptr, label string) error {
	var pointer *uint16
	if label != "" {
		pointer, _ = syscall.UTF16PtrFromString(label)
	}
	if result, _, callErr := appendMenuProc.Call(menu, flags, id, uintptr(unsafe.Pointer(pointer))); result == 0 {
		return fmt.Errorf("AppendMenuW(%q): %w", label, callErr)
	}
	return nil
}

func (r *windowsMenuRegistration) close() {
	if r == nil || r.hwnd == 0 {
		return
	}
	windowsMenuState.Delete(r.hwnd)
	setWndProc.Call(r.hwnd, gwlpWndProc, r.original)
	setMenuProc.Call(r.hwnd, 0)
	drawMenuBarProc.Call(r.hwnd)
	if r.menu != 0 {
		destroyMenuProc.Call(r.menu)
	}
	r.hwnd = 0
	r.menu = 0
}

func windowsWindowProcedure(hwnd, message, wParam, lParam uintptr) uintptr {
	value, ok := windowsMenuState.Load(hwnd)
	if !ok {
		return 0
	}
	registration := value.(*windowsMenuRegistration)
	switch message {
	case wmCommand:
		command := int(wParam & 0xffff)
		if runWindowsMenuCommand(registration.window, command) {
			return 0
		}
	case wmClose:
		runCleanup()
	case wmQueryEndSession:
		runCleanup()
		return 1
	case wmEndSession:
		if wParam != 0 {
			runCleanup()
		}
	}
	result, _, _ := callWndProc.Call(registration.original, hwnd, message, wParam, lParam)
	return result
}

func runWindowsMenuCommand(window *nativeWindow, command int) bool {
	switch command {
	case winMenuAbout:
		caption, _ := syscall.UTF16PtrFromString("About Cats")
		message, _ := syscall.UTF16PtrFromString("Cats for Windows\nNative WebView2 launcher with a WSL2 backend.")
		messageBoxProc.Call(uintptr(window.w.Window()), uintptr(unsafe.Pointer(message)),
			uintptr(unsafe.Pointer(caption)), mbOK|mbIconInformation)
	case winMenuQuit:
		runCleanup()
		postMessageProc.Call(uintptr(window.w.Window()), wmClose, 0, 0)
	case winMenuUndo:
		window.Eval(`document.execCommand("undo")`)
	case winMenuRedo:
		window.Eval(`document.execCommand("redo")`)
	case winMenuCut:
		window.Eval(`document.execCommand("cut")`)
	case winMenuCopy:
		window.Eval(`document.execCommand("copy")`)
	case winMenuPaste:
		window.Eval(`window.pasteText && window.pasteText()`)
	case winMenuSelectAll:
		window.Eval(`document.execCommand("selectAll")`)
	case winMenuZoomIn:
		zoomFont(1)
	case winMenuZoomOut:
		zoomFont(-1)
	case winMenuZoomReset:
		zoomFont(0)
	case winMenuKeyboardHelp:
		window.Eval(`window.openHelp && window.openHelp()`)
	default:
		return false
	}
	return true
}

func windowsAcceleratorCommand(event webview.WindowsKeyEvent) int {
	if event.Kind != 0 && event.Kind != 2 { // key-down and system-key-down only
		return 0
	}
	mods := event.Modifiers
	if mods&webview.WindowsModifierAltGraph != 0 || mods&webview.WindowsModifierWindows != 0 {
		return 0
	}
	if event.VirtualKey == 0x70 && mods == 0 { // F1
		return winMenuKeyboardHelp
	}
	if mods&webview.WindowsModifierControl == 0 || mods&webview.WindowsModifierAlt != 0 {
		return 0
	}
	shift := mods&webview.WindowsModifierShift != 0
	switch event.VirtualKey {
	case 0xBB, 0x6B: // OEM/numpad plus
		return winMenuZoomIn
	case 0xBD, 0x6D: // OEM/numpad minus
		return winMenuZoomOut
	case 0x30, 0x60: // top-row/numpad zero
		return winMenuZoomReset
	}
	if !shift {
		return 0
	}
	switch event.VirtualKey {
	case 'Z':
		return winMenuUndo
	case 'Y':
		return winMenuRedo
	case 'X':
		return winMenuCut
	case 'C':
		return winMenuCopy
	case 'V':
		return winMenuPaste
	case 'A':
		return winMenuSelectAll
	default:
		return 0
	}
}
