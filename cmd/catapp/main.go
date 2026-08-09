//go:build darwin || windows || catapp_headless

// Command catapp is the native desktop launcher for cats: a thin Go
// supervisor around a native webview window (github.com/webview/webview_go). It has two
// runtime modes, chosen by the build-time defaultMode and overridable per user
// in app.json:
//
//   - local  — supervise the in-bundle daemons (cathost -persistent + catway
//     --auth none on loopback) and show their UI in the window. Fully offline;
//     this is the "self-contained" Cats.app (make macapp).
//   - remote — a thin client: start no daemons, point the window at a remote
//     catway URL (a relay host or a direct LAN/VPN address). The catway's own
//     login page collects the password and the webview persists the session
//     cookie across launches. This is Cats Client.app (make macapp-client).
//
// The launcher itself is plain Go (no -tags ghostty) — it only supervises
// processes and shows a window; the terminal/VT work lives in the platform's
// local backend. Platform adapters own menus, bridges, signals, configuration
// paths, and local-backend startup.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"runtime"
	"strings"
)

// defaultMode is the build-time default mode: "local" for the self-contained
// Cats.app, "remote" for the thin client. app.json overrides it at runtime.
// Injected via -ldflags "-X main.defaultMode=local|remote"; the var default
// keeps a plain `go build`/`go run` (development) in local mode.
var defaultMode = "local"

// Window geometry. Roomy default that still fits a laptop; the user can resize.
const (
	windowTitle  = "cats"
	windowWidth  = 1280
	windowHeight = 820
)

var (
	desktopWindowFactory = newDesktopWindow
	localBackendStarter  = startLocalBackend
)

func main() {
	// Native webviews require UI calls on the process's main thread. Run then
	// blocks there until the window closes.
	runtime.LockOSThread()
	log.SetFlags(0)
	log.SetPrefix("catapp: ")

	cfg := loadAppConfig()
	if isRemoteMode(cfg) {
		runRemote(cfg)
	} else { // "local" and any unrecognised value
		runLocal(cfg)
	}
}

func isRemoteMode(cfg appConfig) bool { return cfg.Mode == "remote" }

// runLocal supervises the in-bundle daemons and shows the UI they serve on
// loopback. The backend is reaped when the window closes (Run returns), on a
// Cmd-Q, or on a termination signal — all routed through runCleanup.
func runLocal(cfg appConfig) {
	// Platform environment preparation runs before any child exists. On Darwin
	// this hydrates Finder's bare PATH; Windows local startup is handled later by
	// the WSL host and currently leaves the launcher environment unchanged.
	hydratePlatformEnvironment()

	b, err := localBackendStarter(context.Background(), cfg)
	if err != nil {
		if handleLocalBackendError(cfg, err) {
			return
		}
		showError("Could not start cats", err.Error())
		return
	}
	registerCleanup(backendCleanup(b))
	defer runCleanup()
	installSignalHandler()

	w := newWindow(windowTitle)
	defer w.Destroy()
	w.Navigate(b.URL())
	w.Run()
}

// runRemote is the thin-client path: no local daemons, just point the window at
// a remote catway. On first run (no saved URL) it shows a small connect form;
// the bound catsConnect callback persists the entered URL and navigates the
// same window to it, so subsequent launches connect straight away.
func runRemote(cfg appConfig) {
	installSignalHandler() // no daemons to reap, but honour a clean quit uniformly

	if cfg.Remote.URL != "" {
		w := newWindow(remoteTitle(cfg.Remote.URL))
		defer w.Destroy()
		w.Navigate(cfg.Remote.URL)
		w.Run()
		return
	}

	w := newWindow(windowTitle)
	defer w.Destroy()
	if err := w.Bind("catsConnect", func(rawURL string) {
		u := strings.TrimSpace(rawURL)
		if u == "" {
			return
		}
		cfg.Remote.URL = u
		if err := saveAppConfig(cfg); err != nil {
			log.Printf("could not save connection choice: %v", err)
		}
		// Navigate on the UI thread; the callback runs off it.
		w.Dispatch(func() {
			w.SetTitle(remoteTitle(u))
			w.Navigate(u)
		})
	}); err != nil {
		showError("Could not initialise the connect form", err.Error())
		return
	}
	w.SetHtml(connectPageHTML)
	w.Run()
}

// newWindow builds the shared webview window, then lets the platform install
// its menu and native bridges. Debug is false in the shipped app.
func newWindow(title string) desktopWindow {
	w := desktopWindowFactory(false)
	uiWindow = w
	installMenu(w)
	w.SetTitle(title)
	w.SetSize(windowWidth, windowHeight, sizeHintNone)
	if err := bindPlatformBridges(w); err != nil {
		log.Printf("native bridge unavailable: %v", err)
	}
	return w
}

// uiWindow is the single UI window, kept for the View menu's zoom actions.
// Menu actions fire only while Run() is blocking, so the reference is valid
// whenever zoomFont is reached.
var uiWindow desktopWindow

// zoomFont steps the terminal font size in the page (+1/-1, 0 = reset) by
// calling the hook the UI exposes for exactly this path — see catappZoom in
// menu_darwin.go for why the native menu owns ⌘+/⌘-/⌘0. The guard on the JS
// side keeps this a no-op on pages without the hook (connect form, login).
func zoomFont(delta int) {
	w := uiWindow
	if w == nil {
		return
	}
	w.Dispatch(func() {
		w.Eval(fmt.Sprintf("window.catsAdjustFont && window.catsAdjustFont(%d)", delta))
	})
}

// remoteTitle labels the window with the connected host so a thin client that
// can point anywhere shows where it is pointing. Falls back to the bare title
// if the URL won't parse.
func remoteTitle(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return windowTitle + " — " + u.Host
	}
	return windowTitle
}

// showError opens a small, self-contained window describing a startup failure.
// A double-clicked .app has no console, so surfacing the reason in a window is
// the only way the user sees why nothing opened. Also logged for a dev terminal.
func showError(title, detail string) {
	log.Printf("%s: %s", title, detail)
	w := desktopWindowFactory(false)
	installMenu(w)
	defer w.Destroy()
	w.SetTitle("cats — error")
	w.SetSize(560, 320, sizeHintFixed)
	w.SetHtml(errorPageHTML(title, detail))
	w.Run()
}
