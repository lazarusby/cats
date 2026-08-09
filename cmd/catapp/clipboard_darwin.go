//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Native pasteboard access for the webview page. WKWebView restricts
// navigator.clipboard — reads resolve empty and writes demand a user
// activation that WebSocket-driven copies (OSC 52 from a pane, §7 reads)
// never have — so catapp binds these into the page (see newWindow) and the
// UI prefers them over the browser API when present. pbcopy/pbpaste are
// used instead of NSPasteboard to keep this file cgo-free.

func clipboardWrite(text string) error {
	cmd := exec.Command("/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func clipboardRead() (string, error) {
	out, err := exec.Command("/usr/bin/pbpaste").Output()
	return string(out), err
}

// bindPlatformBridges preserves the existing JavaScript API while keeping its
// Darwin implementation out of common window flow. Attempt both bindings so a
// failure in one direction does not hide the other.
func bindPlatformBridges(w desktopWindow) error {
	var errs []error
	if err := w.Bind("catsClipWrite", clipboardWrite); err != nil {
		errs = append(errs, fmt.Errorf("clipboard write: %w", err))
	}
	if err := w.Bind("catsClipRead", clipboardRead); err != nil {
		errs = append(errs, fmt.Errorf("clipboard read: %w", err))
	}
	return errors.Join(errs...)
}
