//go:build linux

package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("CATS_WSL_FAKE_DAEMON") == "1" {
		runFakeDaemon()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSessionRequestedStopAndOrder(t *testing.T) {
	s := newTestSession(t, nil)
	control := make(chan controlEvent, 2)
	s.control = control
	ready := false
	result := s.run(func(addr string) error {
		ready = true
		if err := waitForFakeEvent(environmentValue(s.env, "CATS_WSL_EVENTS"), "ready-catway", time.Second); err != nil {
			return err
		}
		if !tcpReady(addr) {
			return fmt.Errorf("ready callback could not dial %s", addr)
		}
		control <- controlEvent{reason: "requested"}
		control <- controlEvent{reason: "requested"} // duplicate cannot trigger a second teardown
		return nil
	})
	if !ready || result.err != nil || result.reason != "requested" {
		t.Fatalf("ready=%v result=%#v", ready, result)
	}
	events := readEvents(t, environmentValue(s.env, "CATS_WSL_EVENTS"))
	catwayStop := slices.Index(events, "stop-catway")
	cathostStop := slices.Index(events, "stop-cathost")
	if catwayStop < 0 || cathostStop < 0 || catwayStop >= cathostStop {
		t.Fatalf("events = %q, want catway stop before cathost", events)
	}
}

func TestSessionOwnerEOF(t *testing.T) {
	control := make(chan controlEvent, 1)
	s := newTestSession(t, []string{"CATS_TEST_CASE=eof"})
	s.control = control
	result := s.run(func(string) error {
		control <- controlEvent{reason: "stdin_eof"}
		return nil
	})
	if result.err != nil || result.reason != "stdin_eof" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionSignal(t *testing.T) {
	signals := make(chan os.Signal, 1)
	s := newTestSession(t, nil)
	s.signals = signals
	result := s.run(func(string) error {
		signals <- syscall.SIGTERM
		return nil
	})
	if result.err != nil || result.reason != "signal_terminated" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionCatwayExitStopsCathost(t *testing.T) {
	s := newTestSession(t, []string{"CATS_FAKE_EXIT_AFTER_READY=catway"})
	result := s.run(func(string) error { return nil })
	if result.err == nil || result.reason != "child_exit" || result.stage != "catway" {
		data, _ := os.ReadFile(environmentValue(s.env, "CATS_WSL_EVENTS"))
		t.Fatalf("result = %#v (%v), events=%q", result, result.err, data)
	}
}

func TestSessionPartialStartupStopsCathost(t *testing.T) {
	s := newTestSession(t, nil)
	if err := os.Remove(filepath.Join(s.payloadDir, "catway")); err != nil {
		t.Fatal(err)
	}
	result := s.run(func(string) error { return nil })
	if result.err == nil || result.reason != "startup_error" || result.stage != "catway" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionReadinessTimeout(t *testing.T) {
	s := newTestSession(t, []string{"CATS_FAKE_NO_READY=catway"})
	s.startupLimit = 120 * time.Millisecond
	result := s.run(func(string) error { return nil })
	if result.err == nil || result.reason != "startup_error" || result.stage != "catway" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionUnresponsiveChildIsKilledWithinGrace(t *testing.T) {
	s := newTestSession(t, []string{"CATS_FAKE_UNRESPONSIVE=catway"})
	s.catwayGrace = 80 * time.Millisecond
	control := make(chan controlEvent, 1)
	s.control = control
	start := time.Now()
	result := s.run(func(string) error {
		control <- controlEvent{reason: "requested"}
		return nil
	})
	if result.err != nil || result.reason != "requested" {
		t.Fatalf("result = %#v", result)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("forced teardown took %s", elapsed)
	}
}

func TestProtocolErrorStopsChildren(t *testing.T) {
	control := make(chan controlEvent, 1)
	s := newTestSession(t, nil)
	s.control = control
	result := s.run(func(string) error {
		control <- controlEvent{reason: "protocol_error", err: fmt.Errorf("bad frame")}
		return nil
	})
	if result.err == nil || result.reason != "protocol_error" || result.stage != "protocol" {
		t.Fatalf("result = %#v", result)
	}
}

func TestChildLogSignalsClassifiesBindConflict(t *testing.T) {
	var signals childLogSignals
	_, _ = signals.Write([]byte("catway: listen tcp 127.0.0.1:42: bind: address al"))
	_, _ = signals.Write([]byte("ready in use\n"))
	if !signals.hasBindConflict() {
		t.Fatal("bind conflict was not classified")
	}
	s := session{bindConflict: signals.hasBindConflict}
	if result := s.catwayStartupFailure(errors.New("exit status 1")); result.stage != "bind" {
		t.Fatalf("startup stage = %q, want bind", result.stage)
	}
}

func newTestSession(t *testing.T, extraEnv []string) *session {
	t.Helper()
	payload := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"catway", "cathost", "catctl"} {
		if err := os.Symlink(self, filepath.Join(payload, name)); err != nil {
			t.Fatal(err)
		}
	}
	events := filepath.Join(t.TempDir(), "events.log")
	runtimeDir := t.TempDir()
	paths := newRuntimePaths(runtimeDir)
	port := freePort(t)
	control := make(chan controlEvent, 1)
	signals := make(chan os.Signal, 1)
	env := append(os.Environ(),
		"CATS_WSL_FAKE_DAEMON=1",
		"CATS_WSL_EVENTS="+events,
	)
	env = append(env, extraEnv...)
	return &session{
		opts:         options{port: port, idleTimeout: time.Minute},
		payloadDir:   payload,
		startDir:     t.TempDir(),
		env:          env,
		paths:        paths,
		stderr:       os.Stderr,
		control:      control,
		signals:      signals,
		catwayGrace:  500 * time.Millisecond,
		cathostGrace: 500 * time.Millisecond,
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func readEvents(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func waitForFakeEvent(path, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %q in %s", want, path)
}

func runFakeDaemon() {
	role := filepath.Base(os.Args[0])
	events := os.Getenv("CATS_WSL_EVENTS")
	appendFakeEvent(events, "start-"+role)
	if os.Getenv("CATS_FAKE_EXIT") == role {
		appendFakeEvent(events, "exit-"+role)
		os.Exit(17)
	}

	var listener net.Listener
	var err error
	switch role {
	case "cathost":
		socket := daemonArg("-socket", "--socket")
		listener, err = net.Listen("unix", socket)
	case "catway":
		if os.Getenv("CATS_FAKE_NO_READY") != role {
			listener, err = net.Listen("tcp", daemonArg("--addr"))
		}
	default:
		os.Exit(18)
	}
	if err != nil {
		appendFakeEvent(events, "listen-error-"+role)
		os.Exit(19)
	}
	if listener != nil {
		defer listener.Close()
	}
	appendFakeEvent(events, "ready-"+role)
	if os.Getenv("CATS_FAKE_EXIT_AFTER_READY") == role {
		time.Sleep(75 * time.Millisecond)
		appendFakeEvent(events, "exit-"+role)
		os.Exit(17)
	}

	if os.Getenv("CATS_FAKE_UNRESPONSIVE") == role {
		signal.Ignore(os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		select {}
	}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-sigc
	appendFakeEvent(events, "stop-"+role)
}

func daemonArg(names ...string) string {
	for index, arg := range os.Args {
		if slices.Contains(names, arg) && index+1 < len(os.Args) {
			return os.Args[index+1]
		}
	}
	return ""
}

func appendFakeEvent(path, event string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(20)
	}
	w := bufio.NewWriter(f)
	_, writeErr := fmt.Fprintln(w, event)
	flushErr := w.Flush()
	closeErr := f.Close()
	if writeErr != nil || flushErr != nil || closeErr != nil {
		os.Exit(21)
	}
}
