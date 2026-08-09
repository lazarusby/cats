//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestWindowsAppDataDir(t *testing.T) {
	root := filepath.Join(`C:\`, "Users", "cats", "AppData", "Local")
	t.Setenv("LOCALAPPDATA", root)
	got, err := appDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "Cats"); got != want {
		t.Fatalf("appDataDir = %q, want %q", got, want)
	}
}

func TestWindowsAppDataDirRequiresLocalAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	if _, err := appDataDir(); err == nil {
		t.Fatal("appDataDir succeeded without LOCALAPPDATA")
	}
}
