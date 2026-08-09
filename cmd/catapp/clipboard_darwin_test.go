//go:build darwin

package main

import (
	"slices"
	"testing"
)

func TestDarwinClipboardBridgeNames(t *testing.T) {
	w := &fakeWebView{}
	if err := bindPlatformBridges(w); err != nil {
		t.Fatal(err)
	}
	want := []string{"catsClipWrite", "catsClipRead"}
	if !slices.Equal(w.bindings, want) {
		t.Fatalf("bindings = %q, want %q", w.bindings, want)
	}
}
