//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rohanthewiz/cats/internal/buildinfo"
	"github.com/rohanthewiz/cats/internal/desktopproto"
	"github.com/rohanthewiz/cats/internal/wslclient"
)

const (
	wslHealthTimeout  = 15 * time.Second
	wslStartupTimeout = 20 * time.Second
	wslHTTPTimeout    = 5 * time.Second
	wslStopTimeout    = 12 * time.Second
	wslStartupTries   = 3
	createNoWindow    = 0x08000000
)

type windowsBackend struct {
	url      string
	launchID string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	done     <-chan error
	log      io.WriteCloser
	once     sync.Once
	err      error
}

func (b *windowsBackend) URL() string { return b.url }

func (b *windowsBackend) Stop(ctx context.Context) error {
	b.once.Do(func() {
		if b.stdin != nil {
			if err := desktopproto.EncodeLauncher(b.stdin, desktopproto.NewStop()); err != nil {
				b.err = fmt.Errorf("request helper stop: %w", err)
			}
			_ = b.stdin.Close()
		}
		waitCtx, cancel := context.WithTimeout(ctx, wslStopTimeout)
		defer cancel()
		select {
		case err := <-b.done:
			if err != nil && b.err == nil {
				b.err = fmt.Errorf("wsl.exe exited: %w", err)
			}
		case <-waitCtx.Done():
			if b.cmd != nil && b.cmd.Process != nil {
				_ = b.cmd.Process.Kill()
			}
			<-b.done
			if b.err == nil {
				b.err = fmt.Errorf("wsl.exe did not stop within %s", wslStopTimeout)
			}
		}
		if b.log != nil {
			_ = b.log.Close()
		}
	})
	return b.err
}

type windowsStartupError struct {
	Stage        string
	Distribution string
	Version      string
	LogPath      string
	Err          error
}

type windowsTargetRequiredError struct {
	Distributions []string
	LogPath       string
	Cause         error
}

func (e *windowsTargetRequiredError) Error() string {
	return fmt.Sprintf("WSL target selection or repair is required: %v (log: %s)", e.Cause, e.LogPath)
}

func (e *windowsTargetRequiredError) Unwrap() error { return e.Cause }

func (e *windowsStartupError) Error() string {
	return fmt.Sprintf("stage: %s\ndistribution: %s\npayload version: %s\nlog: %s\n\n%v",
		e.Stage, e.Distribution, e.Version, e.LogPath, e.Err)
}

func (e *windowsStartupError) Unwrap() error { return e.Err }

func startLocalBackend(ctx context.Context, cfg appConfig) (localBackend, error) {
	wslPath, err := exec.LookPath("wsl.exe")
	if err != nil {
		return nil, &windowsStartupError{Stage: "wsl", Distribution: cfg.WSL.Distribution, Version: buildinfo.Version(), Err: errors.New("wsl.exe was not found; install or enable WSL2")}
	}
	logFile, logPath, err := openLauncherLog()
	if err != nil {
		return nil, &windowsStartupError{Stage: "log", Distribution: cfg.WSL.Distribution, Version: buildinfo.Version(), Err: err}
	}
	keepLogOpen := false
	defer func() {
		if !keepLogOpen {
			_ = logFile.Close()
		}
	}()

	distributions, err := listWSLDistributions(ctx, wslPath, logFile)
	if err != nil {
		return nil, startupError("distribution", cfg, logPath, err)
	}
	if err := cfg.WSL.Validate(); err != nil {
		return nil, &windowsTargetRequiredError{Distributions: distributions, LogPath: logPath, Cause: err}
	}
	if !slices.Contains(distributions, cfg.WSL.Distribution) {
		return nil, &windowsTargetRequiredError{Distributions: distributions, LogPath: logPath,
			Cause: fmt.Errorf("configured distribution %q is not installed", cfg.WSL.Distribution)}
	}
	resolvedTarget, err := resolveWSLPayloadTarget(ctx, wslPath, cfg.WSL, logFile)
	if err != nil {
		return nil, &windowsTargetRequiredError{Distributions: distributions, LogPath: logPath, Cause: err}
	}
	launchCfg := cfg
	launchCfg.WSL = resolvedTarget
	if err := checkWSLHealth(ctx, wslPath, launchCfg, logFile); err != nil {
		return nil, &windowsTargetRequiredError{Distributions: distributions, LogPath: logPath, Cause: err}
	}

	var lastErr error
	for attempt := 1; attempt <= wslStartupTries; attempt++ {
		backend, err := startWindowsAttempt(ctx, wslPath, launchCfg, logFile)
		if err == nil {
			backend.log = logFile
			keepLogOpen = true
			return backend, nil
		}
		lastErr = err
		if !retryableStartup(err) {
			break
		}
		fmt.Fprintf(logFile, "catapp: retryable startup failure on attempt %d/%d: %v\n", attempt, wslStartupTries, err)
	}
	return nil, startupError(errorStage(lastErr), cfg, logPath, lastErr)
}

// resolveWSLPayloadTarget pins a versioned directory once before health and
// launch. The installer stores the atomic `current` symlink in app.json, while
// Linux /proc/self/exe correctly reports the resolved sibling directory. Using
// the same resolved target for both operations makes those identities agree
// and prevents an upgrade from switching versions between the two operations.
func resolveWSLPayloadTarget(ctx context.Context, wslPath string, target wslclient.Target, stderr io.Writer) (wslclient.Target, error) {
	commandCtx, cancel := context.WithTimeout(ctx, wslHealthTimeout)
	defer cancel()
	output, err := runBoundedCommand(commandCtx, stderr, wslPath,
		"--distribution", target.Distribution,
		"--user", target.User,
		"--exec", "/usr/bin/readlink", "-f", "--", target.PayloadPath)
	if err != nil {
		return wslclient.Target{}, fmt.Errorf("resolve WSL payload current link: %w", err)
	}
	resolved := strings.TrimSpace(string(output))
	if err := wslclient.ValidateLinuxPath("resolved payload path", resolved); err != nil {
		return wslclient.Target{}, err
	}
	result := target
	result.PayloadPath = resolved
	if err := result.Validate(); err != nil {
		return wslclient.Target{}, err
	}
	return result, nil
}

func startupError(stage string, cfg appConfig, logPath string, err error) error {
	return &windowsStartupError{
		Stage: stage, Distribution: cfg.WSL.Distribution,
		Version: buildinfo.Version(), LogPath: logPath, Err: err,
	}
}

func listWSLDistributions(ctx context.Context, wslPath string, stderr io.Writer) ([]string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, wslHealthTimeout)
	defer cancel()
	output, err := runBoundedCommand(commandCtx, stderr, wslPath, "--list", "--quiet")
	if err != nil {
		return nil, fmt.Errorf("list WSL distributions: %w", err)
	}
	return wslclient.DecodeDistributionList(output)
}

func checkWSLHealth(ctx context.Context, wslPath string, cfg appConfig, stderr io.Writer) error {
	commandCtx, cancel := context.WithTimeout(ctx, wslHealthTimeout)
	defer cancel()
	output, err := runBoundedCommand(commandCtx, stderr, wslPath, wslclient.HealthArgs(cfg.WSL)...)
	if err != nil {
		return fmt.Errorf("run helper health: %w", err)
	}
	health, err := wslclient.DecodeHelperHealth(output)
	if err != nil {
		return err
	}
	return health.Validate(cfg.WSL, buildinfo.Version(), "amd64")
}

func startWindowsAttempt(ctx context.Context, wslPath string, cfg appConfig, stderr io.Writer) (*windowsBackend, error) {
	port, err := pickWindowsPort()
	if err != nil {
		return nil, err
	}
	launchID, err := newLaunchID()
	if err != nil {
		return nil, err
	}
	args, err := wslclient.LaunchArgs(cfg.WSL, port, launchID, "")
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, wslPath, args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open helper stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open helper stdout: %w", err)
	}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start wsl.exe: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	cleanup := func() {
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(wslStopTimeout):
			_ = command.Process.Kill()
			<-done
		}
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	startupCtx, cancel := context.WithTimeout(ctx, wslStartupTimeout)
	startup, err := wslclient.AwaitStartup(startupCtx, stdout, buildinfo.Version(), addr)
	cancel()
	if err != nil {
		cleanup()
		return nil, err
	}
	baseURL := "http://" + startup.Addr
	httpCtx, cancel := context.WithTimeout(ctx, wslHTTPTimeout)
	httpClient := &http.Client{
		Timeout: wslHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	err = wslclient.VerifyBackend(httpCtx, httpClient, baseURL, buildinfo.Version())
	cancel()
	if err != nil {
		_ = desktopproto.EncodeLauncher(stdin, desktopproto.NewStop())
		cleanup()
		return nil, err
	}
	return &windowsBackend{url: baseURL, launchID: launchID, cmd: command, stdin: stdin, done: done}, nil
}

func runBoundedCommand(ctx context.Context, stderr io.Writer, name string, args ...string) ([]byte, error) {
	var stdout limitedBuffer
	command := exec.CommandContext(ctx, name, args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	command.Stdout = &stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, err
	}
	if stdout.overflow {
		return nil, errors.New("wsl.exe output exceeded 16 KiB")
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := wslclient.MaxOutputBytes - b.Len()
	if remaining < len(data) {
		b.overflow = true
		if remaining < 0 {
			remaining = 0
		}
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return original, nil
}

func pickWindowsPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve Windows loopback port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func newLaunchID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate launch id: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func retryableStartup(err error) bool {
	var startup *wslclient.StartupError
	if errors.As(err, &startup) {
		return startup.Retryable
	}
	var verification *wslclient.VerificationError
	return errors.As(err, &verification) && verification.Retryable
}

func errorStage(err error) string {
	var startup *wslclient.StartupError
	if errors.As(err, &startup) {
		return startup.Stage
	}
	var verification *wslclient.VerificationError
	if errors.As(err, &verification) {
		return verification.Stage
	}
	return "startup"
}

func openLauncherLog() (io.WriteCloser, string, error) {
	dir, err := appDataDir()
	if err != nil {
		return nil, "", err
	}
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(logDir, "launcher.log")
	if info, err := os.Stat(path); err == nil && info.Size() >= 1<<20 {
		previous := path + ".1"
		_ = os.Remove(previous)
		_ = os.Rename(path, previous)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, "", err
	}
	return &redactingLog{file: file}, path, nil
}

type redactingLog struct {
	file    *os.File
	mu      sync.Mutex
	pending []byte
}

func (w *redactingLog) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(data)
	w.pending = append(w.pending, data...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		if err := w.writeRedacted(w.pending[:newline+1]); err != nil {
			return 0, err
		}
		w.pending = w.pending[newline+1:]
	}
	if len(w.pending) > 64<<10 {
		if err := w.writeRedacted(w.pending); err != nil {
			return 0, err
		}
		w.pending = w.pending[:0]
	}
	return original, nil
}

func (w *redactingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		if err := w.writeRedacted(w.pending); err != nil {
			_ = w.file.Close()
			return err
		}
		w.pending = nil
	}
	return w.file.Close()
}

func (w *redactingLog) writeRedacted(data []byte) error {
	_, err := w.file.Write(wslclient.RedactLogChunk(data))
	return err
}
