//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	startupTimeout   = 10 * time.Second
	catwayStopGrace  = 5 * time.Second
	cathostStopGrace = 3 * time.Second
)

type session struct {
	opts         options
	payloadDir   string
	startDir     string
	env          []string
	paths        runtimePaths
	stderr       io.Writer
	control      <-chan controlEvent
	signals      <-chan os.Signal
	bindConflict func() bool

	startupLimit time.Duration
	catwayGrace  time.Duration
	cathostGrace time.Duration
}

type sessionResult struct {
	reason string
	stage  string
	err    error
}

func (s *session) run(onReady func(string) error) (result sessionResult) {
	startupLimit := durationOr(s.startupLimit, startupTimeout)
	catwayGrace := durationOr(s.catwayGrace, catwayStopGrace)
	cathostGrace := durationOr(s.cathostGrace, cathostStopGrace)

	cathost, err := startManagedChild(childSpec{
		path: filepath.Join(s.payloadDir, "cathost"),
		args: []string{"-persistent", "-socket", s.paths.th, "-idle-timeout", s.opts.idleTimeout.String()},
		dir:  s.startDir, env: s.env, stderr: s.stderr,
	})
	if err != nil {
		return sessionResult{reason: "startup_error", stage: "cathost", err: err}
	}
	supervisor := childSupervisor{catwayGrace: catwayGrace, cathostGrace: cathostGrace, cathost: cathost}
	defer func() {
		if err := supervisor.stop(); err != nil {
			shutdownErr := fmt.Errorf("ordered child teardown: %w", err)
			if result.err == nil {
				result = sessionResult{reason: "shutdown_error", stage: "shutdown", err: shutdownErr}
			} else {
				result.err = errors.Join(result.err, shutdownErr)
			}
		}
	}()

	catway, err := startManagedChild(childSpec{
		path: filepath.Join(s.payloadDir, "catway"),
		args: []string{
			"--addr", s.addr(), "--auth", "none",
			"--socket", s.paths.th,
			"--control-socket", s.paths.ctl,
			"--hook-socket", s.paths.hook,
		},
		dir: s.startDir, env: s.env, stderr: s.stderr,
	})
	if err != nil {
		return sessionResult{reason: "startup_error", stage: "catway", err: err}
	}
	supervisor.catway = catway

	readyTimer := time.NewTimer(startupLimit)
	defer readyTimer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if exited, err := childExited(catway); exited {
			return s.catwayStartupFailure(err)
		}
		if exited, err := childExited(cathost); exited {
			return sessionResult{reason: "child_exit", stage: "cathost", err: childExitError("cathost", err)}
		}
		if tcpReady(s.addr()) {
			// A listener that vanished in the same sampling window is not ready.
			if exited, err := childExited(catway); exited {
				return s.catwayStartupFailure(err)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-readyTimer.C:
			return sessionResult{reason: "startup_error", stage: "catway", err: fmt.Errorf("did not become ready at %s within %s", s.addr(), startupLimit)}
		case <-catway.done:
			return s.catwayStartupFailure(catway.err())
		case <-cathost.done:
			return sessionResult{reason: "child_exit", stage: "cathost", err: childExitError("cathost", cathost.err())}
		case event := <-s.control:
			return resultForControl(event)
		case signal := <-s.signals:
			return sessionResult{reason: signalReason(signal)}
		}
	}

	if err := onReady(s.addr()); err != nil {
		return sessionResult{reason: "protocol_error", stage: "protocol", err: fmt.Errorf("write ready record: %w", err)}
	}

	select {
	case event := <-s.control:
		return resultForControl(event)
	case signal := <-s.signals:
		return sessionResult{reason: signalReason(signal)}
	case <-catway.done:
		return sessionResult{reason: "child_exit", stage: "catway", err: childExitError("catway", catway.err())}
	case <-cathost.done:
		return sessionResult{reason: "child_exit", stage: "cathost", err: childExitError("cathost", cathost.err())}
	}
}

func (s *session) catwayStartupFailure(err error) sessionResult {
	stage := "catway"
	if s.bindConflict != nil && s.bindConflict() {
		stage = "bind"
	}
	return sessionResult{reason: "child_exit", stage: stage, err: childExitError("catway", err)}
}

func (s *session) addr() string { return fmt.Sprintf("127.0.0.1:%d", s.opts.port) }

func durationOr(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func resultForControl(event controlEvent) sessionResult {
	if event.err != nil {
		return sessionResult{reason: "protocol_error", stage: "protocol", err: event.err}
	}
	return sessionResult{reason: event.reason}
}

func signalReason(signal os.Signal) string {
	if signal == nil {
		return "signal"
	}
	return "signal_" + signal.String()
}

func tcpReady(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func childExited(child *managedChild) (bool, error) {
	select {
	case <-child.done:
		return true, child.err()
	default:
		return false, nil
	}
}

type childSpec struct {
	path   string
	args   []string
	dir    string
	env    []string
	stderr io.Writer
}

type managedChild struct {
	cmd  *exec.Cmd
	done chan struct{}

	mu      sync.Mutex
	waitErr error
}

func startManagedChild(spec childSpec) (*managedChild, error) {
	cmd := exec.Command(spec.path, spec.args...)
	cmd.Dir = spec.dir
	cmd.Env = spec.env
	cmd.Stdout = spec.stderr
	cmd.Stderr = spec.stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	child := &managedChild{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		child.mu.Lock()
		child.waitErr = err
		child.mu.Unlock()
		close(child.done)
	}()
	return child, nil
}

func (c *managedChild) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.waitErr
}

func (c *managedChild) terminate(grace time.Duration) error {
	if c == nil || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	select {
	case <-c.done:
		return c.err()
	default:
	}
	pid := c.cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-c.done:
		return nil
	case <-timer.C:
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		<-c.done
		return nil
	}
}

type childSupervisor struct {
	catwayGrace  time.Duration
	cathostGrace time.Duration
	catway       *managedChild
	cathost      *managedChild
	once         sync.Once
	err          error
}

func (s *childSupervisor) stop() error {
	s.once.Do(func() {
		var errs []error
		if err := s.catway.terminate(s.catwayGrace); err != nil {
			errs = append(errs, fmt.Errorf("stop catway: %w", err))
		}
		if err := s.cathost.terminate(s.cathostGrace); err != nil {
			errs = append(errs, fmt.Errorf("stop cathost: %w", err))
		}
		s.err = joinErrors(errs)
	})
	return s.err
}

func childExitError(name string, err error) error {
	if err == nil {
		return fmt.Errorf("%s exited unexpectedly", name)
	}
	return fmt.Errorf("%s exited unexpectedly: %w", name, err)
}

type childLogSignals struct {
	mu           sync.Mutex
	bindConflict bool
	tail         string
}

func (s *childLogSignals) Write(data []byte) (int, error) {
	const marker = "address already in use"
	s.mu.Lock()
	text := s.tail + strings.ToLower(string(data))
	if strings.Contains(text, marker) {
		s.bindConflict = true
	}
	if len(text) >= len(marker)-1 {
		s.tail = text[len(text)-(len(marker)-1):]
	} else {
		s.tail = text
	}
	s.mu.Unlock()
	return len(data), nil
}

func (s *childLogSignals) hasBindConflict() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindConflict
}
