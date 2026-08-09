package wslclient

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/rohanthewiz/cats/internal/desktopproto"
)

type Startup struct {
	Version string
	Addr    string
	PID     int
}

type StartupError struct {
	Stage     string
	Retryable bool
	Err       error
}

func (e *StartupError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *StartupError) Unwrap() error { return e.Err }

func AwaitStartup(ctx context.Context, reader io.Reader, expectedVersion, expectedAddr string) (Startup, error) {
	type result struct {
		startup Startup
		err     error
	}
	results := make(chan result, 1)
	go func() {
		startup, err := readStartup(reader, expectedVersion, expectedAddr)
		results <- result{startup: startup, err: err}
	}()
	select {
	case <-ctx.Done():
		return Startup{}, &StartupError{Stage: "protocol_timeout", Err: ctx.Err()}
	case result := <-results:
		return result.startup, result.err
	}
}

func readStartup(reader io.Reader, expectedVersion, expectedAddr string) (Startup, error) {
	decoder := desktopproto.NewDecoder(reader)
	first, err := decoder.ReadHelper()
	if err != nil {
		return Startup{}, protocolReadError(err)
	}
	if first.Type == desktopproto.TypeError {
		return Startup{}, helperError(first)
	}
	if first.Type != desktopproto.TypeStarting {
		return Startup{}, &StartupError{Stage: "protocol", Err: fmt.Errorf("expected starting record, received %s", first.Type)}
	}
	if first.AppVersion != expectedVersion {
		return Startup{}, &StartupError{Stage: "version", Err: fmt.Errorf("helper version %q does not match launcher %q", first.AppVersion, expectedVersion)}
	}

	second, err := decoder.ReadHelper()
	if err != nil {
		return Startup{}, protocolReadError(err)
	}
	if second.Type == desktopproto.TypeError {
		return Startup{}, helperError(second)
	}
	if second.Type != desktopproto.TypeReady {
		return Startup{}, &StartupError{Stage: "protocol", Err: fmt.Errorf("expected ready record, received %s", second.Type)}
	}
	if second.AppVersion != expectedVersion || second.AppVersion != first.AppVersion {
		return Startup{}, &StartupError{Stage: "version", Err: fmt.Errorf("ready version %q does not match launcher %q", second.AppVersion, expectedVersion)}
	}
	if second.Addr != expectedAddr {
		return Startup{}, &StartupError{Stage: "protocol", Err: fmt.Errorf("ready address %q does not match selected address %q", second.Addr, expectedAddr)}
	}
	if second.PID != first.PID {
		return Startup{}, &StartupError{Stage: "protocol", Err: fmt.Errorf("helper pid changed from %d to %d", first.PID, second.PID)}
	}
	return Startup{Version: second.AppVersion, Addr: second.Addr, PID: second.PID}, nil
}

func helperError(record desktopproto.HelperRecord) error {
	return &StartupError{
		Stage:     record.Stage,
		Retryable: record.Stage == "bind" || record.Stage == "forwarding",
		Err:       errors.New(record.Message),
	}
}

func protocolReadError(err error) error {
	stage := "protocol"
	if errors.Is(err, io.EOF) {
		stage = "protocol_eof"
	}
	return &StartupError{Stage: stage, Err: err}
}
