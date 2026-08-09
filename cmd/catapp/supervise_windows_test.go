//go:build windows

package main

import (
	"context"
	"testing"
)

func TestWindowsLocalBackendFailsClosedUntilPhase4(t *testing.T) {
	backend, err := startLocalBackend(context.Background(), appConfig{Mode: "local"})
	if err == nil || backend != nil {
		t.Fatalf("startLocalBackend = (%v, %v), want nil backend and explicit error", backend, err)
	}
}
