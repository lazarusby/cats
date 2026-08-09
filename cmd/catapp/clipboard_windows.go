//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	cfUnicodeText    = 13
	gmemMoveable     = 0x0002
	clipboardTries   = 8
	clipboardBackoff = 15 * time.Millisecond
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	openClipboardProc     = user32.NewProc("OpenClipboard")
	closeClipboardProc    = user32.NewProc("CloseClipboard")
	emptyClipboardProc    = user32.NewProc("EmptyClipboard")
	getClipboardDataProc  = user32.NewProc("GetClipboardData")
	setClipboardDataProc  = user32.NewProc("SetClipboardData")
	isClipboardFormatProc = user32.NewProc("IsClipboardFormatAvailable")
	globalAllocProc       = kernel32.NewProc("GlobalAlloc")
	globalFreeProc        = kernel32.NewProc("GlobalFree")
	globalLockProc        = kernel32.NewProc("GlobalLock")
	globalUnlockProc      = kernel32.NewProc("GlobalUnlock")
	globalSizeProc        = kernel32.NewProc("GlobalSize")
)

func bindWindowsClipboard(w desktopWindow) error {
	var errs []error
	if err := w.Bind("catsClipWrite", func(text string) error { return writeWindowsClipboard(text) }); err != nil {
		errs = append(errs, fmt.Errorf("bind catsClipWrite: %w", err))
	}
	if err := w.Bind("catsClipRead", func() (string, error) { return readWindowsClipboard() }); err != nil {
		errs = append(errs, fmt.Errorf("bind catsClipRead: %w", err))
	}
	return errors.Join(errs...)
}

func writeWindowsClipboard(text string) error {
	encoded, err := encodeClipboardUTF16(text)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openWindowsClipboard(); err != nil {
		return err
	}
	defer closeClipboardProc.Call()
	if result, _, callErr := emptyClipboardProc.Call(); result == 0 {
		return fmt.Errorf("EmptyClipboard: %w", callErr)
	}

	bytes := uintptr(len(encoded) * 2)
	handle, _, callErr := globalAllocProc.Call(gmemMoveable, bytes)
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc: %w", callErr)
	}
	owned := true
	defer func() {
		if owned {
			globalFreeProc.Call(handle)
		}
	}()
	pointer, _, callErr := globalLockProc.Call(handle)
	if pointer == 0 {
		return fmt.Errorf("GlobalLock: %w", callErr)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), len(encoded)), encoded)
	globalUnlockProc.Call(handle)
	if result, _, callErr := setClipboardDataProc.Call(cfUnicodeText, handle); result == 0 {
		return fmt.Errorf("SetClipboardData: %w", callErr)
	}
	owned = false // the clipboard owns the HGLOBAL after SetClipboardData
	return nil
}

func readWindowsClipboard() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openWindowsClipboard(); err != nil {
		return "", err
	}
	defer closeClipboardProc.Call()
	if available, _, _ := isClipboardFormatProc.Call(cfUnicodeText); available == 0 {
		return "", nil
	}
	handle, _, callErr := getClipboardDataProc.Call(cfUnicodeText)
	if handle == 0 {
		return "", fmt.Errorf("GetClipboardData: %w", callErr)
	}
	size, _, callErr := globalSizeProc.Call(handle)
	if size == 0 || size%2 != 0 || size > uintptr((maxClipboardUTF16Units+1)*2) {
		return "", fmt.Errorf("GlobalSize returned invalid CF_UNICODETEXT size %d: %w", size, callErr)
	}
	pointer, _, callErr := globalLockProc.Call(handle)
	if pointer == 0 {
		return "", fmt.Errorf("GlobalLock: %w", callErr)
	}
	defer globalUnlockProc.Call(handle)
	encoded := append([]uint16(nil), unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), int(size/2))...)
	return decodeClipboardUTF16(encoded)
}

func openWindowsClipboard() error {
	var last error
	for attempt := 0; attempt < clipboardTries; attempt++ {
		if result, _, callErr := openClipboardProc.Call(0); result != 0 {
			return nil
		} else {
			last = callErr
		}
		if !errors.Is(last, syscall.ERROR_ACCESS_DENIED) {
			break
		}
		if attempt+1 < clipboardTries {
			time.Sleep(clipboardBackoff)
		}
	}
	return fmt.Errorf("OpenClipboard after %d attempts: %w", clipboardTries, last)
}
