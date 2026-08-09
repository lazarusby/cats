//go:build catapp_headless && !darwin && !windows

package main

import (
	"context"
	"slices"
	"testing"
)

func TestLocalModeNavigatesAndCleansUpInOrder(t *testing.T) {
	var events []string
	backend := &fakeBackend{
		url: "http://127.0.0.1:49152",
		stop: func(context.Context) error {
			events = append(events, "backend-stop")
			return nil
		},
	}
	previousStarter := localBackendStarter
	localBackendStarter = func(context.Context, appConfig) (localBackend, error) {
		return backend, nil
	}
	t.Cleanup(func() { localBackendStarter = previousStarter })

	w := withHeadlessWindow(t, func(*fakeWebView) {
		events = append(events, "window-run")
	})
	w.onDestroy = func(*fakeWebView) { events = append(events, "window-destroy") }

	appCleanup = cleanupController{}
	t.Cleanup(func() { appCleanup = cleanupController{} })
	runLocal(appConfig{Mode: "local"})

	if got, want := w.url, backend.url; got != want {
		t.Fatalf("navigated to %q, want %q", got, want)
	}
	wantEvents := []string{"window-run", "window-destroy", "backend-stop"}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %q, want %q", events, wantEvents)
	}
}
