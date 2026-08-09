package wslclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/internal/desktopproto"
)

func startupWire(t *testing.T, records ...desktopproto.HelperRecord) *bytes.Buffer {
	t.Helper()
	var wire bytes.Buffer
	for _, record := range records {
		if err := desktopproto.EncodeHelper(&wire, record); err != nil {
			t.Fatal(err)
		}
	}
	return &wire
}

func TestAwaitStartup(t *testing.T) {
	wire := startupWire(t,
		desktopproto.NewStarting("abc1234", 77),
		desktopproto.NewReady("abc1234", "127.0.0.1:49152", 77),
	)
	got, err := AwaitStartup(context.Background(), wire, "abc1234", "127.0.0.1:49152")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "abc1234" || got.Addr != "127.0.0.1:49152" || got.PID != 77 {
		t.Fatalf("startup = %#v", got)
	}
}

func TestAwaitStartupRejectsMismatchAndClassifiesBind(t *testing.T) {
	tests := []struct {
		name      string
		wire      *bytes.Buffer
		retryable bool
	}{
		{"version", startupWire(t, desktopproto.NewStarting("old", 1)), false},
		{"address", startupWire(t, desktopproto.NewStarting("abc1234", 1), desktopproto.NewReady("abc1234", "127.0.0.1:42", 1)), false},
		{"pid", startupWire(t, desktopproto.NewStarting("abc1234", 1), desktopproto.NewReady("abc1234", "127.0.0.1:49152", 2)), false},
		{"bind", startupWire(t, desktopproto.NewStarting("abc1234", 1), desktopproto.NewError("bind", "address in use")), true},
		{"daemon", startupWire(t, desktopproto.NewStarting("abc1234", 1), desktopproto.NewError("catway", "crashed")), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AwaitStartup(context.Background(), tc.wire, "abc1234", "127.0.0.1:49152")
			var startupErr *StartupError
			if !errors.As(err, &startupErr) || startupErr.Retryable != tc.retryable {
				t.Fatalf("error = %#v, want retryable=%v", err, tc.retryable)
			}
		})
	}
}

func TestAwaitStartupTimeoutAndEOF(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := AwaitStartup(ctx, reader, "abc1234", "127.0.0.1:42"); err == nil {
		t.Fatal("blocked startup unexpectedly succeeded")
	}
	_, err := AwaitStartup(context.Background(), bytes.NewReader(nil), "abc1234", "127.0.0.1:42")
	var startupErr *StartupError
	if !errors.As(err, &startupErr) || startupErr.Stage != "protocol_eof" {
		t.Fatalf("EOF error = %#v", err)
	}
}
