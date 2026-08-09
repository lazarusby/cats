//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"testing"

	"github.com/rohanthewiz/cats/internal/wslclient"
)

var (
	windowsWSLIntegration = flag.Bool("cats-wsl-integration", false, "run the opt-in Windows/WSL backend integration test")
	windowsWSLDistro      = flag.String("cats-wsl-distro", "", "integration-test WSL distribution")
	windowsWSLUser        = flag.String("cats-wsl-user", "", "integration-test Linux user")
	windowsWSLPayload     = flag.String("cats-wsl-payload", "", "integration-test absolute Linux payload path")
)

func TestWindowsRetryClassification(t *testing.T) {
	if !retryableStartup(&wslclient.StartupError{Stage: "bind", Retryable: true, Err: errors.New("busy")}) {
		t.Fatal("bind conflict must be retryable")
	}
	if retryableStartup(&wslclient.StartupError{Stage: "version", Err: errors.New("mismatch")}) {
		t.Fatal("version mismatch must not be retryable")
	}
}

func TestWindowsWSLBackendIntegration(t *testing.T) {
	if !*windowsWSLIntegration {
		t.Skip("pass -cats-wsl-integration on a qualified Windows/WSL host")
	}
	target := wslclient.Target{
		Distribution: *windowsWSLDistro,
		User:         *windowsWSLUser,
		PayloadPath:  *windowsWSLPayload,
	}
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	backend, err := startLocalBackend(context.Background(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	if backend.URL() == "" {
		t.Fatal("backend returned an empty URL")
	}
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsLimitedBuffer(t *testing.T) {
	var buffer limitedBuffer
	data := make([]byte, wslclient.MaxOutputBytes+1)
	if n, err := buffer.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if !buffer.overflow || buffer.Len() != wslclient.MaxOutputBytes {
		t.Fatalf("overflow=%v len=%d", buffer.overflow, buffer.Len())
	}
}

func TestWindowsClipboardBindingNames(t *testing.T) {
	w := &fakeWebView{}
	if err := bindWindowsClipboard(w); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"catsClipWrite", "catsClipRead"} {
		if _, ok := w.bound[name]; !ok {
			t.Fatalf("binding %q missing", name)
		}
	}
}

func TestWindowsPrivilegedBridgesRemainGated(t *testing.T) {
	w := &fakeWebView{}
	if err := bindPlatformBridges(w); err != nil {
		t.Fatal(err)
	}
	if len(w.bindings) != 0 {
		t.Fatalf("privileged bindings exposed before navigation guard: %v", w.bindings)
	}
}
