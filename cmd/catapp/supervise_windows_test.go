//go:build windows

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/internal/wslclient"
)

const createBreakawayFromJob = 0x01000000

var (
	windowsWSLIntegration = flag.Bool("cats-wsl-integration", false, "run the opt-in Windows/WSL backend integration test")
	windowsWSLDistro      = flag.String("cats-wsl-distro", "", "integration-test WSL distribution")
	windowsWSLUser        = flag.String("cats-wsl-user", "", "integration-test Linux user")
	windowsWSLPayload     = flag.String("cats-wsl-payload", "", "integration-test absolute Linux payload path")
	windowsWSLDestructive = flag.Bool("cats-wsl-destructive-integration", false,
		"run owner-kill, wsl --terminate, and wsl --shutdown lifecycle qualification")
)

func integrationWSLTarget() wslclient.Target {
	return wslclient.Target{
		Distribution: *windowsWSLDistro,
		User:         *windowsWSLUser,
		PayloadPath:  *windowsWSLPayload,
	}
}

func TestWindowsRetryClassification(t *testing.T) {
	if !retryableStartup(&wslclient.StartupError{Stage: "bind", Retryable: true, Err: errors.New("busy")}) {
		t.Fatal("bind conflict must be retryable")
	}
	if retryableStartup(&wslclient.StartupError{Stage: "version", Err: errors.New("mismatch")}) {
		t.Fatal("version mismatch must not be retryable")
	}
}

func TestWindowsWSLBackendIntegration(t *testing.T) {
	if !*windowsWSLIntegration {
		t.Skip("pass -cats-wsl-integration on a qualified Windows/WSL host")
	}
	target := integrationWSLTarget()
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	backend, err := startLocalBackend(context.Background(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	if backend.URL() == "" {
		t.Fatal("backend returned an empty URL")
	}
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsLimitedBuffer(t *testing.T) {
	var buffer limitedBuffer
	data := make([]byte, wslclient.MaxOutputBytes+1)
	if n, err := buffer.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if !buffer.overflow || buffer.Len() != wslclient.MaxOutputBytes {
		t.Fatalf("overflow=%v len=%d", buffer.overflow, buffer.Len())
	}
}

func TestWindowsClipboardBindingNames(t *testing.T) {
	w := &fakeWebView{}
	if err := bindWindowsClipboard(w); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"catsClipWrite", "catsClipRead"} {
		if _, ok := w.bound[name]; !ok {
			t.Fatalf("binding %q missing", name)
		}
	}
}

func TestWindowsPrivilegedBridgesBindAfterNavigationGuard(t *testing.T) {
	w := &fakeWebView{}
	if err := bindPlatformBridges(w); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"catsClipWrite", "catsClipRead"} {
		if _, ok := w.bound[name]; !ok {
			t.Fatalf("binding %q missing after guard installation path", name)
		}
	}
}

func TestWindowsLocalUsesPersistentProgressWindow(t *testing.T) {
	window := &fakeWebView{}
	navigated := make(chan string, 1)
	window.onNavigate = func(url string) { navigated <- url }
	window.onRun = func(*fakeWebView) {
		select {
		case <-navigated:
		case <-time.After(2 * time.Second):
			t.Error("startup window never navigated to backend")
		}
	}
	stopped := false
	previousFactory := desktopWindowFactory
	previousStarter := localBackendStarter
	desktopWindowFactory = func(bool) desktopWindow { return window }
	localBackendStarter = func(context.Context, appConfig) (localBackend, error) {
		return &fakeBackend{url: "http://127.0.0.1:8421", stop: func(context.Context) error {
			stopped = true
			return nil
		}}, nil
	}
	appCleanup = cleanupController{}
	t.Cleanup(func() {
		desktopWindowFactory = previousFactory
		localBackendStarter = previousStarter
		appCleanup = cleanupController{}
	})
	runWindowsLocal(appConfig{Mode: "local"})
	if !strings.Contains(window.html, "Starting cats") {
		t.Fatalf("persistent starting page was not shown: %q", window.html)
	}
	for _, name := range []string{"catsRetry", "catsRepair", "catsChangeWSL", "catsOpenLogs", "catsSelectWSL"} {
		if _, ok := window.bound[name]; !ok {
			t.Errorf("startup action %q was not bound", name)
		}
	}
	if !stopped {
		t.Fatal("backend was not cleaned after startup window closed")
	}
}

func TestWindowsWSLDisruptionIntegration(t *testing.T) {
	if !*windowsWSLDestructive {
		t.Skip("pass -cats-wsl-destructive-integration on an idle qualified WSL host")
	}
	target := integrationWSLTarget()
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, disruption := range []struct {
		name string
		args []string
	}{
		{"terminate distribution", []string{"--terminate", target.Distribution}},
		{"shutdown WSL", []string{"--shutdown"}},
	} {
		t.Run(disruption.name, func(t *testing.T) {
			backend, err := startLocalBackend(context.Background(), appConfig{Mode: "local", WSL: target})
			if err != nil {
				t.Fatal(err)
			}
			url := backend.URL()
			command := exec.Command("wsl.exe", disruption.args...)
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("wsl.exe %v: %v: %s", disruption.args, err, output)
			}
			_ = backend.Stop(context.Background()) // proxy failure is expected after forced distro loss
			waitBackendUnavailable(t, url)
			waitNoWSLPayloadProcesses(t, target)
		})
	}
}

func TestWindowsWSLOwnerLossIntegration(t *testing.T) {
	if !*windowsWSLDestructive {
		t.Skip("pass -cats-wsl-destructive-integration on an idle qualified WSL host")
	}
	target := integrationWSLTarget()
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestWindowsWSLOwnerLossChild$")
	command.Env = append(os.Environ(),
		"CATS_OWNER_LOSS_CHILD=1",
		"CATS_OWNER_LOSS_DISTRO="+target.Distribution,
		"CATS_OWNER_LOSS_USER="+target.User,
		"CATS_OWNER_LOSS_PAYLOAD="+target.PayloadPath)
	// Test runners commonly put the test process in a kill-on-close Job Object.
	// Break the simulated desktop owner out so killing it models Task Manager
	// rather than having the harness kill its wsl.exe descendant as well.
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, CreationFlags: createNoWindow | createBreakawayFromJob,
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	type ownerReady struct{ url, launchID string }
	ready := make(chan ownerReady, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line, ok := strings.CutPrefix(scanner.Text(), "CATS_OWNER_READY "); ok {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					ready <- ownerReady{url: fields[0], launchID: fields[1]}
				}
				return
			}
		}
	}()
	var child ownerReady
	select {
	case child = <-ready:
	case <-time.After(30 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("owner child did not become ready: %s", stderr.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waitBackendUnavailable(t, child.url)
	waitNoWSLPayloadProcesses(t, target)
	waitWSLRuntimeRemoved(t, target, child.launchID)
}

func TestWindowsWSLOwnerLossChild(t *testing.T) {
	if os.Getenv("CATS_OWNER_LOSS_CHILD") != "1" {
		t.Skip("owner-loss subprocess only")
	}
	target := wslclient.Target{
		Distribution: os.Getenv("CATS_OWNER_LOSS_DISTRO"),
		User:         os.Getenv("CATS_OWNER_LOSS_USER"),
		PayloadPath:  os.Getenv("CATS_OWNER_LOSS_PAYLOAD"),
	}
	backend, err := startLocalBackend(context.Background(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	concrete := backend.(*windowsBackend)
	fmt.Printf("CATS_OWNER_READY %s %s\n", backend.URL(), concrete.launchID)
	select {}
}

func waitBackendUnavailable(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequest(http.MethodGet, url, nil)
		response, err := client.Do(request)
		if err != nil {
			return
		}
		_ = response.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("backend %s remained reachable after owner/distribution loss", url)
}

func waitNoWSLPayloadProcesses(t *testing.T, target wslclient.Target) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		for _, name := range []string{"cats-wsl-host", "catway", "cathost"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			output, err := exec.CommandContext(ctx, "wsl.exe", "--distribution", target.Distribution,
				"--user", target.User, "--exec", "/usr/bin/pgrep", "-f", target.PayloadPath+"/"+name).Output()
			cancel()
			if err == nil && len(strings.TrimSpace(string(output))) != 0 {
				found = true
			}
		}
		if !found {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("WSL payload processes remained after bounded cleanup")
}

func waitWSLRuntimeRemoved(t *testing.T, target wslclient.Target, launchID string) {
	t.Helper()
	uidOutput, err := exec.Command("wsl.exe", "--distribution", target.Distribution,
		"--user", target.User, "--exec", "/usr/bin/id", "-u").Output()
	if err != nil {
		t.Fatalf("resolve WSL uid: %v", err)
	}
	runtimePath := "/run/user/" + strings.TrimSpace(string(uidOutput)) + "/cats/" + launchID
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := exec.CommandContext(ctx, "wsl.exe", "--distribution", target.Distribution,
			"--user", target.User, "--exec", "/usr/bin/stat", runtimePath).Run()
		cancel()
		if err != nil {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("WSL runtime directory %s remained after owner loss", runtimePath)
}
