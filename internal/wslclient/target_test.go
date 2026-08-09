package wslclient

import (
	"encoding/binary"
	"reflect"
	"testing"
	"unicode/utf16"
)

var validTarget = Target{
	Distribution: "Ubuntu-24.04",
	User:         "alice",
	PayloadPath:  "/home/alice/.local/lib/cats/current",
}

func TestTargetAndArgumentVectors(t *testing.T) {
	if err := validTarget.Validate(); err != nil {
		t.Fatal(err)
	}
	wantHealth := []string{
		"--distribution", "Ubuntu-24.04", "--user", "alice", "--exec",
		"/home/alice/.local/lib/cats/current/cats-wsl-host", "--health-json",
	}
	if got := HealthArgs(validTarget); !reflect.DeepEqual(got, wantHealth) {
		t.Fatalf("HealthArgs = %q, want %q", got, wantHealth)
	}
	wantLaunch := []string{
		"--distribution", "Ubuntu-24.04", "--user", "alice", "--exec",
		"/home/alice/.local/lib/cats/current/cats-wsl-host",
		"--port", "49152", "--launch-id", "launch_123",
		"--start-dir", "/home/alice/project with spaces",
	}
	got, err := LaunchArgs(validTarget, 49152, "launch_123", "/home/alice/project with spaces")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantLaunch) {
		t.Fatalf("LaunchArgs = %q, want %q", got, wantLaunch)
	}
	for _, forbidden := range []string{"sh", "-c", "cmd.exe", "/c"} {
		for _, arg := range got {
			if arg == forbidden {
				t.Fatalf("unsafe shell argument %q in %q", forbidden, got)
			}
		}
	}
}

func TestTargetValidationRejectsUnsafeState(t *testing.T) {
	for _, target := range []Target{
		{},
		{Distribution: "Ubuntu\nother", User: "alice", PayloadPath: "/home/alice/cats"},
		{Distribution: "Ubuntu", User: "alice\x00root", PayloadPath: "/home/alice/cats"},
		{Distribution: "Ubuntu", User: "alice", PayloadPath: "relative"},
		{Distribution: "Ubuntu", User: "alice", PayloadPath: "/"},
		{Distribution: " Ubuntu", User: "alice", PayloadPath: "/home/alice/cats"},
		{Distribution: "Ubuntu", User: "alice", PayloadPath: "/mnt"},
		{Distribution: "Ubuntu", User: "alice", PayloadPath: "/mnt/c/Cats"},
	} {
		if err := target.Validate(); err == nil {
			t.Fatalf("Validate(%#v) unexpectedly succeeded", target)
		}
	}
	if _, err := LaunchArgs(validTarget, 0, "ok", ""); err == nil {
		t.Fatal("invalid port unexpectedly accepted")
	}
	if _, err := LaunchArgs(validTarget, 42, "../bad", ""); err == nil {
		t.Fatal("unsafe launch id unexpectedly accepted")
	}
	if _, err := LaunchArgs(validTarget, 42, "ok", "relative"); err == nil {
		t.Fatal("relative start directory unexpectedly accepted")
	}
}

func TestDecodeDistributionListUTF8AndUTF16(t *testing.T) {
	want := []string{"Ubuntu-24.04", "Debian 猫"}
	utf8Data := []byte(" Ubuntu-24.04\r\nDebian 猫\r\nUbuntu-24.04\r\n")
	if got, err := DecodeDistributionList(utf8Data); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("UTF-8 distributions = %q, %v", got, err)
	}
	units := append([]uint16{0xfeff}, utf16.Encode([]rune("Ubuntu-24.04\r\nDebian 猫\r\n"))...)
	utf16Data := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(utf16Data[index*2:], unit)
	}
	if got, err := DecodeDistributionList(utf16Data); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("UTF-16 distributions = %q, %v", got, err)
	}
}

func TestDecodeDistributionListFailures(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		[]byte("\n\r\n"),
		append([]byte{0xff, 0xfe}, []byte{1}...),
		make([]byte, MaxOutputBytes+1),
	} {
		if _, err := DecodeDistributionList(data); err == nil {
			t.Fatalf("DecodeDistributionList(%d bytes) unexpectedly succeeded", len(data))
		}
	}
}
