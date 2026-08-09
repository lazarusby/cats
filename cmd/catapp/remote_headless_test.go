//go:build catapp_headless && !darwin && !windows

package main

import (
	"strings"
	"testing"
)

func TestRemoteModeNavigatesSavedTargetWithoutLocalBackend(t *testing.T) {
	w := withHeadlessWindow(t, nil)
	runRemote(appConfig{Mode: "remote", Remote: remoteTarget{URL: "https://cats.example/session"}})
	if got, want := w.url, "https://cats.example/session"; got != want {
		t.Fatalf("navigated to %q, want %q", got, want)
	}
	if got, want := w.title, "cats — cats.example"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if !w.ran || !w.destroy {
		t.Fatalf("window lifecycle run=%v destroy=%v", w.ran, w.destroy)
	}
	if _, ok := w.bound["catsConnect"]; ok {
		t.Fatal("saved remote target unexpectedly installed first-run connect binding")
	}
}

func TestRemoteFirstRunConnectsInSameWindow(t *testing.T) {
	w := withHeadlessWindow(t, func(w *fakeWebView) {
		binding, ok := w.bound["catsConnect"].(func(string))
		if !ok {
			t.Fatal("catsConnect binding missing or has wrong type")
		}
		binding("  https://remote.example/cats  ")
	})
	runRemote(appConfig{Mode: "remote"})
	if !strings.Contains(w.html, "window.catsConnect(v)") {
		t.Fatal("first-run connect page was not installed")
	}
	if got, want := w.url, "https://remote.example/cats"; got != want {
		t.Fatalf("connect callback navigated to %q, want %q", got, want)
	}
	if got, want := w.title, "cats — remote.example"; got != want {
		t.Fatalf("connect callback title = %q, want %q", got, want)
	}
}

func withHeadlessWindow(t *testing.T, onRun func(*fakeWebView)) *fakeWebView {
	t.Helper()
	w := &fakeWebView{onRun: onRun}
	previous := desktopWindowFactory
	desktopWindowFactory = func(bool) desktopWindow { return w }
	t.Cleanup(func() { desktopWindowFactory = previous })
	return w
}
