package wslclient

import (
	"strings"
	"testing"
)

func TestRedactLogChunk(t *testing.T) {
	raw := []byte(`failed password=hunter2 TOKEN: "abc def" authorization=Bearer-xyz safe=value`)
	got := string(RedactLogChunk(raw))
	for _, secret := range []string{"hunter2", "abc def", "Bearer-xyz"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted log still contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "safe=value") {
		t.Fatalf("redaction removed ordinary diagnostic data: %s", got)
	}
}
