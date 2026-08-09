//go:build darwin || windows || catapp_headless

package main

import (
	"context"
	"log"
)

// localBackend is the entire platform seam needed by common local-mode flow.
// Darwin directly supervises sibling daemons; Windows will supervise the WSL
// helper. Terminal data continues to use the existing browser protocol.
type localBackend interface {
	URL() string
	Stop(context.Context) error
}

func backendCleanup(backend localBackend) func() {
	return func() {
		if err := backend.Stop(context.Background()); err != nil {
			log.Printf("could not stop local backend cleanly: %v", err)
		}
	}
}
