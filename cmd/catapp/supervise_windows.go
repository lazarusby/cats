//go:build windows

package main

import (
	"context"
	"fmt"
)

// Local mode fails closed until Phase 4 adds safe wsl.exe discovery and helper
// supervision. Remote mode does not call this function and remains independent
// of WSL.
func startLocalBackend(context.Context, appConfig) (localBackend, error) {
	return nil, fmt.Errorf("the Windows local WSL backend is not implemented yet")
}
