package shellenv

import "testing"

func TestBetweenMarkers(t *testing.T) {
	const marker = "__CATS_PATH__"
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"bare", marker + "/usr/bin:/bin" + marker, "/usr/bin:/bin"},
		{"noise", "banner\n" + marker + "/opt/go/bin" + marker + "\n$ ", "/opt/go/bin"},
		{"missing", "command not found", ""},
		{"unterminated", marker + "/usr/bin", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := BetweenMarkers(tc.in, marker); got != tc.want {
				t.Fatalf("BetweenMarkers = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergePATH(t *testing.T) {
	got := MergePATH("/opt/go/bin:/usr/bin:/bin", "/usr/bin:/bin:/managed/only:")
	want := "/opt/go/bin:/usr/bin:/bin:/managed/only"
	if got != want {
		t.Fatalf("MergePATH = %q, want %q", got, want)
	}
}
