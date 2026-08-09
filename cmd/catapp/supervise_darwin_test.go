//go:build darwin

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDarwinBackendURL(t *testing.T) {
	b := &backend{addr: "127.0.0.1:49152"}
	if got, want := b.URL(), "http://127.0.0.1:49152"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestWaitReady(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	defer listener.Close()
	if err := waitReady(addr, time.Second); err != nil {
		t.Fatalf("waitReady on listener: %v", err)
	}

	port, err := pickPort()
	if err != nil {
		t.Fatal(err)
	}
	if err := waitReady(fmt.Sprintf("127.0.0.1:%d", port), 10*time.Millisecond); err == nil {
		t.Fatal("waitReady unexpectedly succeeded on a closed port")
	}
}

func TestDarwinBackendStopsCatwayBeforeCathost(t *testing.T) {
	logPath := t.TempDir() + "/events.log"
	start := func(role string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDarwinStopHelperProcess$")
		cmd.Env = append(os.Environ(),
			"CATS_STOP_HELPER=1",
			"CATS_STOP_ROLE="+role,
			"CATS_STOP_LOG="+logPath)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s helper: %v", role, err)
		}
		return cmd
	}

	b := &backend{catway: start("catway"), cathost: start("cathost")}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	waitForLog(t, logPath, []string{"ready-catway", "ready-cathost"})
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	events := strings.Fields(string(data))
	catway := slices.Index(events, "stop-catway")
	cathost := slices.Index(events, "stop-cathost")
	if catway < 0 || cathost < 0 || catway >= cathost {
		t.Fatalf("stop events = %q, want catway before cathost", events)
	}
}

func TestDarwinStopHelperProcess(t *testing.T) {
	if os.Getenv("CATS_STOP_HELPER") != "1" {
		return
	}
	role := os.Getenv("CATS_STOP_ROLE")
	path := os.Getenv("CATS_STOP_LOG")
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	appendTestEvent(path, "ready-"+role)
	<-sigc
	appendTestEvent(path, "stop-"+role)
	os.Exit(0)
}

func appendTestEvent(path, event string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	_, err = fmt.Fprintln(f, event)
	_ = f.Close()
	if err != nil {
		os.Exit(2)
	}
}

func waitForLog(t *testing.T, path string, wants []string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			text := string(data)
			ready := true
			for _, want := range wants {
				ready = ready && strings.Contains(text, want)
			}
			if ready {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s", wants, path)
}
