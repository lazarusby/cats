package desktopproto

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProtocolGoldens pins the exact lifecycle bytes consumed on both sides of
// the Windows/WSL boundary. CI runs this same test on native Windows and Linux;
// a field/order/newline change therefore cannot land on only one half.
func TestProtocolGoldens(t *testing.T) {
	helperRecords := []HelperRecord{
		NewStarting("v7.0.0", 7001),
		NewReady("v7.0.0", "127.0.0.1:47007", 7001),
		NewError("catway", "exited before readiness"),
		NewStopped("requested"),
	}
	var helper bytes.Buffer
	for _, record := range helperRecords {
		if err := EncodeHelper(&helper, record); err != nil {
			t.Fatalf("encode helper golden: %v", err)
		}
	}
	assertGoldenBytes(t, "helper.ndjson", helper.Bytes())
	decoder := NewDecoder(bytes.NewReader(helper.Bytes()))
	for _, want := range helperRecords {
		got, err := decoder.ReadHelper()
		if err != nil || got != want {
			t.Fatalf("decode helper golden = %#v, %v; want %#v", got, err, want)
		}
	}

	var launcher bytes.Buffer
	if err := EncodeLauncher(&launcher, NewStop()); err != nil {
		t.Fatalf("encode launcher golden: %v", err)
	}
	assertGoldenBytes(t, "launcher.ndjson", launcher.Bytes())
	if got, err := NewDecoder(bytes.NewReader(launcher.Bytes())).ReadLauncher(); err != nil || got != NewStop() {
		t.Fatalf("decode launcher golden = %#v, %v", got, err)
	}
}

func assertGoldenBytes(t *testing.T, name string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed\n got: %q\nwant: %q", name, got, want)
	}
}

func TestHelperRoundTrip(t *testing.T) {
	records := []HelperRecord{
		NewStarting("v0.9.0", 123),
		NewReady("v0.9.0", "127.0.0.1:49152", 123),
		NewError("catway", "exited before readiness"),
		NewStopped("requested"),
	}
	var wire bytes.Buffer
	for _, record := range records {
		if err := EncodeHelper(&wire, record); err != nil {
			t.Fatalf("EncodeHelper(%s): %v", record.Type, err)
		}
	}
	decoder := NewDecoder(&wire)
	for _, want := range records {
		got, err := decoder.ReadHelper()
		if err != nil {
			t.Fatalf("ReadHelper(%s): %v", want.Type, err)
		}
		if got != want {
			t.Fatalf("ReadHelper = %#v, want %#v", got, want)
		}
	}
	if _, err := decoder.ReadHelper(); !errors.Is(err, io.EOF) {
		t.Fatalf("final ReadHelper error = %v, want EOF", err)
	}
}

func TestLauncherRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	want := NewStop()
	if err := EncodeLauncher(&wire, want); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(&wire).ReadLauncher()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ReadLauncher = %#v, want %#v", got, want)
	}
}

func TestUnknownAdditiveFieldsAreIgnored(t *testing.T) {
	wire := strings.NewReader(`{"v":1,"type":"ready","app_version":"v1","addr":"127.0.0.1:42","pid":7,"future":true}` + "\n")
	got, err := NewDecoder(wire).ReadHelper()
	if err != nil {
		t.Fatal(err)
	}
	if got != NewReady("v1", "127.0.0.1:42", 7) {
		t.Fatalf("ReadHelper = %#v", got)
	}
}

func TestDecodeFailures(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want error
	}{
		{"malformed JSON", "{not json}\n", ErrMalformed},
		{"not an object", "[]\n", ErrMalformed},
		{"unknown version", `{"v":2,"type":"starting"}` + "\n", ErrUnknownVersion},
		{"missing version", `{"type":"starting"}` + "\n", ErrUnknownVersion},
		{"unknown type", `{"v":1,"type":"launch"}` + "\n", ErrUnknownType},
		{"missing type", `{"v":1}` + "\n", ErrUnknownType},
		{"truncated", `{"v":1,"type":"stop"}`, ErrTruncated},
		{"blank", "\n", ErrMalformed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDecoder(strings.NewReader(tc.wire)).ReadHelper()
			if !errors.Is(err, tc.want) {
				t.Fatalf("ReadHelper error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestOversizedRecord(t *testing.T) {
	wire := `{"v":1,"type":"error","stage":"probe","message":"` + strings.Repeat("x", MaxRecordBytes) + `"}` + "\n"
	_, err := NewDecoder(strings.NewReader(wire)).ReadHelper()
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("ReadHelper error = %v, want ErrRecordTooLarge", err)
	}

	record := NewError("probe", strings.Repeat("x", MaxRecordBytes))
	if err := EncodeHelper(io.Discard, record); !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("EncodeHelper error = %v, want ErrRecordTooLarge", err)
	}
}

func TestPartialReads(t *testing.T) {
	wire := []byte(`{"v":1,"type":"stop"}` + "\n")
	got, err := NewDecoder(&oneByteReader{data: wire}).ReadLauncher()
	if err != nil {
		t.Fatal(err)
	}
	if got != NewStop() {
		t.Fatalf("ReadLauncher = %#v, want %#v", got, NewStop())
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name   string
		record HelperRecord
		want   error
	}{
		{"starting missing version", HelperRecord{Type: TypeStarting, AppVersion: "v1", PID: 1}, ErrUnknownVersion},
		{"starting missing pid", HelperRecord{V: 1, Type: TypeStarting, AppVersion: "v1"}, ErrMalformed},
		{"ready non-loopback", NewReady("v1", "0.0.0.0:80", 1), ErrMalformed},
		{"ready bad port", NewReady("v1", "127.0.0.1:0", 1), ErrMalformed},
		{"error missing stage", NewError("", "failed"), ErrMalformed},
		{"stopped missing reason", NewStopped(""), ErrMalformed},
		{"helper stop direction", HelperRecord{V: 1, Type: TypeStop}, ErrUnknownType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.record.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate error = %v, want %v", err, tc.want)
			}
		})
	}
	if err := (LauncherRecord{V: 1, Type: TypeReady}).Validate(); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("launcher ready validation error = %v", err)
	}
}

func TestEncodeHandlesShortWrites(t *testing.T) {
	var dst bytes.Buffer
	w := &shortWriter{dst: &dst, max: 2}
	if err := EncodeLauncher(w, NewStop()); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.String(), `{"v":1,"type":"stop"}`+"\n"; got != want {
		t.Fatalf("wire = %q, want %q", got, want)
	}
}

type oneByteReader struct {
	data []byte
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

type shortWriter struct {
	dst *bytes.Buffer
	max int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.max {
		p = p[:w.max]
	}
	return w.dst.Write(p)
}
