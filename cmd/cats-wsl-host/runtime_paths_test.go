//go:build linux

package main

import (
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func currentUID(t *testing.T) int {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(current.Uid)
	if err != nil {
		t.Fatal(err)
	}
	return uid
}

func TestAllocateRuntimeDirUsesPrivateXDG(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", root)
	paths, err := allocateRuntimeDir("launch-1", currentUID(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = paths.cleanup() })
	want := filepath.Join(root, "cats", "launch-1")
	if paths.dir != want {
		t.Fatalf("runtime dir = %q, want %q", paths.dir, want)
	}
	st, err := os.Stat(paths.dir)
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("runtime mode = %v, %v", st.Mode().Perm(), err)
	}
}

func TestAllocateRuntimeDirFallsBackFromInsecureXDG(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", root)
	paths, err := allocateRuntimeDir("launch-2", currentUID(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = paths.cleanup() })
	if strings.HasPrefix(paths.dir, root+string(filepath.Separator)) {
		t.Fatalf("used insecure XDG runtime dir: %s", paths.dir)
	}
	if !strings.Contains(filepath.Base(paths.dir), "cats-wsl-"+strconv.Itoa(currentUID(t))+"-") {
		t.Fatalf("fallback path lacks numeric uid: %s", paths.dir)
	}
}

func TestRuntimeCleanupRemovesRealUnixSockets(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	paths, err := allocateRuntimeDir("socket-test", currentUID(t))
	if err != nil {
		t.Fatal(err)
	}
	listeners := make([]net.Listener, 0, 3)
	for _, path := range []string{paths.th, paths.ctl, paths.hook} {
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
	}
	for _, listener := range listeners {
		_ = listener.Close()
	}
	if err := paths.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.dir); !os.IsNotExist(err) {
		t.Fatalf("runtime directory still exists: %v", err)
	}
}

func TestRuntimeCleanupRefusesUnsafePaths(t *testing.T) {
	for _, paths := range []runtimePaths{
		newRuntimePaths("/"),
		newRuntimePaths(os.TempDir()),
		newRuntimePaths("/home"),
		newRuntimePaths("relative"),
		{dir: t.TempDir(), th: "/tmp/unrelated", ctl: "/tmp/ctl", hook: "/tmp/hook"},
	} {
		if err := paths.cleanup(); err == nil {
			t.Fatalf("unsafe cleanup unexpectedly accepted: %#v", paths)
		}
	}
}

func TestRuntimeCleanupDoesNotRemoveUnexpectedFiles(t *testing.T) {
	dir := t.TempDir()
	paths := newRuntimePaths(dir)
	extra := filepath.Join(dir, "keep-me")
	if err := os.WriteFile(extra, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := paths.cleanup(); err == nil {
		t.Fatal("cleanup unexpectedly removed a non-empty runtime directory")
	}
	if data, err := os.ReadFile(extra); err != nil || string(data) != "user data" {
		t.Fatalf("unexpected file changed: data=%q err=%v", data, err)
	}
}

func TestRuntimeSocketPathBound(t *testing.T) {
	paths := newRuntimePaths("/" + strings.Repeat("a", maxUnixSocketPathBytes))
	if paths.socketPathsFit() {
		t.Fatal("oversized socket paths unexpectedly fit")
	}
}
