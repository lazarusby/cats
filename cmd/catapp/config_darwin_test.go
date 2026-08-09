//go:build darwin

package main

import (
	"path/filepath"
	"testing"
)

func TestDarwinAppDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := appDataDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Library", "Application Support", "cats")
	if got != want {
		t.Fatalf("appDataDir = %q, want %q", got, want)
	}
}
