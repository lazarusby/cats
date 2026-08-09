//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPhase5InInstalledEdge(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	edge := os.Getenv("CATS_EDGE_PATH")
	if edge == "" {
		edge = filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe")
	}
	if _, err := os.Stat(edge); err != nil {
		t.Skipf("installed Edge not found at %s", edge)
	}
	cmd := exec.Command(node, "../../scripts/test-webui-edge.mjs")
	cmd.Env = append(os.Environ(), "CATS_EDGE_PATH="+edge)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Phase 5 Edge regression: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
