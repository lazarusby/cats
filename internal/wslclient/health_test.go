package wslclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rohanthewiz/cats/internal/backendhealth"
)

func TestHelperHealth(t *testing.T) {
	wire := []byte(`{"architecture":"amd64","helper_version":"abc1234","home":"/home/alice","payload_path":"/home/alice/.local/lib/cats/current"}`)
	health, err := DecodeHelperHealth(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := health.Validate(validTarget, "abc1234", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		[]byte(`{"architecture":"amd64","helper_version":"abc1234","home":"/home/alice","payload_path":"/x","secret":"no"}`),
		[]byte(`{} {}`),
		make([]byte, MaxOutputBytes+1),
	} {
		if _, err := DecodeHelperHealth(bad); err == nil {
			t.Fatalf("DecodeHelperHealth(%q) unexpectedly succeeded", bad)
		}
	}
	if err := (HelperHealth{Architecture: "arm64", HelperVersion: "old", Home: "/home/alice", PayloadPath: validTarget.PayloadPath}).Validate(validTarget, "abc1234", "amd64"); err == nil {
		t.Fatal("mismatched helper health unexpectedly accepted")
	}
}

func TestVerifyBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != backendhealth.Path {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"product":"cats","version":"abc1234"}`))
	}))
	defer server.Close()
	if err := VerifyBackend(context.Background(), server.Client(), server.URL, "abc1234"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyBackendRejectsIdentityAndClassifiesNetwork(t *testing.T) {
	for _, baseURL := range []string{
		"https://127.0.0.1:42", "http://localhost:42", "http://127.0.0.1:42/path",
		"http://user@127.0.0.1:42", "http://127.0.0.1:bad", "http://127.0.0.1:42?token=x",
	} {
		if err := VerifyBackend(context.Background(), http.DefaultClient, baseURL, "abc1234"); err == nil {
			t.Fatalf("unsafe URL %q unexpectedly accepted", baseURL)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"product":"cats","version":"wrong"}`))
	}))
	if err := VerifyBackend(context.Background(), server.Client(), server.URL, "abc1234"); err == nil {
		t.Fatal("wrong backend version unexpectedly accepted")
	}
	server.Close()

	err := VerifyBackend(context.Background(), errorDoer{err: &net.DNSError{Err: "unreachable"}}, "http://127.0.0.1:42", "abc1234")
	var verification *VerificationError
	if !errors.As(err, &verification) || !verification.Retryable || verification.Stage != "http_forwarding" {
		t.Fatalf("network error = %#v", err)
	}
}

type errorDoer struct{ err error }

func (d errorDoer) Do(*http.Request) (*http.Response, error) { return nil, d.err }

func TestVerifyBackendBoundsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", MaxOutputBytes+1)))
	}))
	defer server.Close()
	if err := VerifyBackend(context.Background(), server.Client(), server.URL, "abc1234"); err == nil {
		t.Fatal("oversized backend response unexpectedly accepted")
	}
}
