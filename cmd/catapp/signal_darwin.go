//go:build darwin

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// installSignalHandler reaps the backend on termination. The deferred cleanup
// in runLocal covers only a normal webview return.
func installSignalHandler() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigc
		runCleanup()
		os.Exit(0)
	}()
}
