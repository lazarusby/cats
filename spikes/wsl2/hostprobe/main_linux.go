//go:build linux

// Command hostprobe is a disposable Phase 1 stand-in for cats-wsl-host. It
// proves the exact wsl.exe process, lifecycle-pipe, and localhost seams without
// taking on production supervision responsibilities.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/rohanthewiz/cats/internal/desktopproto"
)

func main() {
	port := flag.Int("port", 18421, "loopback TCP port")
	flag.Parse()

	pid := os.Getpid()
	if err := desktopproto.EncodeHelper(os.Stdout, desktopproto.NewStarting("phase1-spike", pid)); err != nil {
		fatal("protocol", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fatal("listen", err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("cats WSL Phase 1 probe\n"))
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	if err := desktopproto.EncodeHelper(os.Stdout, desktopproto.NewReady("phase1-spike", addr, pid)); err != nil {
		_ = server.Close()
		fatal("protocol", err)
	}

	reason := "stdin_eof"
	record, err := desktopproto.NewDecoder(os.Stdin).ReadLauncher()
	if err == nil && record.Type == desktopproto.TypeStop {
		reason = "requested"
	} else if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "hostprobe: control input: %v\n", err)
		reason = "control_error"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		_ = server.Close()
	}
	if err := desktopproto.EncodeHelper(os.Stdout, desktopproto.NewStopped(reason)); err != nil {
		fatal("protocol", err)
	}
}

func fatal(stage string, err error) {
	_ = desktopproto.EncodeHelper(os.Stdout, desktopproto.NewError(stage, err.Error()))
	fmt.Fprintf(os.Stderr, "hostprobe: %s: %v\n", stage, err)
	os.Exit(1)
}
