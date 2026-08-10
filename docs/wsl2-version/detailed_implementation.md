# Detailed WSL2/Windows implementation plan

- Status: proposed
- Primary languages: Go for application/runtime code; PowerShell only for
  Windows installation glue; existing HTML/CSS/JavaScript for the embedded UI;
  existing Zig build for vendored libghostty-vt.

This document expands the high-level plan in chronological order. Names for new
internal packages and protocol fields are proposed and may be adjusted during
implementation, but their responsibilities should remain narrow.

## Phase 0 — baseline and contracts

Implementation status (2026-08-09): **in progress**. The current source/build
baseline, initial Windows/WSL support matrix, browser-probe identity, and parity
checklist are captured in [phase0_baseline.md](phase0_baseline.md) and
[windows_parity_checklist.md](windows_parity_checklist.md). The version 1
launcher/host contract is implemented and tested in `internal/desktopproto`.
Linux checks pass; a macOS builder and visual recording remain outstanding.

### 0.1 Capture the baseline

- Record the current commit and output of `make check`, `make binaries`,
  `make macapp`, and `make macapp-client` on supported Linux/macOS builders.
- Archive the current browser protocol probe script and a representative visual
  walkthrough: open workspace, split, type, selection/copy/paste, copy mode,
  agent detection, hook state, worktree, plugin, chat, notification, save,
  restart, and restore.
- Convert that walkthrough into the Windows parity checklist. Every item must
  name where it runs: Windows launcher, WebView2 page, `catway`, `cathost`, or a
  WSL child process.

### 0.2 Fix the support matrix

Before shipping, decide and encode:

- minimum Windows 11 build and minimum Store WSL version;
- initial WSL distributions (recommend one Ubuntu LTS baseline for v1);
- `amd64` only versus `amd64` plus `arm64`;
- required WebView2 runtime version;
- NAT and mirrored networking coverage;
- whether local app close stops all daemons (recommended for Mac parity) or
  leaves a background host (future option).

Current decision: the initial matrix is Windows 11/amd64 with Store WSL2 and
Ubuntu 24.04.3 LTS/amd64. The reference versions (Windows build 26200, WSL
2.7.11.0, WebView2 151.0.4129.72) are the presently qualified floor; no lower
version is claimed yet. Mirrored networking is qualified first and NAT is a
release gate. Closing the local app stops its helper and daemons. See D-009,
D-013, and D-014.

### 0.3 Define the launcher/host protocol

Add `internal/desktopproto` as a standard-library-only JSON-lines contract. It
must be versioned independently of the existing browser, orchestration, control,
and hook protocols.

Proposed helper-to-launcher records:

```json
{"v":1,"type":"starting","app_version":"v0.x","pid":123}
{"v":1,"type":"ready","app_version":"v0.x","addr":"127.0.0.1:49152","pid":123}
{"v":1,"type":"error","stage":"catway","message":"exited before readiness"}
{"v":1,"type":"stopped","reason":"requested"}
```

Proposed launcher-to-helper records:

```json
{"v":1,"type":"stop"}
```

Rules:

- stdout contains protocol records only; all human logs go to stderr;
- one bounded JSON object per line, with a read deadline during startup;
- unknown additive fields are ignored; unknown versions are fatal;
- no secrets, clipboard data, terminal bytes, or arbitrary commands cross it;
- stdin EOF means the Windows owner disappeared and triggers cleanup;
- the helper reports the same build stamp as its sibling daemons;
- `Cats.exe` refuses a mismatched version before showing the main page.

Tests go in `internal/desktopproto/*_test.go`, including truncation, oversized
records, malformed JSON, unknown version/type, EOF, and partial reads.

Implemented contract details:

- protocol version 1 and a 16 KiB maximum JSON-object size;
- a trailing newline is mandatory (EOF after partial JSON is truncation);
- helper and launcher directions use distinct record types;
- `ready.addr` must be an explicit `127.0.0.1:<port>` endpoint;
- additive unknown JSON fields are ignored, while unknown versions/types and
  cross-type known fields are rejected;
- encoders validate before writing and finish short writes correctly.

## Phase 1 — platform feasibility spikes

Implementation status (2026-08-09): **in progress**. Disposable probes live in
`spikes/wsl2`. The explicit `wsl.exe` execution, Windows localhost, graceful
stop, stdin-EOF, and mirrored-network port-conflict probes pass on the reference
host. The existing CATS backend was also used successfully from Windows Chrome.
Native WebView2, clipboard, keyboard, notification, and external-navigation
results are pending because the Windows Go/UCRT64 toolchain is not installed.
See [implementation_log.md](implementation_log.md) for exact evidence.

Keep spikes outside the production package or behind a temporary build tag and
remove them when their results are captured in the decision log.

### 1.1 WebView2 build

- On a Windows runner with a supported C++ toolchain, build a minimal program
  using the currently pinned `github.com/webview/webview_go`.
- Confirm `Bind`, `Dispatch`, `Eval`, `Navigate`, window resize, cookies,
  WebSocket, and teardown.
- Build with `-trimpath` and `-ldflags "-H windowsgui"`.
- Verify behavior when WebView2 Runtime is unavailable or damaged.
- Update the Go dependency only if the pinned version fails a required Windows
  behavior; record the reason and exact version in the decision log.

Current result: the build-tagged spike is implemented at
`spikes/wsl2/webview2`, and the installed Evergreen Runtime is
151.0.4129.72. Build/run is pending installation of native Windows Go and
UCRT64 MinGW. The pinned dependency remains unchanged under D-016.

### 1.2 WSL execution and network

- Enumerate distributions with `wsl.exe --list --quiet`; treat output as
  untrusted Unicode and avoid locale-dependent parsing where possible.
- Start a long-lived helper with explicit `--distribution`, `--user`, and
  `--exec` arguments. Do not use `cmd.exe /c`, and do not assemble a shell
  command from a distribution name.
- Test startup and Windows `http://127.0.0.1:<port>` access under NAT and
  mirrored networking, with VPN/firewall software in the qualification matrix.
- Test conflicts where the port is free in Windows but busy in WSL and vice
  versa. The launcher should retry a bounded number of complete startups.
- Verify pipe EOF and graceful-stop behavior on normal close, Task Manager kill,
  `wsl --terminate <distro>`, and `wsl --shutdown`.

Current result: direct distribution/user/exec startup, Windows HTTP access,
protocol pipes, requested stop, stdin EOF, and both cross-namespace port
conflict directions pass under mirrored networking. NAT, Task Manager kill,
`wsl --terminate`, and `wsl --shutdown` remain pending and will be run without
risking the shared development distribution. The rerunnable driver is
`spikes/wsl2/probe-wsl.ps1`.

### 1.3 Clipboard, keyboard, and notifications

- Prototype `catsClipRead` and `catsClipWrite` with Win32 `CF_UNICODETEXT`.
  Prefer `golang.org/x/sys/windows` over hand-maintained syscall declarations;
  keep the dependency Windows-only by file/build tag.
- Test CRLF/LF, emoji, combining characters, NUL rejection, large selections,
  clipboard contention, and remote desktop clipboard redirection.
- Capture WebView2 key events for Control, Alt, Shift, Windows, AltGr, dead
  keys, IME composition, numpad, and common non-US layouts.
- Validate Web Notifications when the window is hidden/unfocused and validate
  click-to-focus. If unreliable, prove a native Windows notification bridge.
- Validate `window.open`/new-window behavior. OSC 8 links must open in the
  default Windows browser without replacing the trusted CATS WebView or
  inheriting its native JavaScript bindings.

Current result: the WebView2 spike contains manual keyboard, notification,
new-window, and teardown surfaces, but they have not run. The Go/Win32 Unicode
clipboard prototype is also pending the native Windows toolchain; no result is
claimed from browser-only clipboard behavior.

## Phase 2 — make `cmd/catapp` cross-platform safely

The refactor must land in small commits with macOS green after each commit.

Implementation status (2026-08-09): **structural refactor implemented; native
verification pending**. Common mode, window, config, lifecycle, page, and
backend-cleanup flow now builds for `darwin || windows`; Darwin-only behavior is
isolated in suffixed files. A `catapp_headless` adapter runs the common tests on
Linux/WSL without importing the cgo webview. Windows local mode deliberately
returns an explicit not-implemented error until Phase 4; no incomplete WSL
launcher or privileged bridge is exposed. A Darwin builder is still required to
run `make macapp`, `make macapp-client`, and the native Cocoa/supervision tests.

### 2.1 Split common and platform files

Refactor `cmd/catapp` approximately as follows:

```text
cmd/catapp/
  main.go                 common window/mode flow (`darwin || windows`)
  pages.go                common connect and error HTML
  lifecycle.go            common cleanup-once state
  config.go               common structs/load/save
  config_darwin.go        macOS app-data path
  config_windows.go       LocalAppData path and WSL fields
  clipboard_darwin.go     current pbcopy/pbpaste implementation
  clipboard_windows.go    Win32 Unicode clipboard
  menu_darwin.go/.m       current Cocoa menu
  menu_windows.go         Windows menu/accelerator adapter
  signal_darwin.go        Unix signal behavior
  signal_windows.go       Windows console/session-close behavior where available
  supervise_darwin.go     current direct sibling supervision
  supervise_windows.go    wsl.exe/helper supervision
  shellenv_darwin.go      current LaunchServices PATH hydration
```

Exact names may differ, but common files must not import `syscall` structures,
Cocoa, or Windows packages. Preserve the `defaultMode=local|remote` link-time
variable so the same launcher can still be built as a thin client.

Implemented layout adds `backend.go`, `window.go`, and `window_native.go` for
the two narrow common interfaces; `platform_headless.go` and headless-only tests
are test infrastructure, not a Linux launcher. Windows now owns
`%LOCALAPPDATA%\Cats` path resolution, console-interrupt handling, no-op Phase 2
menu/bridge adapters, and a fail-closed local-backend stub. WSL selection fields
remain Phase 4 configuration work because Phase 2 does not yet discover or
validate a distribution.

### 2.2 Add platform interfaces

Keep the surface small and local to `cmd/catapp`, for example:

```go
type localBackend interface {
    URL() string
    Stop(context.Context) error
}

type desktopWindow interface {
    Run()
    Dispatch(func())
    Destroy()
    SetTitle(string)
    SetSize(int, int, sizeHint)
    Navigate(string)
    SetHtml(string)
    Eval(string)
    Bind(string, interface{}) error
}

func startLocalBackend(context.Context, appConfig) (localBackend, error)
func appDataDir() (string, error)
func installMenu(desktopWindow)
func bindPlatformBridges(desktopWindow) error
```

Do not introduce a general dependency-injection framework. Package variables or
small interfaces are sufficient for process and filesystem fakes in tests.

### 2.3 Preserve macOS

- Rename/move existing files without changing behavior first.
- Keep Darwin-only Objective-C compilation and menu callbacks.
- Retain byte-identical daemon packaging semantics where practical.
- Run the existing shell-environment tests and add missing tests for config,
  supervision, readiness, cleanup order, and remote mode.

Current result: `make test-catapp-common` passes in WSL and is part of
`make check`. It covers config round trips/fallbacks/permissions, mode choice,
remote saved-target and first-run flows, HTML escaping, zoom dispatch,
cleanup-once behavior, local navigation, and window-destroy/backend-stop order.
Darwin-only tests cover app-data location, clipboard binding names, readiness,
and real subprocess stop order (`catway` before `cathost`), but cannot execute on
this WSL host. Existing shell-environment tests remain Darwin-only and unchanged
apart from their platform-specific filename.

## Phase 3 — implement `cats-wsl-host`

Implementation status (2026-08-09): **implemented and verified on the reference
WSL2 host**. The Linux-only helper validates its payload and WSL environment,
hydrates the selected Linux user's environment, creates private bounded runtime
paths, starts and monitors the two daemons, speaks `desktopproto` on its
standard streams, and performs bounded ordered teardown. Unit and fake-process
integration coverage passes, as does a smoke test with the real tagged
`catway`/`cathost` payload. Native Windows launcher integration and abrupt
Windows-owner loss now pass against the real payload. Ignoring `SIGPIPE` lets
the helper finish its exact runtime cleanup after the dead owner closes both
protocol pipes. `wsl --terminate` and `wsl --shutdown` qualification are
implemented but still require an explicitly approved idle WSL host because
they intentionally stop distribution-wide workloads.

### 3.1 Add the command

Create `cmd/cats-wsl-host/main.go` with a Linux build constraint. Add it to a
separate `WSL_BINS`/payload list, not the ordinary three public binaries unless
there is a reason to expose it to all Linux users.

Supported flags should be narrow:

```text
cats-wsl-host --port <windows-selected-port> --launch-id <random-id>
              [--start-dir <absolute-linux-path>]
              [--idle-timeout <duration>]
```

It locates `catway` and `cathost` beside its own executable, just as macOS
`catapp` does. It never trusts `PATH` for the packaged daemons.

Implemented result: `cmd/cats-wsl-host` has Linux build constraints and accepts
the proposed flags. The only additional operational modes are `--health-json`
and the explicit development-only `--allow-non-wsl` override. `Makefile` keeps
the existing `BINS` list unchanged and adds `WSL_BINS := cats-wsl-host`; the
new `make wsl-payload` target builds the ordinary three binaries plus the
stamped helper.

### 3.2 Validate the environment

- Require `runtime.GOOS == linux` by build tag.
- Check `/proc/sys/kernel/osrelease` or `/proc/version` for Microsoft/WSL and
  return a useful diagnostic if invoked on ordinary Linux by mistake. Permit a
  test/development override rather than baking detection into core behavior.
- Report architecture, helper version, home, and payload path through a
  `--health-json` mode used by the installer/launcher.
- Verify the three sibling binaries exist and are executable.
- Refuse a relative start directory or socket directory.

Implemented result: both proc files are checked case-insensitively for a WSL or
Microsoft marker. Normal startup requires `catway`, `cathost`, and `catctl` to
be regular executable sibling files before any child starts. Health mode emits
one JSON object with `architecture`, `helper_version`, `home`, and
`payload_path`. Port, launch token, duration, positional arguments, and start
directory validation fail closed.

### 3.3 Hydrate the Linux user environment

`wsl.exe --exec` does not represent an interactive login shell. Before starting
the daemons, derive the selected user's shell and its login/interactive `PATH`
with the same bounded, marker-based approach used by macOS
`shellenv_darwin.go`.

- Determine a safe default shell from `SHELL`, then the user's passwd entry,
  then `/bin/sh`.
- Merge the derived `PATH` ahead of the inherited interop path so Linux agent
  and plugin tools are found while intentional Windows interop entries survive.
- Never evaluate captured output as shell syntax.
- Bound startup time and fall back to the inherited environment on failure.
- Ensure `HOME`, `USER`, `LOGNAME`, and `SHELL` correspond to the selected WSL
  user. Do not copy arbitrary Windows secrets into WSL.

Move reusable marker/merge functions into a small internal package if that
avoids copying; keep platform launch-detection policy in platform files.

Implemented result: `internal/shellenv` now owns only marker extraction and
stable PATH merging; both Darwin `catapp` and the WSL helper use it. The helper
chooses an absolute executable shell from `SHELL`, the selected UID's passwd
entry, then `/bin/sh`; runs one bounded `-ilc` marker probe; preserves inherited
interop-only PATH entries behind Linux shell entries; and explicitly normalizes
`HOME`, `USER`, `LOGNAME`, and `SHELL`. Failure to derive PATH is logged to
stderr and retains the normalized inherited environment.

### 3.4 Allocate runtime paths

- Prefer `$XDG_RUNTIME_DIR/cats/<launch-id>` only when it exists, belongs to the
  user, and is not group/world-accessible.
- Otherwise create a mode-0700 directory under `os.TempDir()` containing the
  numeric UID and a random short suffix.
- Keep each Unix socket path below the platform limit; use short names such as
  `th.sock`, `ctl.sock`, and `hook.sock`.
- Reject a user-supplied launch id that is not a bounded ASCII token.
- Remove the directory after all children exit; never recursively remove an
  unresolved or broad path.

Implemented result: launch IDs are 1–64 ASCII alphanumeric/underscore/hyphen
tokens. A private, selected-UID-owned `XDG_RUNTIME_DIR` uses
`cats/<launch-id>`; any ownership, permission, collision, or socket-length
failure falls back to a mode-0700 `cats-wsl-<uid>-<random>` temp directory.
All three names are checked against Linux's 107-byte usable Unix-socket limit.
Cleanup validates the exact absolute directory and exact socket children,
unlinks only those three names, and removes the directory non-recursively.

### 3.5 Start and supervise daemons

1. Start sibling `cathost -persistent -socket <th.sock>` in its own process
   group.
2. Start sibling `catway --addr 127.0.0.1:<port> --auth none` with all three
   explicit socket paths and the intended WSL start directory.
3. Capture stdout/stderr to the helper's stderr/log stream, never protocol
   stdout.
4. Poll TCP readiness inside WSL and detect early child exit concurrently.
5. Emit `ready`; continue monitoring stdin, signals, and both children.
6. If `catway` dies unexpectedly, report the error and stop `cathost` for
   standalone parity. A future restart policy can be a separate decision.
7. On stop/EOF/signal, signal `catway`, allow its save/final-capture budget,
   then signal `cathost`, wait, escalate only after bounded grace periods, reap
   both, and remove runtime paths.

Tests should use fake child processes and real temporary Unix sockets. Include
partial startup, readiness timeout, duplicate stop, EOF, signal, child exit,
unresponsive child, and cleanup-path safety.

Implemented result: each child has its own process group and one wait/reap
owner. Readiness polling observes both early-exit channels. Version 1 stop,
stdin EOF, SIGINT/SIGTERM/SIGHUP, malformed input, and either child exit all
enter a single teardown path. Teardown sends SIGTERM to `catway`, waits up to
five seconds for save/final capture, escalates its process group if needed,
then gives `cathost` three seconds and applies the same bound. There is no
automatic restart policy. Tests cover every case listed above, including a
real Unix socket and an unresponsive fake child; a production smoke emitted
`starting`, `ready`, and `stopped(requested)` and left neither child processes
nor its runtime directory behind.

## Phase 4 — Windows launcher implementation

Implementation status (2026-08-09): **core Windows/WSL launch path and native
desktop security boundary implemented and verified**. Configuration, distribution
discovery, first-run/repair selection, helper health, direct argument-vector
startup, bounded protocol readiness, build identity, Windows-side HTTP
verification, classified retry, rotating/redacted logs, and ordered stop are in
place. The native Go/UCRT64 toolchain built the real WebView2 launcher on the
reference Windows host. Hidden native tests exercised WebView2, menus,
session-ending delivery, persistent cookies, clipboard read/contention, and the
navigation/new-window boundary. A final native WebView loaded the real
build-matched WSL-backed CATS page and stopped it cleanly. The privileged
clipboard bindings are enabled only after native navigation hooks attach.

### 4.1 Windows configuration

Extend the launcher config without changing remote-mode compatibility:

```json
{
  "mode": "local",
  "wsl": {
    "distribution": "Ubuntu-24.04",
    "user": "alice",
    "payload_path": "/home/alice/.local/lib/cats/current"
  },
  "remote": {
    "url": "https://host.example:8421",
    "label": "home"
  }
}
```

- Store it at `%LOCALAPPDATA%\Cats\app.json` with user-only ACLs where
  practical and atomic replace semantics.
- Never put passwords, tokens, clipboard data, Windows environment dumps, or
  WSL shell output in it.
- A malformed file logs and falls back to first-run selection, as macOS does.
- Validate distribution/user/path on every local start because distributions
  can be renamed, removed, imported, or reset independently of Windows files.

Implemented result: `appConfig` now adds the proposed `wsl` target without
changing the remote object. Saves stage a 0600 sibling file, sync it, and use
same-volume replacement; Windows uses `MoveFileExW` with replace/write-through
flags and inherits the per-user `%LOCALAPPDATA%` DACL. The target contains only
distribution, user, and absolute Linux payload path. Windows-executed tests
passed round trip, malformed fallback, replacement, and LocalAppData behavior.

### 4.2 First-run and repair flow

If local mode has no valid target:

1. Verify `wsl.exe` exists and WSL responds.
2. List candidate distributions and let the user select one; do not silently
   install or modify a distribution from ordinary app startup.
3. Run the helper health command in the selected distribution/user.
4. If the payload is absent or stale, show a repair/install action with the
   exact problem and launch the installer explicitly.
5. Save only after health succeeds.

The launcher should never open a blank WebView while waiting on a slow WSL
startup. Show a local, self-contained starting page with the current stage and
a cancel/diagnostics path.

Implemented result: `wsl.exe --list --quiet` is bounded and decoded as either
UTF-8 or redirected UTF-16LE. Missing, renamed, or unhealthy targets enter a
self-contained setup/repair page listing discovered distributions and accepting
the Linux user and absolute payload directory. Submission shows a starting
page, reruns health/startup, and saves only after success. Ordinary startup
does not silently install, import, reset, or choose a distribution. Windows now
starts valid and first-run targets asynchronously behind one persistent,
cancellable progress window. Retry, Change Distribution, Open Logs, and Repair
are bound only while that local startup UI is active. Repair launches a packaged
`Install-Cats.ps1` explicitly when present and reports a precise missing-installer
error until Phase 6 supplies that release asset.

### 4.3 Safe WSL process construction

Use `exec.CommandContext` with individual arguments, conceptually:

```text
wsl.exe --distribution <distro> --user <user> --exec
  /home/.../.local/lib/cats/current/cats-wsl-host
  --port <port> --launch-id <random>
```

- Do not use `sh -c` in normal launch.
- Pass an absolute Linux helper path because `--exec` does not perform shell
  tilde expansion.
- Keep a writable stdin pipe and bounded stdout protocol reader; stream stderr
  to the launcher log with redaction/rotation.
- Use a Windows Job Object if testing shows it improves `wsl.exe` proxy cleanup,
  but do not assume killing the proxy gracefully signals Linux descendants.
  The protocol stop/EOF path remains authoritative.
- Retry with a new port only for classified bind/forwarding failures, not for
  missing payload, bad version, or daemon crashes.

Implemented result: `internal/wslclient` validates untrusted target text,
rejects relative/root/`/mnt` payload locations, constructs individual arguments,
and never emits a shell/cmd wrapper. The Windows adapter keeps stdin/stdout
pipes, bounds startup at 20 seconds, and directs helper stderr through a
1 MiB rotating LocalAppData log with credential-shaped values redacted. It
tries at most three complete startups, with a fresh cryptographic launch ID and
Windows-reserved loopback port each time, only for helper `bind` or Windows HTTP
forwarding classifications. Phase 3 now detects the specific early
`address already in use` signal. No Job Object was added without native evidence;
protocol stop/EOF remains authoritative and forced proxy kill is only a bound.
An abrupt-owner test now kills a separate Windows launcher process and proves
the exact WSL payload, port, and launch directory disappear within the bound.

### 4.4 Windows readiness and error handling

After the helper says ready:

- perform an HTTP request from Windows to the exact URL;
- require an expected CATS response/build stamp, not merely an open TCP port;
- reject a backend version mismatch;
- navigate WebView2 only after success;
- show a useful error page containing stage, distro, payload version, and log
  location, but no full environment or secrets;
- provide Retry, Repair, Change distribution, and Open logs actions.

Implemented result: catway now serves bounded JSON at
`/.well-known/cats/health` with product `cats` and the stable build hash. The
Windows launcher requires exact loopback URL shape, disables redirects, checks
HTTP 200, rejects extra/oversized JSON and product/version mismatch, then and
only then returns the URL for WebView navigation. Startup errors carry stage,
distribution, payload version, and rotated log path. Dedicated Retry, Repair,
Change Distribution, and Open Logs buttons share the persistent startup window;
they are disabled logically before navigation to the trusted CATS origin so the
loaded application cannot invoke launcher-only actions.

### 4.5 Clipboard bridge

Implement `clipboard_windows.go` with Win32 APIs and bind the existing names:

- `catsClipWrite(text string) error`
- `catsClipRead() (string, error)`

Open/close the clipboard on the correct thread, allocate/free global memory
correctly, use UTF-16, normalize only what Windows APIs require, and retry
short-lived `ERROR_ACCESS_DENIED` contention with a tight bound. Unit-test pure
encoding/validation functions and integration-test the real clipboard on the
Windows UI runner.

Implemented result: the Win32 `CF_UNICODETEXT` bridge allocates movable global
memory, keeps Open/Close/Lock/Unlock on one locked OS thread, transfers ownership
only after `SetClipboardData`, caps data at 16 MiB, rejects NUL and malformed
surrogates, and retries only bounded access-denied contention. Pure emoji,
combining-character, CRLF/LF, NUL, malformed, and size tests pass; Windows
compilation and binding-name tests pass. A native WebView2 run installed and
called the real read binding, and a real `OpenClipboard` contention run proved
the bounded retry. The bridge is now attached only after top-level/new-window
handlers are installed. An automated write mutation is still opt-in because it
would destroy non-text formats in the user's current system clipboard.

### 4.6 Menu, accelerators, and window lifecycle

- Provide About, Quit, standard edit actions, Bigger Text, Smaller Text,
  Default Text Size, and keyboard-help access.
- Route font actions to the existing `window.catsAdjustFont` hook.
- Use Windows labels and accelerators; do not claim Command-key shortcuts.
- Ensure the title-bar close button, menu Quit, Windows session ending, and a
  normal app shutdown all pass through idempotent cleanup.
- Keep the UI thread locked as required by webview and marshal callbacks to it.
- Write launcher logs before the GUI subsystem removes console visibility.

Implemented result: Windows installs native File/Edit/View menus with About,
Quit, editing, text-size, and keyboard-help actions. WebView2 accelerator events
map `Ctrl+Shift+C/V/X/A`, `Ctrl+Plus/Minus/0`, and F1 without stealing ordinary
terminal Control chords; AltGr-shaped events are rejected, and the page's
palette chord also checks `AltGraph`. Title close, menu Quit,
`WM_QUERYENDSESSION`, normal shutdown, and console interrupt all converge on
idempotent cleanup. The hidden native runner verified menu installation and
actual session-ending message delivery; non-US hardware-layout sign-off remains
part of the broader parity matrix.

### 4.7 Remote mode

Remote mode must remain a thin client:

- start no WSL process;
- show the existing connection form on first run;
- store the URL in Windows app config;
- retain WebView2 cookies across launches;
- use Windows clipboard/menu/notification adapters;
- keep remote server password/TLS behavior unchanged.

Current result: the config extension does not alter remote fields or mode
selection, and existing headless saved-target/first-run tests still prove that
remote mode never invokes the local backend. Remote navigation now accepts only
HTTP(S) URLs with a host and no user information. A native WebView2 test proved
a persistent HTTP cookie survives destruction and recreation of the view, and
the same guarded menu/clipboard adapters are used in local and remote windows.

### 4.8 Navigation and native-binding boundary

- Record the intended CATS origin after local readiness or remote URL
  validation.
- Allow the main WebView to navigate within that origin for login and normal
  application routes.
- Send `_blank`/external HTTP(S) links to the user's default Windows browser.
- Reject or externally open unexpected top-level cross-origin navigation; never
  silently replace CATS with arbitrary content that still has clipboard or
  notification bindings.
- Deny unsafe schemes by default. Add an explicit allowlist only for a feature
  with a tested owner, such as a future CATS deep link.
- Test redirects, `window.open`, target `_blank`, malformed URLs, login flows,
  and remote-mode origins.

Implemented result: malformed/non-loopback backend readiness URLs and HTTP
redirects are rejected before navigation. The pinned wrapper is retained in a
small local fork that exposes its existing WebView2 controller through narrow,
Windows-only navigation, new-window, and accelerator hooks. The launcher records
one normalized HTTP(S) origin, allows same-origin routes, externalizes HTTP(S)
new windows and cross-origin top-level requests, and cancels unsafe schemes.
Pure policy tests cover local/remote origins and malformed URLs; the hidden
native test proves same-origin routing plus `file:`/`javascript:` and
`window.open` denial. Clipboard bindings are installed only after hook setup,
closing the D-012/D-031 release gate.

## Phase 5 — web UI platform behavior

Implementation status (2026-08-09): **implemented and verified on native
Windows/Edge/WebView2**. The native shell publishes a pre-page immutable
platform/capability descriptor, the served UI has one testable platform-policy
module, native menu callbacks reach exported page actions, and the browser
matrix runs without downloaded JavaScript packages or browsers. Manual non-US
hardware-layout and Windows Action Center appearance remain parity-walkthrough
items rather than code blockers.

### 5.1 Platform descriptor

Use `webview.Init` to define a value before the loaded page executes, for
example:

```js
window.catsDesktop = Object.freeze({ platform: "windows", nativeClipboard: true });
```

Do not infer the product mode solely from `navigator.userAgent`; ordinary Edge
and embedded WebView2 need different bridge behavior. The page must tolerate the
descriptor being absent in normal browsers.

Implemented result: `desktopWindow.Init` now maps to `webview.Init`. Windows and
macOS publish frozen `catsDesktop` descriptors before every page, and
`nativeClipboard` is true only when bridge setup succeeds. An ordinary browser
receives no descriptor; OS hints choose labels but never privileged product
mode. Headless and hidden-native tests cover the injected shape, and a real
WebView2 page reported the Windows descriptor before application execution.

### 5.2 Keyboard mapping

Update `cmd/catway/web/index.html`:

- centralize desktop modifier/label detection;
- replace hard-coded `⌘K`, `⌘V`, `⌘+`, `⌘-`, and `⌘0` help strings;
- select Windows chords that do not steal common terminal Control sequences;
- retain current Mac behavior exactly;
- retain `Ctrl+Alt+K` as a portable palette fallback unless testing finds an
  AltGr collision on supported layouts;
- respect IME composition and never encode composition control events into the
  PTY;
- make paste menu/button use `catsClipRead` when present;
- keep copy-mode/OSC52 writes on `catsClipWrite` when present.

Implemented result: `web/platform.js` centralizes descriptor interpretation,
labels, and global keyboard ownership. The page retains Mac Command behavior,
uses `Ctrl+Alt+K`, `Ctrl+Shift+V`, `Ctrl+Plus/Minus/0`, and the Phase 4 native
edit chords on Windows, leaves ordinary terminal Control/Alt and
modifyOtherKeys events structured for the server, rejects AltGr as a palette
modifier, and drops composition/dead/process/229 control events. Help and
toolbar labels are generated from the same policy. Native Paste and Help now
call explicitly exported `window.pasteText`/`window.openHelp`; every selection,
copy-mode, scrollback, and OSC52 write still shares the native-first clipboard
path and bridge failures retain a visible toast.

### 5.3 Notifications

Keep in-page toasts unchanged. If a native bridge is required, expose one
bounded call with title/body/pane identity and route clicks back to a narrowly
defined JavaScript callback that focuses the window and sends `agent.focus`.
Never expose arbitrary activation URIs or shell commands to page data.

Implemented result: no additional notification RPC was needed. WebView2's
standards non-persistent Notification UI is retained, while the native hook
allows only the Notifications permission kind from the already-trusted CATS
origin and denies every other origin/kind. The page caps title/body to 160/512
characters, keeps the in-page toast fallback, catches API failures, and a click
can only focus the window, close that notification, and send `agent.focus` for
a positive bounded pane id. Native WebView2 reported permission `granted`.

### 5.4 Browser regression harness

Add automated cases for:

- printable text, Control sequences, Alt/AltGr, IME, and modifyOtherKeys;
- palette, font, edit, paste, undo/redo, and terminal shortcut ownership;
- select/copy, copy mode, OSC52 write, paste button, and native bridge failure;
- notification permission/fallback, click-to-focus, hidden and visible panes;
- OSC 8 external-link handling and attempted cross-origin navigation;
- reconnect after WSL startup delay or transient localhost failure.

Implemented result: Node's built-in runner executes nine dependency-free policy
cases. `scripts/test-webui-edge.mjs` launches the installed Edge headlessly with
an isolated temporary profile and drives the actual page over CDP. It covers a
failed first WebSocket followed by recovery, printable/IME/AltGr/global shortcut
ownership, native paste and failure toast, the paste context action, drag-select
copy, copy mode, OSC52, visible/hidden/denied/clicked notifications, blocked
unsafe OSC8 and externalized HTTP(S) links. Go contract tests run the Node suite
and keep all shared page paths connected; Windows Go tests additionally invoke
the installed-Edge harness and the existing native cross-origin policy suite.

## Phase 6 — build, install, update, uninstall

**Implementation status: complete (2026-08-09).** The reset-recovery audit,
transaction design, artifact inventory, qualification results, and known
release boundary are recorded in
[phase6_completion.md](phase6_completion.md). The chronological work record is
in [implementation_log.md](implementation_log.md), and D-039 through D-044 in
[decision_log.md](decision_log.md) record the Phase 6 decisions.

### 6.1 Make targets and scripts

Add targets with explicit responsibilities, for example:

```text
make windows-launcher     # Windows Cats.exe on a Windows builder
make wsl-payload          # Linux catway/cathost/catctl/cats-wsl-host
make windows-dist         # assemble matched launcher + payload + scripts
```

Do not make the ordinary Linux `make binaries` unexpectedly install Windows
assets. Reuse the existing build stamp for every executable and add it to the
helper/launcher health output.

### 6.2 Linux payload

- Build libghostty-vt and the tagged Linux binaries natively on the oldest
  supported distro/architecture.
- Include `config.example.yaml`, licenses/NOTICE, version metadata, and payload
  checksums.
- Confirm the glibc floor with `readelf`/runtime tests.
- Do not include Linux GUI libraries; the GUI is Windows-native.

### 6.3 Windows launcher

- Build on Windows with the supported C++ toolchain, CGO, and WebView2 headers.
- Embed version/icon/manifest resources and select Per-Monitor DPI awareness.
- Use the GUI subsystem while retaining file-based diagnostics.
- Produce an Authenticode-signable executable; record whether release
  candidates are signed.

### 6.4 Installer

The first installer may be a signed PowerShell bootstrap plus archive; no
application business logic belongs in it.

Chronological install transaction:

1. Validate Windows 11, WSL command availability/version, WebView2, architecture,
   and PowerShell execution context.
2. Discover WSL2 distributions and select/confirm one and its default user.
3. Verify distro and payload architecture match.
4. Copy the archive through WSL into a new versioned directory under
   `~/.local/lib/cats`; never execute the payload from `/mnt/c`.
5. Verify checksums inside WSL, permissions, sibling binaries, helper
   `--health-json`, and versions.
6. Atomically switch the `current` symlink and update optional
   `~/.local/bin` links.
7. Install `Cats.exe` and assets under a per-user Windows application directory.
8. Write/merge `app.json` without destroying an existing remote target.
9. Create Start-menu and optional desktop shortcuts.
10. Launch a smoke check; on failure restore the previous `current` link and
    leave logs plus repair instructions.

Do not change `/etc/wsl.conf`, enable systemd, install a distribution, or alter
Windows firewall rules without a distinct user-approved installer action.

### 6.5 Upgrade and uninstall

- Stage new Windows and WSL halves before switching either current version.
- On first launch after upgrade, refuse mixed versions and offer repair/rollback.
- Preserve `~/.config/cats`, `~/.local/state/cats`, plugins, worktrees, agent
  configurations, and launcher config by default.
- Uninstall executable payloads and shortcuts only. A separate, explicit
  “remove all CATS data” action must enumerate the exact paths and warn that
  sessions/configuration will be unrecoverable.

## Phase 7 — CI and release

Status: **Complete (2026-08-09).** The requirement/evidence matrix, release
transaction, runner/secret contract, local validation, and honest external
publication boundary are recorded in
[phase7_completion.md](phase7_completion.md). The chronological work is in
[implementation_log.md](implementation_log.md), and the Phase 7 policy choices
are D-045 through D-050 in [decision_log.md](decision_log.md).

### 7.1 Fast jobs

- Existing Ubuntu quick and Linux/macOS ghostty jobs remain required.
- Add Windows format/vet/unit coverage that does not require WSL.
- Cross-compile or natively compile platform-neutral packages where useful, but
  build the CGO WebView2 launcher natively on Windows.
- Add protocol golden tests shared between Windows and Linux jobs.

### 7.2 Integration jobs

- Linux job: run real `cats-wsl-host` with test daemons and then with real
  `catway`/`cathost`; execute `catctl probe`.
- Windows job: build WebView2 launcher and test app config, distro parsing,
  command construction, port retry, clipboard encoding, and mocked protocol.
- Windows 11/WSL2 GUI job: install a clean payload, launch, drive WebView2,
  create a pane, run a command, copy/paste, close, relaunch, and confirm restore.
  Use a dedicated runner if the hosted service cannot guarantee WSL2/GUI.

### 7.3 Release transaction

- Build Windows and Linux halves from the same tag and full git history.
- Name artifacts by Windows arch, WSL arch, version, and supported distro floor.
- Generate SHA-256 checksums and a machine-readable manifest that maps launcher
  to payload.
- Do not publish a Windows release when either half or the end-to-end smoke test
  failed.
- Attach signing/provenance information and generated release notes.

## Phase 8 — qualification checklist

Implementation status (2026-08-09): **qualification implementation complete;
release sign-off remains gated**. The assembled-candidate harness now covers
the broad functional path, loopback security, archive safety, owner loss,
performance budgets, ext4/drvfs comparison, and persistence. Repository-wide
Ubuntu and focused race tests pass. The current desktop session cannot provide
a native clipboard/WebView pass, and distro-wide terminate/shutdown was not run
against an active shared Ubuntu instance; both remain mandatory on the labeled
dedicated release runner. Reboot, sleep/resume, NAT, VPN/firewall, real external
agent credentials, and the dated visual walkthrough remain manual matrix work.
See [phase8_qualification.md](phase8_qualification.md) and
[windows_parity_checklist.md](windows_parity_checklist.md) for exact evidence
and non-passing items. Decisions are D-051 through D-056.

### Functional

- local and remote mode;
- workspace/tab/pane CRUD, resize, zoom, reorder, rename, lock;
- terminal text, colors, Unicode, mouse, hyperlinks, scrollback, copy mode;
- Win32 clipboard read/write and OSC52;
- every documented Windows keyboard/menu action;
- all supported agent detection/integrations/resume flows in WSL;
- plugins, plugin update/link, worktrees, path picker, completions, `catctl`;
- ACP chat and credentials inside WSL;
- themes/settings/live reload;
- browser and native notifications;
- persistence and cold/live restore.

### Resilience

- close button, Quit, Task Manager kill, helper/daemon crash;
- WSL cold start, terminate, shutdown, Windows reboot, sleep/resume;
- port conflicts, localhost forwarding failure, VPN/firewall interference;
- missing/renamed/reset distribution, changed default user, corrupt config;
- missing/mismatched/partial payload, failed upgrade and rollback;
- multiple launcher instances and simultaneous ordinary browser clients.

### Security

- verify only `127.0.0.1` is listening in local mode;
- verify no password/token is written to launcher config or logs;
- verify helper protocol rejects oversized/malformed input and has no arbitrary
  command facility;
- verify socket/runtime directories are user-only;
- verify WebView bindings are unavailable to untrusted navigation or are gated
  to the intended CATS origins;
- verify OSC 8 and other external links cannot navigate the privileged WebView;
- inspect installer quoting, archive traversal defenses, symlink handling, and
  exact uninstall targets;
- verify WebSocket origin enforcement and remote TLS/password behavior.

### Performance

- compare startup cold/warm, keystroke-to-frame latency, scroll throughput,
  memory, disk, many-pane behavior, and restore time with macOS/Linux baselines;
- compare WSL ext4 and `/mnt/c` workspaces and document the expected penalty;
- ensure the host resource display is labeled/interpreted as WSL VM memory and
  disk, not total Windows host resources.

## Phase 9 — documentation and release completion

Update the main documentation only after behavior is stable:

- architecture overview and topology chooser;
- getting started for Windows 11/WSL2;
- build and packaging reference;
- configuration path and ownership tables;
- integration/plugin/worktree instructions emphasizing “install/run in WSL”;
- troubleshooting for WSL distribution selection, localhost, WebView2,
  clipboard, PATH, payload mismatch, `/mnt/c`, and logs;
- release support matrix and limitations.

Add the new pages to `mkdocs.yml`, run link/build checks, and have a clean
Windows user follow the docs without developer assistance before declaring the
version complete.
