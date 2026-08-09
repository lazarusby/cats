package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWebPlatformPolicyWithNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping dependency-free browser policy tests")
	}
	cmd := exec.Command(node, "--test", "web/platform.test.cjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node platform policy tests: %v\n%s", err, out)
	}
	t.Logf("Node platform policy tests passed:\n%s", out)
}

func TestPhase5PageContracts(t *testing.T) {
	raw, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	for _, want := range []string{
		"CatsPlatform.keyboardAction",
		"window.addEventListener(\"compositionstart\"",
		"window.pasteText = pasteText",
		"window.openHelp = openHelp",
		"CatsPlatform.clipboardRead",
		"CatsPlatform.clipboardWrite",
		`case "clipboard"`, // OSC 52 uses the shared clipWrite path.
		"readAndCopy",      // selection/copy mode uses the shared clipWrite path.
		"copyScrollback",
		"CatsPlatform.notificationPlan",
		"sendCmd(\"agent.focus\"",
		"CatsPlatform.safeExternalURL",
		"CatsPlatform.reconnectDelay",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Phase 5 page contract missing %q", want)
		}
	}
}
