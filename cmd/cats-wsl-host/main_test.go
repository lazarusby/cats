//go:build linux

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/internal/desktopproto"
)

func TestParseOptions(t *testing.T) {
	opts, err := parseOptions([]string{
		"--port", "49152", "--launch-id", "launch_ABC-123",
		"--start-dir", "/tmp", "--idle-timeout", "2m",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if opts.port != 49152 || opts.launchID != "launch_ABC-123" || opts.startDir != "/tmp" || opts.idleTimeout != 2*time.Minute {
		t.Fatalf("options = %#v", opts)
	}
}

func TestParseOptionsRejectsUnsafeValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"missing port", []string{"--launch-id", "ok"}},
		{"bad port", []string{"--port", "65536", "--launch-id", "ok"}},
		{"missing id", []string{"--port", "42"}},
		{"traversal id", []string{"--port", "42", "--launch-id", "../bad"}},
		{"unicode id", []string{"--port", "42", "--launch-id", "猫"}},
		{"long id", []string{"--port", "42", "--launch-id", strings.Repeat("a", 65)}},
		{"relative start", []string{"--port", "42", "--launch-id", "ok", "--start-dir", "relative"}},
		{"negative idle", []string{"--port", "42", "--launch-id", "ok", "--idle-timeout", "-1s"}},
		{"positional", []string{"--port", "42", "--launch-id", "ok", "extra"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseOptions(tc.args, io.Discard); err == nil {
				t.Fatal("parseOptions unexpectedly succeeded")
			}
		})
	}
}

func TestHealthOptionsDoNotRequireLaunchFlags(t *testing.T) {
	opts, err := parseOptions([]string{"--health-json"}, io.Discard)
	if err != nil || !opts.healthJSON {
		t.Fatalf("parseOptions health = %#v, %v", opts, err)
	}
}

func TestRequireWSLAndOverride(t *testing.T) {
	dir := t.TempDir()
	ordinary := filepath.Join(dir, "ordinary")
	microsoft := filepath.Join(dir, "microsoft")
	if err := os.WriteFile(ordinary, []byte("6.8.0-generic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(microsoft, []byte("6.6.87.2-microsoft-standard-WSL2"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := osReleaseFiles
	t.Cleanup(func() { osReleaseFiles = old })
	osReleaseFiles = []string{ordinary}
	if err := requireWSL(false); err == nil {
		t.Fatal("ordinary Linux unexpectedly accepted")
	}
	if err := requireWSL(true); err != nil {
		t.Fatalf("development override rejected: %v", err)
	}
	osReleaseFiles = []string{ordinary, microsoft}
	if err := requireWSL(false); err != nil {
		t.Fatalf("WSL marker rejected: %v", err)
	}
}

func TestVerifyPayload(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"catway", "cathost", "catctl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("binary"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyPayload(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "catctl"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPayload(dir); err == nil {
		t.Fatal("non-executable catctl unexpectedly accepted")
	}
}

func TestReadControl(t *testing.T) {
	var stop bytes.Buffer
	if err := desktopproto.EncodeLauncher(&stop, desktopproto.NewStop()); err != nil {
		t.Fatal(err)
	}
	if event := <-readControl(&stop); event.reason != "requested" || event.err != nil {
		t.Fatalf("stop event = %#v", event)
	}
	if event := <-readControl(strings.NewReader("")); event.reason != "stdin_eof" || event.err != nil {
		t.Fatalf("EOF event = %#v", event)
	}
	if event := <-readControl(strings.NewReader("bad\n")); event.reason != "protocol_error" || event.err == nil {
		t.Fatalf("malformed event = %#v", event)
	}
}

func TestProtocolOutputContainsRecordsOnly(t *testing.T) {
	var wire bytes.Buffer
	p := protocolOutput{w: &wire}
	if err := p.write(desktopproto.NewStarting("test", 1)); err != nil {
		t.Fatal(err)
	}
	record, err := desktopproto.NewDecoder(&wire).ReadHelper()
	if err != nil || record.Type != desktopproto.TypeStarting {
		t.Fatalf("record = %#v, %v", record, err)
	}
	if _, err := desktopproto.NewDecoder(&wire).ReadHelper(); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing stdout data: %v", err)
	}
}
