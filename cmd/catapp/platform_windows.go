//go:build windows

package main

import (
	"context"
	"sync/atomic"
)

// Environment hydration happens inside cats-wsl-host, not the Windows process.
func hydratePlatformEnvironment() {}
func installMenu(desktopWindow)   {} // native menu awaits the WebView2 UI runner
func bindPlatformBridges(desktopWindow) error {
	// The clipboard implementation is ready and compile-tested, but the pinned
	// webview wrapper exposes no navigation/new-window callbacks. Do not attach a
	// privileged binding until cross-origin navigation can be denied reliably.
	return nil
}

func handleLocalBackendError(cfg appConfig, err error) bool {
	targetErr, ok := err.(*windowsTargetRequiredError)
	if !ok {
		return false
	}

	w := newWindow("cats — WSL setup")
	defer w.Destroy()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var submitted atomic.Bool
	if bindErr := w.Bind("catsSelectWSL", func(distribution, user, payloadPath string) {
		if !submitted.CompareAndSwap(false, true) {
			return
		}
		candidate := cfg
		candidate.Mode = "local"
		candidate.WSL.Distribution = distribution
		candidate.WSL.User = user
		candidate.WSL.PayloadPath = payloadPath
		if validateErr := candidate.WSL.Validate(); validateErr != nil {
			submitted.Store(false)
			w.Dispatch(func() {
				w.SetHtml(wslSetupPageHTML(targetErr.Distributions, setupDetail(validateErr, targetErr.LogPath)))
			})
			return
		}
		w.Dispatch(func() { w.SetHtml(startingPageHTML("Verifying WSL payload and starting cats")) })
		go func() {
			backend, startErr := startLocalBackend(ctx, candidate)
			if startErr != nil {
				if ctx.Err() == nil {
					w.Dispatch(func() {
						w.SetHtml(wslSetupPageHTML(targetErr.Distributions, startErr.Error()))
						submitted.Store(false)
					})
				}
				return
			}
			if ctx.Err() != nil {
				_ = backend.Stop(context.Background())
				return
			}
			if saveErr := saveAppConfig(candidate); saveErr != nil {
				_ = backend.Stop(context.Background())
				if ctx.Err() == nil {
					w.Dispatch(func() {
						w.SetHtml(wslSetupPageHTML(targetErr.Distributions, "Target verified but could not be saved: "+saveErr.Error()))
						submitted.Store(false)
					})
				}
				return
			}
			registerCleanup(backendCleanup(backend))
			if ctx.Err() == nil {
				w.Dispatch(func() { w.Navigate(backend.URL()) })
			}
		}()
	}); bindErr != nil {
		return false
	}
	w.SetHtml(wslSetupPageHTML(targetErr.Distributions, setupDetail(targetErr.Cause, targetErr.LogPath)))
	installSignalHandler()
	w.Run()
	cancel()
	runCleanup()
	return true
}

func setupDetail(err error, logPath string) string {
	if logPath == "" {
		return err.Error()
	}
	return err.Error() + "\n\nLog: " + logPath
}
