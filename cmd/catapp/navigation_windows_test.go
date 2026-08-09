//go:build windows

package main

import "testing"

func TestWindowsNavigationPolicy(t *testing.T) {
	var policy windowsNavigationPolicy
	if got := policy.decide("about:blank", false); got != navigationAllow {
		t.Fatalf("initial about:blank = %v", got)
	}
	if got := policy.decide("javascript:alert(1)", false); got != navigationCancel {
		t.Fatalf("javascript navigation = %v", got)
	}
	if err := policy.trust("https://Cats.Example/app/start?x=1"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		url       string
		newWindow bool
		want      navigationAction
	}{
		{"https://cats.example/other", false, navigationAllow},
		{"https://cats.example:443/login", false, navigationAllow},
		{"http://cats.example/", false, navigationExternal},
		{"https://other.example/", false, navigationExternal},
		{"https://cats.example/docs", true, navigationExternal},
		{"file:///C:/secret", false, navigationCancel},
		{"data:text/html,unsafe", false, navigationCancel},
		{"not a URL", false, navigationCancel},
	}
	for _, tc := range tests {
		if got := policy.decide(tc.url, tc.newWindow); got != tc.want {
			t.Errorf("decide(%q, %v) = %v, want %v", tc.url, tc.newWindow, got, tc.want)
		}
	}
	if !policy.allowsTrustedOrigin("https://cats.example/notify") {
		t.Error("trusted notification origin was denied")
	}
	for _, raw := range []string{"https://other.example/notify", "file:///C:/notify", "not a URL"} {
		if policy.allowsTrustedOrigin(raw) {
			t.Errorf("untrusted permission origin %q was allowed", raw)
		}
	}
}

func TestValidateRemoteURL(t *testing.T) {
	for _, raw := range []string{"https://cats.example/path", "http://127.0.0.1:8421/"} {
		if err := validateRemoteURL(raw); err != nil {
			t.Errorf("valid URL %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "file:///tmp/cats", "https://user:secret@cats.example/", "//missing-scheme"} {
		if err := validateRemoteURL(raw); err == nil {
			t.Errorf("unsafe URL %q accepted", raw)
		}
	}
}
