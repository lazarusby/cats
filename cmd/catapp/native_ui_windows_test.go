//go:build windows && !catapp_windows_nocgo

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/internal/wslclient"
)

var windowsUIIntegration = flag.Bool("cats-windows-ui-integration", false,
	"run hidden native WebView2, cookie, menu, navigation, and clipboard integration tests")

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

func runWSLCatctlProbe(t *testing.T, target wslclient.Target, baseURL, script string) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	args := []string{"--distribution", target.Distribution, "--user", target.User, "--exec",
		target.PayloadPath + "/catctl", "probe", "--url", wsURL, "--timeout", "15s", "--script", script}
	command := exec.CommandContext(t.Context(), "wsl.exe", args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("catctl probe: %v\n%s", err, output)
	}
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
