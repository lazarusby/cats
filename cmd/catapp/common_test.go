//go:build darwin || windows || catapp_headless

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rohanthewiz/cats/internal/wslclient"
)

func TestCleanupControllerRunsBackendStopOnce(t *testing.T) {
	var events []string
	backend := &fakeBackend{
		url: "http://127.0.0.1:1234",
		stop: func(context.Context) error {
			events = append(events, "backend-stop")
			return nil
		},
	}
	var cleanup cleanupController
	cleanup.register(backendCleanup(backend))
	cleanup.run()
	cleanup.run()
	if got, want := strings.Join(events, ","), "backend-stop"; got != want {
		t.Fatalf("cleanup events = %q, want %q", got, want)
	}
}

func TestCleanupControllerWithoutCallback(t *testing.T) {
	var cleanup cleanupController
	cleanup.run()
	cleanup.run()
}

func TestCleanupRegisteredAfterQuitRunsImmediately(t *testing.T) {
	var cleanup cleanupController
	cleanup.run()
	called := 0
	cleanup.register(func() { called++ })
	cleanup.register(func() { called++ })
	if called != 2 {
		t.Fatalf("late cleanup calls = %d, want 2", called)
	}
}

func TestBackendCleanupToleratesStopError(t *testing.T) {
	called := false
	cleanup := backendCleanup(&fakeBackend{stop: func(context.Context) error {
		called = true
		return errors.New("test stop failure")
	}})
	cleanup()
	if !called {
		t.Fatal("backend Stop was not called")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", appConfigFile)
	want := appConfig{
		Mode: "remote",
		WSL: wslclient.Target{
			Distribution: "Ubuntu-24.04",
			User:         "alice",
			PayloadPath:  "/home/alice/.local/lib/cats/current",
		},
		Remote: remoteTarget{URL: "https://cats.example", Label: "home"},
	}
	if err := saveAppConfigFile(path, want); err != nil {
		t.Fatal(err)
	}
	if got := loadAppConfigFile(path, "local"); got != want {
		t.Fatalf("loadAppConfigFile = %#v, want %#v", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != appConfigFile {
		t.Fatalf("config directory contains staging debris: %v", entries)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil {
			t.Fatal(err)
		} else if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("app.json mode = %#o, want 0600", got)
		}
	}
}

func TestConfigFallbacks(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content *string
		want    appConfig
	}{
		{"missing", nil, appConfig{Mode: "remote"}},
		{"malformed", ptr("{"), appConfig{Mode: "remote"}},
		{"empty mode", ptr(`{"remote":{"url":"https://cats.example"}}`), appConfig{Mode: "remote", Remote: remoteTarget{URL: "https://cats.example"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if tc.content != nil {
				if err := os.WriteFile(path, []byte(*tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := loadAppConfigFile(path, "remote"); got != tc.want {
				t.Fatalf("loadAppConfigFile = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestModeAndRemoteTitle(t *testing.T) {
	if !isRemoteMode(appConfig{Mode: "remote"}) {
		t.Fatal("remote config did not select remote mode")
	}
	if isRemoteMode(appConfig{Mode: "unknown"}) {
		t.Fatal("unknown mode must retain the local fallback")
	}
	if got, want := remoteTitle("https://cats.example:9443/path"), "cats — cats.example:9443"; got != want {
		t.Fatalf("remoteTitle = %q, want %q", got, want)
	}
	if got := remoteTitle(":bad-url"); got != windowTitle {
		t.Fatalf("invalid remoteTitle = %q, want %q", got, windowTitle)
	}
}

func TestBuiltInPages(t *testing.T) {
	if !strings.Contains(connectPageHTML, "window.catsConnect(v)") {
		t.Fatal("connect page no longer invokes catsConnect")
	}
	page := errorPageHTML(`<startup>`, `bad & "worse"`)
	for _, raw := range []string{"<startup>", `bad & "worse"`} {
		if strings.Contains(page, raw) {
			t.Fatalf("error page contains unescaped input %q", raw)
		}
	}
	setup := wslSetupPageHTML([]string{`Ubuntu"><script>`}, `<repair required>`)
	for _, raw := range []string{`Ubuntu"><script>`, `<repair required>`} {
		if strings.Contains(setup, raw) {
			t.Fatalf("WSL setup page contains unescaped input %q", raw)
		}
	}
	if !strings.Contains(startingPageHTML("Checking WSL"), "Checking WSL") {
		t.Fatal("starting page omitted its stage")
	}
	actions := windowsStartupErrorPageHTML("failed", "detail")
	for _, action := range []string{"catsRetry", "catsRepair", "catsChangeWSL", "catsOpenLogs"} {
		if !strings.Contains(actions, action) {
			t.Fatalf("Windows startup error page omitted %s", action)
		}
	}
}

func TestZoomFontDispatchesToPageHook(t *testing.T) {
	w := &fakeWebView{}
	previous := uiWindow
	uiWindow = w
	t.Cleanup(func() { uiWindow = previous })
	zoomFont(1)
	if got, want := w.eval, "window.catsAdjustFont && window.catsAdjustFont(1)"; got != want {
		t.Fatalf("Eval = %q, want %q", got, want)
	}
}

func ptr(value string) *string { return &value }

type fakeBackend struct {
	url  string
	stop func(context.Context) error
}

func (b *fakeBackend) URL() string { return b.url }
func (b *fakeBackend) Stop(ctx context.Context) error {
	if b.stop == nil {
		return nil
	}
	return b.stop(ctx)
}

type fakeWebView struct {
	bindings   []string
	bound      map[string]interface{}
	eval       string
	title      string
	url        string
	html       string
	ran        bool
	destroy    bool
	onRun      func(*fakeWebView)
	onDestroy  func(*fakeWebView)
	onNavigate func(string)
}

func (w *fakeWebView) Run() {
	if w.onRun != nil {
		w.onRun(w)
	}
	w.ran = true
}
func (_ *fakeWebView) Dispatch(fn func()) { fn() }
func (w *fakeWebView) Destroy() {
	w.destroy = true
	if w.onDestroy != nil {
		w.onDestroy(w)
	}
}
func (w *fakeWebView) SetTitle(title string)    { w.title = title }
func (*fakeWebView) SetSize(int, int, sizeHint) {}
func (w *fakeWebView) Navigate(url string) {
	w.url = url
	if w.onNavigate != nil {
		w.onNavigate(url)
	}
}
func (w *fakeWebView) SetHtml(page string) { w.html = page }
func (w *fakeWebView) Eval(js string)      { w.eval = js }
func (w *fakeWebView) Bind(name string, fn interface{}) error {
	w.bindings = append(w.bindings, name)
	if w.bound == nil {
		w.bound = make(map[string]interface{})
	}
	w.bound[name] = fn
	return nil
}
