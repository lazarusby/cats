//go:build windows && !catapp_windows_nocgo

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/internal/wslclient"
)

var windowsUIIntegration = flag.Bool("cats-windows-ui-integration", false,
	"run hidden native WebView2, cookie, menu, navigation, and clipboard integration tests")
var phase8Qualification = flag.Bool("cats-phase8-qualification", false,
	"run the extended Phase 8 Windows/WSL functional and performance qualification")

type nativeUIReport struct {
	Path                   string `json:"path"`
	Cookie                 bool   `json:"cookie"`
	ClipboardRead          bool   `json:"clipboard_read"`
	ClipboardOK            bool   `json:"clipboard_ok"`
	DesktopPlatform        string `json:"desktop_platform"`
	DesktopNativeClipboard bool   `json:"desktop_native_clipboard"`
	NotificationPermission string `json:"notification_permission"`
	Location               string `json:"location"`
}

type nativeE2EReport struct {
	Href               string `json:"href"`
	Ready              string `json:"ready"`
	ClipboardRead      bool   `json:"clipboard_read"`
	ClipboardRoundTrip string `json:"clipboard_round_trip"`
	HasBody            bool   `json:"has_body"`
}

func TestWindowsNativeWebViewIntegration(t *testing.T) {
	if !*windowsUIIntegration {
		t.Skip("pass -cats-windows-ui-integration on a native Windows UI runner")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/start":
			http.SetCookie(w, &http.Cookie{Name: "cats_native_ui", Value: "ok", Path: "/",
				SameSite: http.SameSiteStrictMode, MaxAge: 3600, Expires: time.Now().Add(time.Hour)})
			http.Redirect(w, r, "/route", http.StatusFound)
		case "/route":
			fmt.Fprint(w, `<!doctype html><script>
window.open("javascript:document.body.textContent='unsafe'", "_blank");
location.href="file:///C:/cats-navigation-must-be-denied";
setTimeout(async()=>{
  let clipboardOK=true;
  try { await window.catsClipRead(); } catch (_) { clipboardOK=false; }
  let notificationPermission="unsupported";
  try { notificationPermission=await Notification.requestPermission(); } catch (_) {}
  window.catsUIReport(JSON.stringify({path:location.pathname,
    cookie:document.cookie.includes("cats_native_ui=ok"),
    clipboard_read:typeof window.catsClipRead==="function",clipboard_ok:clipboardOK,
    desktop_platform:window.catsDesktop&&window.catsDesktop.platform,
    desktop_native_clipboard:!!(window.catsDesktop&&window.catsDesktop.nativeClipboard),
    notification_permission:notificationPermission,
    location:location.href}));
},250);
</script>`)
		case "/persist":
			fmt.Fprint(w, `<!doctype html><script>
window.catsUIReport(JSON.stringify({path:location.pathname,
  cookie:document.cookie.includes("cats_native_ui=ok"),
  clipboard_read:typeof window.catsClipRead==="function",clipboard_ok:true,
  desktop_platform:window.catsDesktop&&window.catsDesktop.platform,
  desktop_native_clipboard:!!(window.catsDesktop&&window.catsDesktop.nativeClipboard),
  location:location.href}));
</script>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	first := runHiddenNativePage(t, server.URL+"/start")
	if first.Path != "/route" || !first.Cookie || !first.ClipboardRead || !first.ClipboardOK ||
		first.DesktopPlatform != "windows" || !first.DesktopNativeClipboard ||
		first.NotificationPermission != "granted" {
		t.Fatalf("first native report = %+v", first)
	}
	if !strings.HasPrefix(first.Location, server.URL+"/route") {
		t.Fatalf("unsafe navigation replaced trusted page: %q", first.Location)
	}
	time.Sleep(500 * time.Millisecond) // allow WebView2's profile store to flush
	second := runHiddenNativePage(t, server.URL+"/persist")
	if second.Path != "/persist" || !second.Cookie || !second.ClipboardRead ||
		second.DesktopPlatform != "windows" || !second.DesktopNativeClipboard {
		t.Fatalf("persisted native report = %+v", second)
	}

	t.Run("clipboard contention", func(t *testing.T) {
		opened := make(chan struct{})
		release := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if result, _, _ := openClipboardProc.Call(0); result == 0 {
				close(opened)
				return
			}
			close(opened)
			<-release
			closeClipboardProc.Call()
		}()
		<-opened
		time.AfterFunc(45*time.Millisecond, func() { close(release) })
		if _, err := readWindowsClipboard(); err != nil {
			t.Fatalf("clipboard read did not survive bounded contention: %v", err)
		}
	})
}

func TestWindowsNativeWSLEndToEnd(t *testing.T) {
	if !*windowsUIIntegration || !*windowsWSLIntegration {
		t.Skip("pass both native UI and WSL integration flags")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	target := integrationWSLTarget()
	backend, err := startLocalBackend(t.Context(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	backendStopped := false
	defer func() {
		if !backendStopped {
			if err := backend.Stop(context.Background()); err != nil {
				t.Errorf("stop backend: %v", err)
			}
		}
	}()

	// Exercise the real browser protocol and PTY before inspecting the native
	// window: create a pane, run commands in both panes, and close the new pane.
	runWSLCatctlProbe(t, target, backend.URL(),
		`wait:800;type:printf CATS_PHASE7_RESTORE\n;expect:1:CATS_PHASE7_RESTORE;split:1:v;panes:2;typeat:2:printf CATS_PHASE7_SECOND\n;expect:2:CATS_PHASE7_SECOND;close:2;panes:1`)

	window := newWindow("cats native WSL integration").(*nativeWindow)
	defer window.Destroy()
	showWindowProc.Call(uintptr(window.w.Window()), 0)
	reports := make(chan nativeE2EReport, 1)
	if err := window.Bind("catsE2EReport", func(raw string) {
		var report nativeE2EReport
		if json.Unmarshal([]byte(raw), &report) == nil {
			reports <- report
		}
		window.w.Terminate()
	}); err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(10*time.Second, window.w.Terminate)
	probe := time.AfterFunc(1200*time.Millisecond, func() {
		window.Dispatch(func() {
			window.Eval(`(async()=>{let clip="";try{await window.catsClipWrite("CATS_PHASE7_CLIPBOARD");clip=await window.catsClipRead();}catch(_){}
window.catsE2EReport(JSON.stringify({href:location.href,ready:document.readyState,
clipboard_read:typeof window.catsClipRead==="function",clipboard_round_trip:clip,
has_body:!!document.body}))})()`)
		})
	})
	window.Navigate(backend.URL())
	window.Run()
	probe.Stop()
	timer.Stop()
	select {
	case report := <-reports:
		if !strings.HasPrefix(report.Href, backend.URL()) || report.Ready != "complete" ||
			!report.ClipboardRead || report.ClipboardRoundTrip != "CATS_PHASE7_CLIPBOARD" || !report.HasBody {
			t.Fatalf("native WSL report = %+v", report)
		}
	default:
		t.Fatal("native WSL page produced no report")
	}

	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	backendStopped = true

	// A fresh helper/catway pair must cold-restore the surviving pane and its
	// captured command output from the first launch.
	restored, err := startLocalBackend(t.Context(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restored.Stop(context.Background()); err != nil {
			t.Errorf("stop restored backend: %v", err)
		}
	}()
	runWSLCatctlProbe(t, target, restored.URL(), `wait:800;panes:1;expect:1:CATS_PHASE7_RESTORE`)
}

func TestWindowsNativeWSLPhase8Qualification(t *testing.T) {
	if !*phase8Qualification || !*windowsWSLIntegration {
		t.Skip("pass Phase 8 and WSL integration flags on the dedicated Windows/WSL runner")
	}
	target := integrationWSLTarget()
	startedAt := time.Now()
	backend, err := startLocalBackend(t.Context(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	startup := time.Since(startedAt)
	stopped := false
	defer func() {
		if !stopped {
			_ = backend.Stop(context.Background())
		}
	}()

	parsed, err := url.Parse(backend.URL())
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("local backend is not loopback-only: %q (%v)", backend.URL(), err)
	}
	assertWSLLoopbackListener(t, target, parsed.Port())

	viewer := wslCatctlProbeCommand(t, target, backend.URL(),
		`wait:500;clients:2:1;wait:2500`, true)
	var viewerOutput strings.Builder
	viewer.Stdout = &viewerOutput
	viewer.Stderr = &viewerOutput
	if err := viewer.Start(); err != nil {
		t.Fatal(err)
	}

	functionalAt := time.Now()
	script := `wait:500;clients:2:1;type:printf CATS_PHASE8_PERSIST\n;expect:f:CATS_PHASE8_PERSIST;` +
		`rename:f:phase8-main;title:f:phase8-main;zoom:f;zoom:f;resize:160:48;` +
		`split:f:v;split:f:h;split:f:v;split:f:h;split:f:v;split:f:h;split:f:v;panes:8;` +
		`type:seq 1 5000\n;expect:f:5000;` +
		`close:f;close:f;close:f;close:f;close:f;close:f;close:f;panes:1;` +
		`tabnew;tabs:2;tabmove:2:0;tabfocus:1;tabclose:2;tabs:1;` +
		`wsnew;workspaces:2;wsrename:w2:phase8;wsmove:w2:0;wslock:w2:on;wslock:w2:off;` +
		`wsfocus:w1;wsclose:w2;workspaces:1`
	runWSLCatctlProbe(t, target, backend.URL(), script)
	functional := time.Since(functionalAt)
	if err := viewer.Wait(); err != nil {
		t.Fatalf("viewer probe: %v\n%s", err, viewerOutput.String())
	}

	if err := backend.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped = true
	restoreAt := time.Now()
	restored, err := startLocalBackend(t.Context(), appConfig{Mode: "local", WSL: target})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restored.Stop(context.Background()); err != nil {
			t.Errorf("stop restored backend: %v", err)
		}
	}()
	runWSLCatctlProbe(t, target, restored.URL(), `wait:500;panes:1;tabs:1;workspaces:1;expect:1:CATS_PHASE8_PERSIST`)
	restore := time.Since(restoreAt)

	if startup > 20*time.Second || functional > 30*time.Second || restore > 20*time.Second {
		t.Fatalf("Phase 8 timing budget exceeded: startup=%s functional=%s restore=%s", startup, functional, restore)
	}
	metrics, _ := json.Marshal(map[string]any{
		"startup_ms": startup.Milliseconds(), "functional_ms": functional.Milliseconds(),
		"restore_ms": restore.Milliseconds(), "scroll_lines": 5000,
		"peak_panes": 8, "simultaneous_clients": 2,
	})
	t.Logf("PHASE8_METRICS %s", metrics)
}

func assertWSLLoopbackListener(t *testing.T, target wslclient.Target, port string) {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), "wsl.exe", "--distribution", target.Distribution,
		"--user", target.User, "--exec", "/usr/bin/ss", "-ltnH").Output()
	if err != nil {
		t.Fatalf("inspect WSL listeners: %v", err)
	}
	wantSuffix := ":" + port
	found := false
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[3], wantSuffix) {
			continue
		}
		found = true
		if !strings.HasPrefix(fields[3], "127.0.0.1:") {
			t.Fatalf("catway port %s has non-loopback WSL listener %q", port, fields[3])
		}
	}
	if !found {
		t.Fatalf("catway port %s was absent from WSL listener table", port)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("backend URL had invalid port %q", port)
	}
}

func runWSLCatctlProbe(t *testing.T, target wslclient.Target, baseURL, script string) {
	t.Helper()
	command := wslCatctlProbeCommand(t, target, baseURL, script, false)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("catctl probe: %v\n%s", err, output)
	}
}

func wslCatctlProbeCommand(t *testing.T, target wslclient.Target, baseURL, script string, viewer bool) *exec.Cmd {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	args := []string{"--distribution", target.Distribution, "--user", target.User, "--exec",
		target.PayloadPath + "/catctl", "probe", "--url", wsURL, "--timeout", "15s", "--script", script}
	if viewer {
		args = append(args, "--viewer")
	}
	return exec.CommandContext(t.Context(), "wsl.exe", args...)
}

func runHiddenNativePage(t *testing.T, target string) nativeUIReport {
	t.Helper()
	window := newWindow("cats native integration").(*nativeWindow)
	defer window.Destroy()
	if menu, _, _ := getMenuProc.Call(uintptr(window.w.Window())); menu == 0 {
		t.Fatal("native Windows menu was not installed")
	}
	appCleanup = cleanupController{}
	sessionCleanup := 0
	registerCleanup(func() { sessionCleanup++ })
	if result, _, _ := sendMessageProc.Call(uintptr(window.w.Window()), wmQueryEndSession, 0, 0); result != 1 {
		t.Fatalf("WM_QUERYENDSESSION result = %d, want 1", result)
	}
	if sessionCleanup != 1 {
		t.Fatalf("WM_QUERYENDSESSION cleanup calls = %d, want 1", sessionCleanup)
	}
	appCleanup = cleanupController{}
	showWindowProc.Call(uintptr(window.w.Window()), 0)
	reports := make(chan nativeUIReport, 1)
	if err := window.Bind("catsUIReport", func(raw string) {
		var report nativeUIReport
		if err := json.Unmarshal([]byte(raw), &report); err == nil {
			select {
			case reports <- report:
			default:
			}
		}
		window.w.Terminate()
	}); err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(10*time.Second, window.w.Terminate)
	window.Navigate(target)
	window.Run()
	timer.Stop()
	select {
	case report := <-reports:
		return report
	default:
		t.Fatalf("native page %s produced no report", target)
		return nativeUIReport{}
	}
}
