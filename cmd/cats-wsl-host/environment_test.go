//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChooseShell(t *testing.T) {
	dir := t.TempDir()
	notExecutable := filepath.Join(dir, "not-executable")
	executable := filepath.Join(dir, "shell")
	if err := os.WriteFile(notExecutable, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte(""), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := chooseShell("relative", notExecutable, executable); got != executable {
		t.Fatalf("chooseShell = %q, want %q", got, executable)
	}
	if got := chooseShell("relative", notExecutable); got != "/bin/sh" {
		t.Fatalf("chooseShell fallback = %q", got)
	}
}

func TestEnvironmentReplacement(t *testing.T) {
	env := []string{"PATH=/bin", "USER=old", "USER=duplicate", "KEEP=yes"}
	env = setEnvironment(env, "USER", "alice")
	if got := environmentValue(env, "USER"); got != "alice" {
		t.Fatalf("USER = %q", got)
	}
	if got := strings.Join(env, "|"); strings.Count(got, "USER=") != 1 || !strings.Contains(got, "KEEP=yes") {
		t.Fatalf("environment = %q", got)
	}
}

func TestLoginShellPATHMarker(t *testing.T) {
	home := t.TempDir()
	identity := userIdentity{home: home, shell: "/bin/sh"}
	env := []string{"HOME=" + home, "PATH=/bin:/usr/bin"}
	got, err := loginShellPATH(identity, env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/bin") {
		t.Fatalf("PATH = %q", got)
	}
}
