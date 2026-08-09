//go:build catapp_headless && !darwin && !windows

package main

import (
	"strings"
	"testing"
)

func TestWindowsClipboardUTF16RoundTrip(t *testing.T) {
	for _, text := range []string{"", "line 1\r\nline 2\n", "猫 😸 e\u0301"} {
		encoded, err := encodeClipboardUTF16(text)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeClipboardUTF16(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if decoded != text {
			t.Fatalf("round trip = %q, want %q", decoded, text)
		}
	}
}

func TestWindowsClipboardUTF16Validation(t *testing.T) {
	if _, err := encodeClipboardUTF16("before\x00after"); err == nil {
		t.Fatal("embedded NUL unexpectedly accepted")
	}
	if _, err := encodeClipboardUTF16(strings.Repeat("x", maxClipboardUTF16Units+1)); err == nil {
		t.Fatal("oversized clipboard unexpectedly accepted")
	}
	for _, encoded := range [][]uint16{{'x'}, {0xd800, 0}, {0xdc00, 0}} {
		if _, err := decodeClipboardUTF16(encoded); err == nil {
			t.Fatalf("invalid UTF-16 %x unexpectedly accepted", encoded)
		}
	}
}
