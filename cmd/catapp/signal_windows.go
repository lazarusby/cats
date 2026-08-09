//go:build windows

package main

import (
	"os"
	"os/signal"
)

// This covers console interrupt during development. Native session/window
// close integration belongs to the Phase 4 Windows adapter.
func installSignalHandler() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)
	go func() {
		<-sigc
		runCleanup()
		os.Exit(0)
	}()
}
