//go:build windows && !catapp_windows_nocgo

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

var windowsUIIntegration = flag.Bool("cats-windows-ui-integration", false,
	"run hidden native WebView2, cookie, menu, navigation, and clipboard integration tests")

type nativeUIReport struct {
	Path          string `json:"path"`
	Cookie        bool   `json:"cookie"`
	ClipboardRead bool   `json:"clipboard_read"`
	ClipboardOK   bool   `json:"clipboard_ok"`
	Location      string `json:"location"`
}

type nativeE2EReport struct {
	Href          string `json:"href"`
	Ready         string `json:"ready"`
	ClipboardRead bool   `json:"clipboard_read"`
	HasBody       bool   `json:"has_body"`
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
  window.catsUIReport(JSON.stringify({path:location.pathname,
    cookie:document.cookie.includes("cats_native_ui=ok"),
    clipboard_read:typeof window.catsClipRead==="function",clipboard_ok:clipboardOK,
    location:location.href}));
},250);
</script>`)
		case "/persist":
			fmt.Fprint(w, `<!doctype html><script>
window.catsUIReport(JSON.stringify({path:location.pathname,
  cookie:document.cookie.includes("cats_native_ui=ok"),
  clipboard_read:typeof window.catsClipRead==="function",clipboard_ok:true,
  location:location.href}));
</script>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	first := runHiddenNativePage(t, server.URL+"/start")
	if first.Path != "/route" || !first.Cookie || !first.ClipboardRead || !first.ClipboardOK {
		t.Fatalf("first native report = %+v", first)
	}
	if !strings.HasPrefix(first.Location, server.URL+"/route") {
		t.Fatalf("unsafe navigation replaced trusted page: %q", first.Location)
	}
	time.Sleep(500 * time.Millisecond) // allow WebView2's profile store to flush
	second := runHiddenNativePage(t, server.URL+"/persist")
	if second.Path != "/persist" || !second.Cookie || !second.ClipboardRead {
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
	defer func() {
		if err := backend.Stop(context.Background()); err != nil {
			t.Errorf("stop backend: %v", err)
		}
	}()

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
			window.Eval(`window.catsE2EReport(JSON.stringify({href:location.href,
ready:document.readyState,clipboard_read:typeof window.catsClipRead==="function",
has_body:!!document.body}))`)
		})
	})
	window.Navigate(backend.URL())
	window.Run()
	probe.Stop()
	timer.Stop()
	select {
	case report := <-reports:
		if !strings.HasPrefix(report.Href, backend.URL()) || report.Ready != "complete" ||
			!report.ClipboardRead || !report.HasBody {
			t.Fatalf("native WSL report = %+v", report)
		}
	default:
		t.Fatal("native WSL page produced no report")
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
