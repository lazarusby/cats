//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Environment hydration happens inside cats-wsl-host, not the Windows process.
func hydratePlatformEnvironment() {}

// Windows owns local startup asynchronously so a slow/cold WSL launch always
// has a real, cancellable window instead of leaving WebView2 blank.
func runPlatformLocal(cfg appConfig) bool {
	runWindowsLocal(cfg)
	return true
}

type windowsLocalUI struct {
	ctx    context.Context
	window desktopWindow
	busy   atomic.Bool
	active atomic.Bool

	mu            sync.Mutex
	cfg           appConfig
	logPath       string
	distributions []string
}

func runWindowsLocal(cfg appConfig) {
	installSignalHandler()
	window := newWindow(windowTitle)
	defer window.Destroy()
	ctx, cancel := context.WithCancel(context.Background())
	ui := &windowsLocalUI{ctx: ctx, window: window, cfg: cfg}
	ui.active.Store(true)
	if err := ui.bindActions(); err != nil {
		window.SetHtml(errorPageHTML("Could not initialise Windows startup actions", err.Error()))
		window.Run()
		cancel()
		return
	}
	window.SetHtml(startingPageHTML("Checking WSL and the selected payload"))
	ui.start(cfg, false)
	window.Run()
	cancel()
	runCleanup()
}

func (ui *windowsLocalUI) bindActions() error {
	bindings := []struct {
		name string
		fn   any
	}{
		{"catsRetry", func() {
			if ui.active.Load() {
				ui.start(ui.currentConfig(), false)
			}
		}},
		{"catsSelectWSL", func(distribution, user, payloadPath string) {
			if !ui.active.Load() {
				return
			}
			candidate := ui.currentConfig()
			candidate.Mode = "local"
			candidate.WSL.Distribution = distribution
			candidate.WSL.User = user
			candidate.WSL.PayloadPath = payloadPath
			if err := candidate.WSL.Validate(); err != nil {
				ui.showSetup(ui.currentDistributions(), setupDetail(err, ui.currentLogPath()))
				return
			}
			ui.start(candidate, true)
		}},
		{"catsChangeWSL", func() {
			if ui.active.Load() {
				ui.discoverAndShowSetup()
			}
		}},
		{"catsOpenLogs", func() {
			if !ui.active.Load() {
				return
			}
			path := ui.currentLogPath()
			if path == "" {
				ui.showActionError("No launcher log has been created yet.")
				return
			}
			if err := openWindowsPath(path); err != nil {
				ui.showActionError("Could not open launcher log: " + err.Error())
			}
		}},
		{"catsRepair", func() {
			if !ui.active.Load() {
				return
			}
			if err := launchWindowsInstaller(); err != nil {
				ui.showActionError("Could not launch repair: " + err.Error())
			}
		}},
	}
	for _, binding := range bindings {
		if err := ui.window.Bind(binding.name, binding.fn); err != nil {
			return fmt.Errorf("bind %s: %w", binding.name, err)
		}
	}
	return nil
}

func (ui *windowsLocalUI) start(cfg appConfig, persist bool) {
	if !ui.busy.CompareAndSwap(false, true) {
		return
	}
	ui.mu.Lock()
	ui.cfg = cfg
	ui.mu.Unlock()
	ui.window.Dispatch(func() {
		ui.window.SetHtml(startingPageHTML("Verifying WSL payload and starting cats"))
	})
	go func() {
		backend, err := localBackendStarter(ui.ctx, cfg)
		if err != nil {
			ui.busy.Store(false)
			ui.presentStartupError(err)
			return
		}
		if ui.ctx.Err() != nil {
			_ = backend.Stop(context.Background())
			return
		}
		if persist {
			if err := saveAppConfig(cfg); err != nil {
				_ = backend.Stop(context.Background())
				ui.busy.Store(false)
				ui.showActionError("Target verified but could not be saved: " + err.Error())
				return
			}
		}
		registerCleanup(backendCleanup(backend))
		ui.active.Store(false)
		ui.window.Dispatch(func() { ui.window.Navigate(backend.URL()) })
	}()
}

func (ui *windowsLocalUI) presentStartupError(err error) {
	if ui.ctx.Err() != nil {
		return
	}
	var targetErr *windowsTargetRequiredError
	if errors.As(err, &targetErr) {
		ui.mu.Lock()
		ui.logPath = targetErr.LogPath
		ui.distributions = append([]string(nil), targetErr.Distributions...)
		ui.mu.Unlock()
		ui.showSetup(targetErr.Distributions, setupDetail(targetErr.Cause, targetErr.LogPath))
		return
	}
	var startupErr *windowsStartupError
	if errors.As(err, &startupErr) {
		ui.mu.Lock()
		ui.logPath = startupErr.LogPath
		ui.mu.Unlock()
	}
	ui.window.Dispatch(func() {
		ui.window.SetHtml(windowsStartupErrorPageHTML("Could not start cats", err.Error()))
	})
}

func (ui *windowsLocalUI) discoverAndShowSetup() {
	if !ui.busy.CompareAndSwap(false, true) {
		return
	}
	ui.window.Dispatch(func() { ui.window.SetHtml(startingPageHTML("Discovering WSL distributions")) })
	go func() {
		defer ui.busy.Store(false)
		wslPath, err := exec.LookPath("wsl.exe")
		if err != nil {
			ui.showActionError("wsl.exe was not found")
			return
		}
		distributions, err := listWSLDistributions(ui.ctx, wslPath, io.Discard)
		if err != nil {
			ui.showActionError("Could not list WSL distributions: " + err.Error())
			return
		}
		ui.mu.Lock()
		ui.distributions = append([]string(nil), distributions...)
		ui.mu.Unlock()
		ui.showSetup(distributions, "Select the WSL2 distribution and installed payload to use.")
	}()
}

func (ui *windowsLocalUI) showSetup(distributions []string, detail string) {
	if ui.ctx.Err() == nil {
		ui.window.Dispatch(func() { ui.window.SetHtml(wslSetupPageHTML(distributions, detail)) })
	}
}

func (ui *windowsLocalUI) showActionError(detail string) {
	if ui.ctx.Err() == nil {
		ui.window.Dispatch(func() { ui.window.SetHtml(windowsStartupErrorPageHTML("Windows action failed", detail)) })
	}
}

func (ui *windowsLocalUI) currentConfig() appConfig {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.cfg
}

func (ui *windowsLocalUI) currentLogPath() string {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.logPath
}

func (ui *windowsLocalUI) currentDistributions() []string {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return append([]string(nil), ui.distributions...)
}

// The Windows asynchronous flow consumes every local error itself.
func handleLocalBackendError(appConfig, error) bool { return false }

func setupDetail(err error, logPath string) string {
	if logPath == "" {
		return err.Error()
	}
	return err.Error() + "\n\nLog: " + logPath
}

var windowsExecutablePath = os.Executable

func launchWindowsInstaller() error {
	executable, err := windowsExecutablePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(executable)
	var installer string
	for _, candidate := range []string{
		filepath.Join(dir, "Install-Cats.ps1"),
		filepath.Join(dir, "install-cats.ps1"),
		filepath.Join(dir, "installer", "Install-Cats.ps1"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			installer = candidate
			break
		}
	}
	if installer == "" {
		return fmt.Errorf("Install-Cats.ps1 is not present beside %s", filepath.Base(executable))
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		return fmt.Errorf("powershell.exe was not found: %w", err)
	}
	command := exec.Command(powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", installer)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
