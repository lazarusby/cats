package wslclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/rohanthewiz/cats/internal/backendhealth"
)

type HelperHealth struct {
	Architecture  string `json:"architecture"`
	HelperVersion string `json:"helper_version"`
	Home          string `json:"home"`
	PayloadPath   string `json:"payload_path"`
}

func DecodeHelperHealth(data []byte) (HelperHealth, error) {
	if len(data) == 0 || len(data) > MaxOutputBytes {
		return HelperHealth{}, errors.New("helper health output has invalid size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var health HelperHealth
	if err := decoder.Decode(&health); err != nil {
		return HelperHealth{}, fmt.Errorf("decode helper health: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return HelperHealth{}, err
	}
	return health, nil
}

func (h HelperHealth) Validate(target Target, expectedVersion, expectedArchitecture string) error {
	if h.HelperVersion == "" || h.HelperVersion != expectedVersion {
		return fmt.Errorf("helper version %q does not match launcher %q", h.HelperVersion, expectedVersion)
	}
	if expectedArchitecture == "" {
		expectedArchitecture = runtime.GOARCH
	}
	if h.Architecture != expectedArchitecture {
		return fmt.Errorf("helper architecture %q does not match launcher payload %q", h.Architecture, expectedArchitecture)
	}
	if err := ValidateLinuxPath("helper home", h.Home); err != nil {
		return err
	}
	if pathClean(h.PayloadPath) != pathClean(target.PayloadPath) {
		return fmt.Errorf("helper payload path %q does not match configured path %q", h.PayloadPath, target.PayloadPath)
	}
	return nil
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type VerificationError struct {
	Stage     string
	Retryable bool
	Err       error
}

func (e *VerificationError) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *VerificationError) Unwrap() error { return e.Err }

func VerifyBackend(ctx context.Context, client HTTPDoer, baseURL, expectedVersion string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return &VerificationError{Stage: "http_identity", Err: fmt.Errorf("unsafe backend URL %q", baseURL)}
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return &VerificationError{Stage: "http_identity", Err: fmt.Errorf("unsafe backend URL %q", baseURL)}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+backendhealth.Path, nil)
	if err != nil {
		return &VerificationError{Stage: "http_identity", Err: err}
	}
	response, err := client.Do(request)
	if err != nil {
		var netErr net.Error
		return &VerificationError{Stage: "http_forwarding", Retryable: errors.As(err, &netErr), Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &VerificationError{Stage: "http_identity", Err: fmt.Errorf("health endpoint returned HTTP %d", response.StatusCode)}
	}
	limited := io.LimitReader(response.Body, MaxOutputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return &VerificationError{Stage: "http_identity", Err: err}
	}
	if len(data) > MaxOutputBytes {
		return &VerificationError{Stage: "http_identity", Err: errors.New("health response is too large")}
	}
	var report backendhealth.Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return &VerificationError{Stage: "http_identity", Err: fmt.Errorf("decode backend health: %w", err)}
	}
	if err := requireJSONEOF(decoder); err != nil {
		return &VerificationError{Stage: "http_identity", Err: err}
	}
	if err := report.Validate(expectedVersion); err != nil {
		return &VerificationError{Stage: "http_identity", Err: err}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON output contains more than one value")
		}
		return fmt.Errorf("JSON trailing data: %w", err)
	}
	return nil
}

func pathClean(value string) string {
	return path.Clean(value)
}
