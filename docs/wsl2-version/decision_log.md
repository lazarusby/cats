# WSL2/Windows decision log

This log records architectural/product decisions for the Windows 11/WSL2
version. “Proposed” items are recommendations that still require owner approval;
“Accepted” items came from the user or are direct consequences of an accepted
choice. Superseded decisions remain in the log with a link to their replacement.

## D-001 — Windows-native launcher with a WSL2 backend

- Date: 2026-08-05
- Status: Accepted
- Decision: Build a native Windows Go launcher using Edge WebView2. Run
  `catway`, `cathost`, `catctl`, shells, agents, plugins, worktrees, and state
  inside WSL2.
- Context: The current launcher is Darwin-only and uses Cocoa/WKWebView. The
  core application already supports Linux, including Linux PTYs, `/proc` agent
  detection, and Linux libghostty-vt builds.
- Consequences: The product crosses a Windows/WSL process and localhost
  boundary, and releases contain a Windows PE plus Linux ELF payload. It avoids
  GTK/WebKitGTK/WSLg runtime dependencies and gives the closest Windows desktop
  behavior.
- Alternatives rejected: WSLg GTK/WebKitGTK launcher; browser/PWA-only client.

## D-002 — Keep Go and existing application technologies

- Date: 2026-08-05
- Status: Accepted
- Decision: Continue using Go for the launcher, helper, and application code;
  retain the existing HTML/CSS/JavaScript UI and vendored Zig/libghostty-vt
  build. Use PowerShell only as Windows installation glue.
- Consequences: No Electron, .NET desktop rewrite, Rust backend, or new browser
  UI framework is introduced. Windows-only Go bindings may use
  `golang.org/x/sys/windows` where the standard library is insufficient.

## D-003 — WSL owns terminal workloads

- Date: 2026-08-05
- Status: Accepted
- Decision: Feature parity means Linux shells and Linux-installed agents inside
  WSL. Native Windows shells and Windows-installed agents are out of scope.
- Rationale: The existing `cathost` is built around Unix PTYs and Linux/Darwin
  process inspection. A Windows-native terminal backend would be a separate
  ConPTY and process-detection project, not a WSL2 port.
- Consequences: Agent credentials, hooks, plugins, `catctl`, projects, and
  worktrees must exist inside the selected distribution. Windows files remain
  accessible through `/mnt/*` but are not the recommended project location.

## D-004 — Preserve existing data-plane protocols

- Date: 2026-08-05
- Status: Accepted
- Decision: WebView2 talks to `catway` over the existing HTTP/WebSocket browser
  protocol through Windows/WSL localhost forwarding. `catway` and `cathost`
  retain their existing private Unix-socket orchestration seam. Control and hook
  sockets remain inside WSL.
- Rationale: No existing protocol requires Windows-native transport when the
  terminal workloads remain in WSL.
- Consequences: Do not add a WebSocket-to-Unix proxy, expose control sockets via
  TCP, or move the orchestration seam across the host boundary.

## D-005 — Add a narrow Go WSL host helper

- Date: 2026-08-05
- Status: Accepted
- Decision: Add `cats-wsl-host`, a Linux Go process started by the Windows
  launcher through `wsl.exe`. It hydrates the Linux environment, allocates
  private sockets, supervises `catway`/`cathost`, reports readiness/version over
  a bounded JSON-lines channel, and performs ordered cleanup.
- Rationale: Starting two independent `wsl.exe` processes from Windows would
  duplicate macOS supervision poorly and make cleanup/versioning fragile.
- Consequences: The helper is lifecycle control only; it does not carry terminal
  traffic or arbitrary command requests.

## D-006 — Local mode is loopback-only and unauthenticated

- Date: 2026-08-05
- Status: Accepted
- Decision: Match standalone Mac behavior: local `catway` binds explicitly to
  `127.0.0.1` and uses `--auth none`. Remote mode retains password/TLS behavior.
- Rationale: The Windows user and WSL user are parts of one local desktop trust
  boundary, and a login prompt would add no useful protection against processes
  owned by that same user.
- Consequences: Binding `:<port>`/`0.0.0.0` is a release-blocking security bug.
  LAN publishing remains a separately configured server topology.

## D-007 — Separate Windows launcher state from WSL application state

- Date: 2026-08-05
- Status: Accepted
- Decision: Store launcher selection/preferences under
  `%LOCALAPPDATA%\Cats`. Keep server config, persistence, plugins, themes,
  worktrees, manifests, and agent data in their existing XDG locations inside
  WSL.
- Consequences: Resetting/reinstalling a distribution may leave a stale Windows
  target that the launcher must diagnose and repair. Neither installer nor
  uninstall may silently delete the other side's user data.

## D-008 — Preserve native clipboard semantics with a Windows bridge

- Date: 2026-08-05
- Status: Accepted
- Decision: Implement the existing `catsClipRead`/`catsClipWrite` bindings with
  Unicode Win32 clipboard APIs.
- Rationale: Browser clipboard permissions do not reliably cover WebSocket-led
  OSC52 writes or non-gesture reads; feature parity requires the native bridge
  already used on macOS.
- Consequences: Clipboard content must never be logged. WebView navigation and
  binding exposure require security review.

## D-009 — The Windows launcher owns the foreground WSL lifetime

- Date: 2026-08-05
- Status: Accepted
- Decision: In standalone local mode, the Windows launcher keeps one foreground
  `wsl.exe` helper connection alive and stops the backend when the window quits.
  Systemd is not required.
- Rationale: This matches current Mac behavior, makes pipe EOF useful for crash
  cleanup, and avoids relying on systemd services to keep a WSL instance alive.
- Consequences: A background/persistent-across-window-close mode may be added
  later as an explicit product decision, not as an accidental service side
  effect.

## D-010 — Install Linux payload into the WSL filesystem

- Date: 2026-08-05
- Status: Accepted
- Decision: Install versioned payloads under `~/.local/lib/cats` inside the
  selected distribution and atomically update a `current` link. Do not execute
  the backend from `/mnt/c`.
- Rationale: This preserves Linux permissions/symlinks and avoids cross-filesystem
  performance and execution edge cases.
- Consequences: The installer must copy and verify data across the WSL boundary
  and manage Windows and WSL halves transactionally.

## D-011 — Native Windows shells are a separate future architecture

- Date: 2026-08-05
- Status: Accepted
- Decision: Requests for PowerShell/cmd/Windows executables as pane processes
  will be evaluated as a future Windows backend, not slipped into this WSL
  implementation.
- Required future work if reconsidered: ConPTY host, Windows process/agent
  detection, Windows path and environment model, named-pipe or authenticated
  transport, `.ps1` integration support, persistence migration, and a new test
  matrix.

## D-012 — Native bindings belong only to the trusted CATS view

- Date: 2026-08-05
- Status: Accepted
- Decision: The privileged WebView that exposes clipboard and possible native
  notification bindings may navigate only within its configured CATS origin.
  External/OSC 8 links open in the user's default Windows browser.
- Rationale: The current page uses `window.open` for terminal hyperlinks. A
  platform port must not allow arbitrary link targets to replace or inherit the
  privileged launcher page.
- Consequences: Navigation/new-window behavior is a required WebView2 spike,
  implementation adapter, and security test area.

## D-013 — Initial Windows/WSL development and qualification baseline

- Date: 2026-08-09
- Status: Accepted
- Decision: For the initial Windows/WSL implementation, use Windows 11 on
  amd64 with Store WSL2 and Ubuntu 24.04.3 LTS on amd64 as the only supported
  development baseline. The measured reference host is Windows 11 Pro build
  26200, WSL 2.7.11.0, Linux kernel 6.18.33.2-2, Ubuntu 24.04.3 LTS, and
  WebView2 Runtime 151.0.4129.72 on an 8-core/16-thread AMD Ryzen AI 7 PRO 350.
- Context: The owner selected the current AMD Ryzen 7-class Windows 11/WSL2
  Ubuntu machine as the baseline for now. Exact measured versions make results
  reproducible without implying that an untested older Windows/WSL version is
  supported.
- Consequences: Initial payloads pair `windows/amd64` with `linux/amd64` and
  target Ubuntu 24.04.3 LTS. ARM64 and other distributions remain future matrix
  additions. Windows build 26200 and WSL 2.7.11.0 are validated baselines, not
  permanent product minimums; lowering either floor requires qualification.
- Evidence: `phase0_baseline.md` and the 2026-08-09 implementation-log entry.

## D-014 — Mirrored networking is the active baseline; NAT remains a gate

- Date: 2026-08-09
- Status: Accepted
- Decision: Develop first against the reference host's mirrored WSL networking.
  A release must also pass the same localhost/lifecycle probe in NAT mode before
  CATS claims coverage for both standard WSL networking configurations.
- Context: The active `%USERPROFILE%\.wslconfig` selects
  `networkingMode=mirrored`. Changing it requires stopping WSL and disrupting
  the shared development environment, so NAT was not changed during this pass.
- Consequences: The launcher must retry a bounded number of complete startups
  on a new port. Under mirrored networking, the spike observed that a listener
  in either Windows or WSL prevented the other side from binding the same
  loopback port.
- Evidence: `spikes/wsl2/probe-wsl.ps1` passed Windows localhost, requested
  stop, stdin EOF, and both port-conflict directions on 2026-08-09.

## D-015 — Desktop lifecycle protocol version 1 is bounded and directional

- Date: 2026-08-09
- Status: Accepted
- Decision: Implement `internal/desktopproto` with separate helper and launcher
  record types, mandatory newline framing, a 16 KiB JSON-object limit, strict
  version/type and per-record validation, and forward-compatible ignoring of
  additive unknown fields. A ready endpoint must be
  `127.0.0.1:<valid-port>`.
- Context: The helper's stdout is an untrusted control boundary. A strict,
  narrow schema prevents it from evolving accidentally into a terminal-data or
  arbitrary-command channel while retaining additive compatibility.
- Consequences: EOF with no buffered bytes is normal owner disappearance; EOF
  after partial JSON is truncation. Unknown versions/types are fatal. Human
  logs remain on stderr. Startup deadline policy belongs to the launcher that
  owns the pipe.
- Evidence: `internal/desktopproto` unit tests cover round trips, malformed and
  truncated JSON, oversize input/output, unknown versions/types, additive
  fields, partial reads, validation, EOF, and short writes.

## D-016 — Retain the pinned webview dependency until its native spike runs

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep `github.com/webview/webview_go` pinned at
  `v0.0.0-20240831120633-6173450d4dd6`. Do not update it merely to begin the
  port; update only if the native Windows spike demonstrates a required
  WebView2 behavior is missing or broken.
- Context: The reference Windows host currently has no native Windows Go or
  UCRT64 MinGW toolchain, so the WebView2 spike source exists but has not been
  compiled or run. The installed Evergreen WebView2 Runtime is 151.0.4129.72.
- Consequences: WebView2 `Bind`, `Dispatch`, `Eval`, navigation, resize,
  cookies, WebSocket, notification, new-window, and teardown claims remain
  pending. The decision avoids an evidence-free dependency change.

## D-017 — Start with a per-user signed archive installer

- Date: 2026-08-09
- Status: Accepted
- Decision: The first installer format will be a per-user archive with a
  Windows PowerShell 5.1-compatible installation script. Release artifacts
  should Authenticode-sign `Cats.exe` and the PowerShell installer; MSIX is
  deferred until the cross-Windows/WSL update transaction is proven.
- Context: Installing the Linux payload inside a selected WSL distribution is a
  transaction MSIX cannot own by itself. The reference host provides inbox
  Windows PowerShell 5.1.
- Consequences: Phase 6 must implement atomic Linux payload staging/current-link
  replacement, rollback, Start-menu integration, and independent preservation
  of Windows and WSL user data. Signing infrastructure is a release prerequisite.

## D-018 — Keep launcher platform seams local and narrow

- Date: 2026-08-09
- Status: Accepted
- Decision: `cmd/catapp` common flow depends on two local interfaces only:
  `localBackend` for URL/stop lifecycle and `desktopWindow` for the webview
  operations common code actually uses. Platform functions own backend startup,
  app-data paths, menus, native bridges, environment hydration, and signals.
- Context: Importing `webview_go`, Cocoa, process syscalls, or future Windows
  APIs directly into common mode flow would make the refactor difficult to test
  and would blur the Windows/WSL ownership boundary.
- Alternatives: A general dependency-injection framework; exposing the full
  upstream webview interface; keeping all behavior in `main.go`.
- Consequences: The interfaces remain package-private and small. Direct adapter
  methods preserve native webview behavior. Package variables are used only for
  small window/backend fakes in tests.
- Evidence: `cmd/catapp/backend.go`, `window.go`, `window_native.go`, and the
  headless common tests.

## D-019 — Phase 2 Windows local mode fails closed

- Date: 2026-08-09
- Status: Accepted
- Decision: Before Phase 4 implements safe `wsl.exe` discovery and helper
  supervision, a Windows build may use common/remote launcher flow but local
  mode returns an explicit not-implemented error. Phase 2 Windows menu and
  privileged bridge adapters are no-ops.
- Context: Starting an incomplete backend or exposing placeholder native
  bindings would make a compile success look like platform parity and could
  violate the trusted-view boundary. Remote mode does not require WSL.
- Consequences: Windows local mode cannot accidentally start daemons, invoke a
  shell, or expose clipboard access. Phase 4 must replace the stub and add its
  security/navigation tests before local Windows builds are usable.
- Evidence: `supervise_windows.go`, `platform_windows.go`, and
  `supervise_windows_test.go`.

## D-020 — Headless testing is not a Linux desktop product

- Date: 2026-08-09
- Status: Accepted
- Decision: Add the explicit `catapp_headless` build tag solely to execute
  platform-neutral launcher tests on Linux/WSL. Normal Linux builds still omit
  `cmd/catapp`; the tag's platform adapter panics if asked to create a real
  window and returns errors for platform services.
- Context: `webview_go` is cgo-only and this WSL host has neither WebKitGTK
  development libraries nor the Darwin/Windows native toolchains. Config,
  lifecycle, page, mode, and cleanup behavior should still receive executable
  coverage in ordinary Linux CI.
- Consequences: `make check` now runs `make test-catapp-common`. Native Cocoa,
  WebView2, menu, clipboard, and process-supervision tests remain mandatory on
  their real platform builders; headless success cannot satisfy those gates.

## D-021 — Package the helper only in the four-binary WSL payload

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep the ordinary `BINS` list as `catway`, `cathost`, and `catctl`.
  Add `cats-wsl-host` through a separate `WSL_BINS` list and
  `make wsl-payload`; require all three public binaries to be regular executable
  siblings of the helper before health or startup succeeds.
- Context: The helper is a private Windows/WSL lifecycle component, not a new
  public daemon for ordinary Linux installations. `catctl` is not supervised,
  but belongs to the matched payload users operate inside WSL.
- Consequences: Existing `make binaries`, `make local`, and Linux distribution
  behavior do not change. The four WSL executables receive the same build stamp
  and can be installed atomically by Phase 6.
- Evidence: `Makefile`, `cmd/cats-wsl-host.verifyPayload`, and the successful
  `make wsl-payload`/health run.

## D-022 — Make the non-WSL override explicit and development-only

- Date: 2026-08-09
- Status: Accepted
- Decision: Production helper startup checks both kernel release proc files for
  a Microsoft/WSL marker. Ordinary Linux fails with a diagnostic. Tests and
  deliberate development runs may opt in with `--allow-non-wsl`; there is no
  ambient environment-variable bypass.
- Context: The payload is Linux code but its owner/localhost/interop assumptions
  are WSL-specific. Tests still need to run on ordinary Linux CI.
- Consequences: Mistaken Linux deployment fails closed, while the override is
  visible in process arguments and cannot be activated accidentally by an
  inherited environment value.
- Evidence: `requireWSL` and its ordinary-Linux, WSL-marker, and override tests.

## D-023 — Share only shell-output parsing, not launch policy

- Date: 2026-08-09
- Status: Accepted
- Decision: Move marker extraction and stable PATH merging to
  `internal/shellenv`. Keep shell selection, launch detection, timeout,
  identity normalization, and error policy in the Darwin and WSL callers.
- Context: Both launchers need to tolerate noisy interactive shell startup and
  preserve inherited-only PATH entries, but Finder and `wsl.exe --exec` have
  different policies and defaults.
- Consequences: Captured shell output is never evaluated as syntax. WSL puts
  the login/interactive Linux PATH first while retaining inherited interop
  entries, explicitly resets the four selected-user identity variables, and
  falls back without blocking startup when its five-second probe fails.
- Evidence: `internal/shellenv`, `environment.go`, and the migrated Darwin tests.

## D-024 — Use exact non-recursive runtime cleanup

- Date: 2026-08-09
- Status: Accepted
- Decision: Use a private owned `XDG_RUNTIME_DIR/cats/<launch-id>` only when
  every precondition and socket-length check passes; otherwise allocate a
  mode-0700 `cats-wsl-<uid>-<random>` temp directory. Cleanup may unlink only
  the resolved `th.sock`, `ctl.sock`, and `hook.sock` children and then remove
  that exact directory; it never calls recursive removal.
- Context: Launch IDs and runtime environment values cross process boundaries,
  and an unsafe cleanup target would turn a lifecycle convenience into a data
  deletion risk. Unix socket path lengths are also sharply bounded.
- Consequences: Collisions, insecure XDG permissions/ownership, and overly long
  paths safely fall back. Unexpected extra files prevent directory removal and
  surface a cleanup error instead of being deleted.
- Evidence: `runtime_paths.go`, fallback/ownership/length tests, cleanup safety
  tests, and the real-socket cleanup test.

## D-025 — Stop once in reverse order and do not auto-restart

- Date: 2026-08-09
- Status: Accepted
- Decision: Each daemon runs in its own process group with one wait/reap owner.
  Any stop, stdin EOF, supported signal, protocol error, or unexpected child
  exit enters an idempotent teardown: SIGTERM `catway`, wait five seconds and
  escalate its group, then SIGTERM `cathost`, wait three seconds and escalate.
  Unexpected exits are reported to the launcher; version 1 never auto-restarts.
- Context: `catway` needs a bounded save/final-capture window and `cathost` owns
  the PTYs. Concurrent waits, duplicate stop paths, or restart policy hidden in
  the first helper would make state loss and orphaning more likely.
- Consequences: App-close parity is deterministic and unresponsive descendants
  cannot block forever. A future background or restart policy requires a new
  product decision and protocol/test work.
- Evidence: `supervise.go`, fake-process lifecycle tests, forced-kill test, and
  the successful real-payload requested-stop smoke.

## D-026 — Match artifacts by stamped hash, not dirty metadata

- Date: 2026-08-09
- Status: Accepted
- Decision: Launcher/helper/backend compatibility uses the shared stamped Git
  hash. `vcs.modified` remains visible diagnostic metadata but is not appended
  to the protocol or HTTP compatibility version.
- Context: The real Phase 4 test found the same dirty source produced a helper
  reporting `d91aa1d-dirty` and a cross-compiled Windows test reporting
  `d91aa1d`; Go build modes do not record dirty metadata uniformly.
- Consequences: Matched artifacts no longer fail due to toolchain metadata,
  while different commits still fail closed. Release provenance may separately
  reject dirty builds.
- Evidence: `buildinfo.Version`, helper health, catway health, and the passing
  rebuilt Windows-to-WSL integration test.

## D-027 — Keep WSL command construction as validated argument vectors

- Date: 2026-08-09
- Status: Accepted
- Decision: Validate distribution/user/control characters and absolute Linux
  paths, reject root and `/mnt` payloads, then pass each `wsl.exe` option/value
  as a separate `exec.CommandContext` argument. Accept spaces and Unicode as
  data where WSL supports them; do not solve quoting with a shell.
- Context: Distribution names are untrusted Windows state and Linux paths can
  contain spaces. Strict ASCII-only policy would be unnecessarily incompatible,
  while string assembly would create command-injection risk.
- Consequences: Normal startup never uses `cmd.exe`, PowerShell, or `sh -c`.
  Health inside the selected distro/user remains the authoritative existence
  check.
- Evidence: `internal/wslclient` argument/path/UTF-16 tests and the real launch.

## D-028 — Retry only classified bind and forwarding failures

- Date: 2026-08-09
- Status: Accepted
- Decision: Permit at most three complete startups, each with a new random
  launch ID and Windows-reserved port, only after helper stage `bind` or a
  Windows HTTP network/forwarding failure. Do not retry target, payload,
  protocol, product, version, or daemon failures.
- Context: Mirrored/NAT forwarding and cross-namespace port races may be
  transient; payload or compatibility failures need repair, not churn.
- Consequences: Phase 3 observes the narrow early
  `address already in use` signal and reports stage `bind`. Every failed attempt
  is stopped/reaped before the next begins.
- Evidence: retry-classification tests, protocol tests, and bounded supervisor.

## D-029 — Require build-stamped HTTP identity before navigation

- Date: 2026-08-09
- Status: Accepted
- Decision: Add `/.well-known/cats/health` returning bounded JSON product and
  version. After protocol ready, Windows must GET the exact loopback URL with
  redirects disabled and require HTTP 200, product `cats`, and matching stamped
  hash before WebView navigation.
- Context: An open port or successful TCP dial cannot distinguish catway from a
  stale or unrelated local service.
- Consequences: The endpoint is additive and local startup remains auth-none.
  Unsafe URL shapes, redirects, extra/oversized output, and mismatches fail
  closed.
- Evidence: backend-health/wslclient tests and the real Windows HTTP pass.

## D-030 — Use atomic config replacement and bounded redacted logs

- Date: 2026-08-09
- Status: Accepted
- Decision: Stage/sync launcher config beside its destination and replace it
  with Windows `MoveFileExW` replace/write-through semantics. Inherit the
  per-user LocalAppData DACL. Rotate launcher logs at 1 MiB and redact common
  credential assignments; never log health stdout, environments, or clipboard.
- Context: Windows and WSL state can change independently, and GUI-subsystem
  failures need durable diagnostics without turning the log/config into a
  secret store.
- Consequences: Interrupted saves retain either old or new complete JSON. One
  previous log is retained. Dedicated ACL hardening beyond inherited user
  profile policy can be added if native security review finds it necessary.
- Evidence: Windows-executed config tests, redaction tests, and launcher log.

## D-031 — Gate privileged bindings on navigation enforcement

- Date: 2026-08-09
- Status: Accepted
- Decision: Implement and compile-test the Win32 Unicode clipboard bridge, but
  keep `bindPlatformBridges` empty until the native Windows view can reject
  unexpected top-level/cross-origin navigation and externalize new windows.
- Context: The pinned `webview_go` API exposes navigation but no navigation or
  new-window decision callbacks. Binding clipboard now would violate D-012 if
  an external page replaced the trusted CATS view.
- Consequences: Native clipboard parity remains a release gate even though its
  memory/thread/encoding implementation exists. Do not weaken the origin
  boundary to claim feature completion.
- Evidence: pure UTF-16 tests, Windows compile/binding tests, and the explicit
  product-flow no-binding test.
- Implementation update (2026-08-09): D-033 supplies the required native
  navigation/new-window enforcement. The condition is now satisfied and the
  bridge is enabled only after the handlers attach successfully.

## D-032 — Defer Job Objects until proxy evidence requires them

- Date: 2026-08-09
- Status: Accepted
- Decision: Do not add a Windows Job Object in the first launcher core. Use
  protocol stop and stdin EOF as authoritative Linux cleanup, with a bounded
  `wsl.exe` proxy kill only after graceful shutdown fails.
- Context: Killing the Windows proxy is not equivalent to gracefully signalling
  Linux descendants, and Phase 1/4 real tests already prove stop and EOF.
- Consequences: Protocol stop/EOF, Windows session ending, and the native
  Task-Manager-equivalent owner-loss case pass without a Job Object.
  `wsl --terminate` and `wsl --shutdown` remain explicitly disruptive opt-in
  qualification cases. Add a Job Object only if those cases demonstrate a
  concrete cleanup improvement.
- Evidence: Phase 1 probes, real Phase 4 ordered-stop integration, native
  `WM_QUERYENDSESSION`, and the passing exact-process/exact-runtime owner-loss
  test after D-034.

## D-033 — Extend the pinned WebView wrapper locally for security hooks

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep the pinned upstream `webview_go` revision in a small local
  fork and expose only Windows navigation-starting, new-window, and accelerator
  callbacks from the `ICoreWebView2Controller` that the bundled C++ library
  already owns. Install privileged bindings only after all hooks attach.
- Context: The official module's latest revision remains the already-pinned
  2024 commit. Its C API exposes the native controller but its Go API exposes
  neither the controller nor the required callbacks. Updating cannot close the
  D-012 boundary, and unsafe reflection into its private Go struct is brittle.
- Alternatives: Keep clipboard disabled; replace the whole WebView stack;
  access private state through reflection/linkname; maintain a narrow fork.
- Consequences: `go.mod` uses a local `replace` and the repository carries about
  1.02 MiB of MIT-licensed upstream source. The extension uses `runtime/cgo.Handle`
  rather than Go pointers in C++, unregisters before WebView destruction, and
  leaves Darwin and the public wrapper behavior unchanged.
- Evidence: Native UCRT64 compile, hidden real-WebView policy tests, persistent
  cookie test, and the real WSL-backed CATS WebView run.

## D-034 — Treat owner-pipe SIGPIPE as a cleanup error, not process death

- Date: 2026-08-09
- Status: Accepted
- Decision: `cats-wsl-host` ignores `SIGPIPE`. When the Windows owner disappears
  and closes protocol stdout, writes return `EPIPE`; the helper reports what it
  can and then completes all ordered child and exact runtime-path defers.
- Context: The native owner-kill test proved both daemons exited but the empty
  launch directory remained. The helper had already stopped its children, then
  the default Linux `SIGPIPE` action killed it while writing `stopped` to the
  dead Windows pipe, before the outer runtime cleanup defer.
- Alternatives: Accept empty `/run` debris; recursively sweep old directories;
  add a detached relay process; handle the broken pipe normally.
- Consequences: Normal protocol behavior is unchanged. Owner disappearance now
  produces a bounded write error rather than bypassing Go cleanup. No broad or
  recursive stale-directory cleanup is introduced.
- Evidence: Linux helper/unit tests and the final native owner-loss rerun passing
  in 2.24 seconds with no exact payload process, reachable port, or launch path.

## D-035 — Use Windows-native menus without stealing terminal Control chords

- Date: 2026-08-09
- Status: Accepted
- Decision: Provide native File/Edit/View menus. Use
  `Ctrl+Shift+C/V/X/A` for edit actions, `Ctrl+Plus/Minus/0` for font size, and
  F1 for keyboard help. Leave plain terminal Control chords untouched and
  reject accelerator events identified as AltGr. Keep `Ctrl+Alt+K` page-owned,
  but require that the DOM event is not `AltGraph`.
- Context: A terminal desktop app cannot safely map ordinary Windows edit
  shortcuts over PTY input. WebView2 exposes accelerator events before page
  dispatch, while Windows reports AltGr as a Control+right-Alt shape.
- Alternatives: Reserve `Ctrl+C/V` globally; provide menu actions without real
  accelerators; keep all shortcuts in page JavaScript.
- Consequences: Menus remain usable with mouse/access keys and have explicit
  Windows chords. Non-US hardware-layout manual qualification remains in the
  parity matrix, while policy tests cover the AltGr/control separation.
- Evidence: Native menu presence, accelerator table tests, AltGraph web guard,
  and hidden WebView2/session-ending integration.

## D-036 — Inject an immutable, capability-truthful desktop descriptor

- Date: 2026-08-09
- Status: Accepted
- Decision: Every native catapp window uses `webview.Init` to publish a frozen
  `window.catsDesktop` before page execution. The descriptor names `windows` or
  `macos` and reports `nativeClipboard: true` only when both bridge bindings
  attached successfully. A normal browser receives no descriptor.
- Context: User-agent/platform hints can select ergonomic labels in a browser,
  but cannot distinguish ordinary Edge from privileged WebView2. The page must
  never infer that native clipboard access exists from an OS string.
- Alternatives: Continue inferring from `navigator`; inject a build-time value
  into the served page; publish a mutable global after navigation.
- Consequences: Product mode and native capability are explicit and available
  before the application script. Browser OS hints affect labels only. A bridge
  setup error produces a truthful false capability and absent bridge functions.
- Evidence: headless descriptor contract test, native compilation, installed-
  Edge absent/present descriptor policy tests, and hidden WebView2 reporting
  `{platform:"windows", nativeClipboard:true}`.

## D-037 — Keep Phase 5 browser policy dependency-free and directly testable

- Date: 2026-08-09
- Status: Accepted
- Decision: Centralize platform labels, global shortcut ownership, IME/AltGr
  classification, clipboard selection, notification decisions, external-URL
  validation, and reconnect timing in `web/platform.js`. Inline that source into
  the rendered page and test it with Node's built-in test runner. Add an
  installed-Edge CDP harness for the actual page/DOM/WebSocket integration.
- Context: `index.html` is intentionally dependency-free and the repository has
  no JavaScript package manager or downloaded-browser toolchain. Source-shape
  assertions alone cannot prove that event listeners and async browser paths
  work together.
- Alternatives: Add Playwright/npm dependencies; keep all behavior in one
  untestable IIFE; test only pure Go render output; require manual browser runs.
- Consequences: The shipped page gains no network/runtime dependency. Policy
  tests run anywhere Node is present; the Windows runner uses the already-
  installed Edge with an isolated temporary profile. Cross-origin top-level
  enforcement remains additionally covered by native WebView2 tests.
- Evidence: nine Node policy cases and the real Edge regression covering
  reconnect, keyboard/IME/AltGr, clipboard success/failure, selection, OSC52,
  notification focus, and allowed/blocked OSC 8 links.

## D-038 — Use WebView2's standards notification UI with a narrow permission gate

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep the page's standards-based non-persistent `Notification` path
  instead of adding a native notification RPC. Extend the local WebView2 hook
  layer to allow only `Notifications` permission from the currently trusted
  CATS HTTP(S) origin and explicitly deny other origins and permission kinds.
- Context: Current WebView2 supports non-persistent Web Notifications and shows
  its default UI for unhandled notifications, but host applications must decide
  `PermissionRequested`; WebView2 presents no browser permission prompt. The
  existing page already has the desired bounded click behavior.
- Alternatives: Add a native toast/activation bridge; leave permission requests
  unhandled; broadly allow all WebView permissions; use in-page toasts only.
- Consequences: Notification page data cannot supply activation URIs or shell
  commands. Titles/bodies are capped at 160/512 characters; clicks only focus
  the window and send `agent.focus` for the bounded pane id. Browser denial or
  construction failure retains the in-page toast fallback.
- Evidence: Microsoft WebView2 API/permission documentation, native WebView2
  permission=`granted` integration, permission-origin unit tests, and Edge
  permission/fallback/click tests. Manual Action Center appearance remains in
  the parity walkthrough rather than requiring a second privileged bridge.

## D-039 — Couple Windows and WSL artifacts by one compatibility identity

- Date: 2026-08-09
- Status: Accepted
- Decision: Build the Linux payload natively and the Windows launcher natively,
  stamp both with the same Git hash, and let `windows-dist` accept only the
  hash-named Linux/amd64 payload that matches its source revision. Put that
  identity, architecture, prerequisites, checksums, and signing status in
  machine-readable manifests.
- Context: The two halves require different native toolchains, but mixed
  launcher/helper builds are unsafe and must never be assembled accidentally.
- Alternatives: Cross-compile the cgo payload on Windows; use independent
  semantic versions; let the installer discover compatibility at runtime only.
- Consequences: Ordinary Linux builds remain unchanged. Distribution assembly
  fails early on a mismatched payload filename, and health checks enforce the
  same identity again after installation. Tag/release provenance remains Phase
  7 work.
- Evidence: `package-wsl-payload.sh`, `Build-WindowsDist.ps1`, both
  `release.json` files, and the matched `b5ba4df` integration package.

## D-040 — Commit installation only after a real cross-boundary smoke

- Date: 2026-08-09
- Status: Accepted
- Decision: Treat installation/upgrade as a two-platform transaction. Stage
  and verify both version directories, atomically switch WSL `current`, update
  launcher config/shortcuts, then run the installed `Cats.exe --install-smoke`.
  Any pre-commit failure restores prior Windows/WSL directories, link, config
  bytes, shortcut definitions, and newly created command links.
- Context: File checks and helper health cannot prove that Windows localhost
  forwarding, matching launcher identity, real daemon startup, and ordered
  shutdown all work together.
- Alternatives: Commit after checksum verification; launch the interactive UI
  and ask the user to diagnose failure; update the two halves independently.
- Consequences: Install takes one bounded backend startup, but a successful
  return means the exact installed pair worked. Installer and launcher logs
  retain repair evidence. Previous versions are retained for rollback rather
  than eagerly collected.
- Evidence: final isolated clean install/uninstall and existing-install forced
  smoke rollback, including a successful smoke after restoration.

## D-041 — Resolve WSL `current` once per launcher startup

- Date: 2026-08-09
- Status: Accepted
- Decision: Store the stable `current` symlink in `app.json`, but have the
  Windows launcher resolve it with direct `readlink -f` exactly once before
  health and startup. Use that versioned path for both operations.
- Context: `/proc/self/exe` reports the resolved helper directory. Passing the
  symlink path made health identity disagree, and resolving separately could
  cross an installer symlink switch between health and launch.
- Alternatives: Teach health to report the unresolved invocation path; resolve
  for every command; store a versioned path in config and rewrite it on every
  upgrade.
- Consequences: Upgrades retain a stable config path while each launcher owns a
  coherent version. A missing/bad `current` link fails closed into repair UI.
- Evidence: `resolveWSLPayloadTarget`, helper health identity validation, and
  successful install/rollback smoke runs.

## D-042 — Treat optional command links as conditionally owned resources

- Date: 2026-08-09
- Status: Accepted
- Decision: Create absolute `~/.local/bin` links to the stable CATS `current`
  targets only when those paths are absent. Preserve an exact existing CATS
  link, refuse any collision, remove newly created links on rollback, and on
  uninstall remove only links whose targets still match CATS.
- Context: Blind `ln -sfn` can overwrite a user-owned command, while deleting
  the payload without deleting CATS-owned links leaves broken commands.
- Alternatives: Never create command links; always overwrite/remove the three
  names; copy executables into `~/.local/bin`.
- Consequences: Reinstall is idempotent, collisions fail before staging, and a
  user who reclaims a command path keeps it during uninstall. The exact managed
  links appear in the printed uninstall plan.
- Evidence: PowerShell plan tests and final real integration verifying all
  three links across install, rollback, and uninstall.

## D-043 — Enforce the qualified Ubuntu 24.04 WSL2 baseline during install

- Date: 2026-08-09
- Status: Accepted
- Decision: Phase 6 installation requires an amd64, non-root user in an actual
  WSL2 kernel and `/etc/os-release` identifying Ubuntu 24.04 LTS. WSL1 and other
  distributions fail before staging. Uninstall requires the target identity and
  safe paths but does not re-require Ubuntu 24.04, so a distro upgrade cannot
  strand payload files.
- Context: D-013 qualified only this baseline. A generic `microsoft|wsl` kernel
  match also accepts WSL1, and discovering a distribution is not evidence that
  its glibc/runtime was qualified.
- Alternatives: Warn but install anywhere; support every installed distro;
  parse localized `wsl --list --verbose` output as the only WSL2 signal.
- Consequences: Initial support claims match actual tests. New distro/releases
  require an explicit matrix decision and payload qualification.
- Evidence: WSL1/OS metadata tests, the actual prerequisite gate, and the final
  Ubuntu 24.04.3 WSL2 transaction.

## D-044 — Pin cross-boundary source line endings with attributes

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep Go, Make, and POSIX shell inputs LF-only; keep PowerShell
  scripts/modules CRLF in the assembled Windows workflow. Record this in
  `.gitattributes` rather than relying on each developer's `core.autocrlf`.
- Context: The native Windows checkout is also built through `/mnt/c`. Automatic
  CRLF conversion made Linux `gofmt -l` report the entire Go tree even though
  the source tokens were unchanged, and shell entry points must remain POSIX
  readable.
- Alternatives: Maintain separate Windows and WSL clones; normalize files
  manually before each WSL check; weaken formatting checks.
- Consequences: One checkout works predictably for both native toolchains.
  Authenticode signs the conventional CRLF PowerShell bytes placed in the
  Windows archive.
- Evidence: initial all-tree formatting failure followed by passing WSL
  `fmt-check` and complete serial verification after normalization.

## D-045 — Publish only from one final release gate

- Date: 2026-08-09
- Status: Accepted
- Decision: Native build jobs upload immutable workflow artifacts but never
  create or update a public release. One `publish` job depends on every native
  build, the signed Windows/WSL assembly, and the dedicated Windows 11/WSL2 GUI
  transaction. It recreates a draft with the complete artifact set and
  changes the draft to public only after every upload succeeds.
- Context: The recovered Phase 6 workflow let independent matrix jobs call the
  release API. A fast platform could therefore publish a partial release before
  the other half of the Windows/WSL pair or its smoke test failed.
- Alternatives: Keep matrix-local publication; publish the Linux files first and
  append Windows later; allow a manual operator to reconcile partial releases.
- Consequences: A missing runner, signature, artifact, or test leaves no public
  release. A failed upload can leave a recoverable draft, never a public partial
  transaction. A rerun deletes only that tag's draft (not its tag) before
  rebuilding it, so stale assets cannot leak into the published set.
- Evidence: `release.yml` job dependencies and its final draft/upload/publish
  step; actionlint validation of the dependency graph.

## D-046 — Separate release version, compatibility hash, and source identity

- Date: 2026-08-09
- Status: Accepted
- Decision: Use the exact `vMAJOR.MINOR.PATCH...` tag as `release_id`, the short
  Git hash stamped into launcher/helper health as `compatibility_version`, and
  the full 40-character commit as `source_commit`. Require the tag to resolve to
  `HEAD` from a full-history checkout. Encode version, both architectures, and
  the Ubuntu 24.04 floor in the Windows/WSL artifact names.
- Context: Phase 6 correctly paired local artifacts by short hash, but Phase 7
  also requires human release versions, full provenance, and names that reveal
  the two-platform support matrix.
- Alternatives: Replace runtime compatibility with the semantic version; name
  release files by hash only; trust `GITHUB_SHA` without resolving the tag.
- Consequences: Runtime mismatch checks remain exact and cheap, while public
  artifacts are understandable and traceable. Installer schema 2 accepts a safe
  semantic release directory name but still validates the hash identity; schema
  1 remains readable for Phase 6 development packages.
- Evidence: tag validation, both package builders, schema-2 manifests, package
  validator, and development artifact names recorded in `phase7_completion.md`.

## D-047 — Gate Windows publication on a labeled dedicated WSL2 GUI runner

- Date: 2026-08-09
- Status: Accepted
- Decision: Route the release-candidate transaction to a self-hosted Windows x64
  runner with the custom `cats-wsl2-gui` label, Windows 11, the qualified Ubuntu
  24.04 WSL2 distribution, WebView2, Go, and UCRT64. Repository variables select
  the dedicated distribution and non-root user. Absence of the runner or its
  variables blocks the release.
- Context: Hosted Windows runners do not guarantee a persistent, interactive
  Windows 11 + WSL2 + WebView2 environment. Treating a mocked Windows test as GUI
  evidence would overstate qualification.
- Alternatives: Skip GUI release gating; try to provision WSL afresh on a hosted
  runner; make the dedicated run advisory.
- Consequences: A tag can wait for the specialized runner and ultimately fail if
  it never becomes available, which is preferable to publishing an unexercised
  desktop pair. The custom label is declared in `.github/actionlint.yaml`.
- Evidence: `windows-11-wsl2-gui` job and the locally passing development-mode
  execution of `Test-CatsReleaseCandidate.ps1`.

## D-048 — Require Authenticode for public Windows archives

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep unsigned builds available only as explicitly labeled local
  development artifacts. A tag build must import a password-protected PFX from
  repository secrets, sign `Cats.exe` and all three PowerShell installer files,
  verify their actual `Valid` status against the manifest, attest the archive,
  and remove the imported certificate in an `always()` cleanup step.
- Context: D-017 selected a signed per-user archive, but Phase 6 only implemented
  optional signing and correctly produced unsigned development packages.
- Alternatives: Publish unsigned archives with checksums; sign only `Cats.exe`;
  trust a manifest claim without re-reading Authenticode status.
- Consequences: Missing/invalid secrets fail before assembly or publication. No
  certificate is stored in the repository or artifact. Developers can still test
  the exact transaction with `-AllowUnsignedDevelopment`, which never weakens the
  release workflow.
- Evidence: `-RequireSigned` fail-closed checks, package signature validator, and
  the release signing/import/cleanup steps. No local code-signing certificate was
  present, so signed publication itself remains an external release operation.

## D-049 — Layer protocol, process, package, and GUI integration evidence

- Date: 2026-08-09
- Status: Accepted
- Decision: Pin lifecycle bytes in NDJSON goldens run unchanged on Windows and
  Linux; test helper supervision with fake daemons; test the real helper with
  real `cathost`, `catway`, and `catctl probe`; test native Windows config/WSL/
  clipboard/retry seams; then run the signed archive through clean install,
  rollback, WebView2, pane create/command/close, native clipboard, relaunch,
  restore, and uninstall on the dedicated runner.
- Context: Unit tests alone cannot prove that both OS halves or WebView2 agree,
  while one large GUI test gives slow and imprecise failure signals.
- Alternatives: Use only mocked protocol tests; make the GUI walkthrough manual;
  duplicate separate golden fixtures for Windows and Linux.
- Consequences: Fast jobs catch contract drift early and the expensive gate
  proves the assembled artifact. The native Go test executable is stamped with
  the archive's compatibility hash, because an unstamped `dev` executable is
  correctly rejected by the real helper.
- Evidence: protocol goldens, `test-wsl-host-integration.sh`, focused Windows CI,
  `Test-CatsInstallerIntegration.ps1`, and `Test-CatsReleaseCandidate.ps1`.

## D-050 — Attach per-artifact GitHub provenance and a release manifest

- Date: 2026-08-09
- Status: Accepted
- Decision: Generate a GitHub artifact attestation for every native archive and
  for the release manifest/checksum set. Publish a deterministic top-level JSON
  manifest containing file names, sizes, SHA-256 digests, source commit, and the
  Windows-launcher-to-Ubuntu-payload mapping, alongside `SHA256SUMS` and generated
  GitHub release notes.
- Context: Checksums establish integrity only after a trusted party distributes
  them; Phase 7 also requires machine-readable provenance linking binaries to the
  workflow and commit that produced them.
- Alternatives: Checksums only; one attestation for a directory; embed all
  release metadata only inside the Windows zip.
- Consequences: Consumers can verify individual download subjects with GitHub's
  attestation tooling, while offline tooling can inspect the attached manifest.
  Public/private availability follows GitHub's artifact-attestation service.
- Evidence: `actions/attest@v4` steps and the locally verified
  `generate-release-manifest.sh` output.

## D-051 — Label Linux resource readings by their WSL boundary

- Date: 2026-08-09
- Status: Accepted
- Decision: Keep the existing Linux memory and home-volume measurements, but
  name their usage group `WSL VM` whenever the runtime has a WSL environment or
  Microsoft/WSL kernel marker. Use boundary-neutral memory/disk tooltips.
- Context: `/proc/meminfo` and `statfs` inside WSL do not report total Windows
  host memory or all Windows disks. The previous `Host` heading made correct
  values easy to misinterpret.
- Alternatives: Add a privileged Windows telemetry bridge; hide the rows on
  WSL; retain `Host` and explain it only in documentation.
- Consequences: The UI accurately describes the values without adding a new
  cross-boundary protocol. Native Linux and macOS retain the `Host` label.
- Evidence: `hostResourceScope`, its Linux/Windows/WSL table test, and the
  Ghostty-tagged Ubuntu package pass.

## D-052 — Allow-list both archive layers before installation

- Date: 2026-08-09
- Status: Accepted
- Decision: Require the Windows package to contain one exact regular-file/
  directory layout with no reparse points. Before extracting the payload in
  WSL, inspect `tar -tvzf` under `LC_ALL=C` and require the exact expected entry
  names and regular-file/directory types; reject links, duplicates, traversal,
  additions, and omissions. Extract without restoring owners or permissions.
- Context: Checksums prove expected bytes but do not reject an extra archive
  member or a symlink placed before a later member. Phase 8 requires archive
  traversal and symlink handling to be inspected as behavior, not assumed from
  an archive tool's defaults.
- Alternatives: Trust `Expand-Archive`/GNU tar defaults; validate only checksum
  paths; extract first and inspect afterward.
- Consequences: Adding a packaged file now requires an intentional allow-list
  update. A hostile link is rejected before it can redirect a later write.
- Evidence: installer layout/listing tests for valid, extra, traversal, and
  symlink cases; real schema-2 install; release-package ZIP preflight.

## D-053 — Make Phase 8 a release-candidate gate with measurable budgets

- Date: 2026-08-09
- Status: Accepted
- Decision: Extend the dedicated Windows 11/WSL2 release transaction with an
  eight-pane, 5,000-line, two-client workflow; tab/workspace create, focus,
  close, reorder, rename, and lock operations; actual loopback listener
  inspection; cold restore; and 20-second startup/restore plus 30-second
  functional budgets. Emit machine-readable timing metrics in test output.
- Context: The Phase 7 GUI transaction proved a narrow happy path but did not
  qualify the breadth or performance called for by Phase 8.
- Alternatives: Keep a manual checklist only; add unbounded smoke actions; run
  performance checks outside the release dependency graph.
- Consequences: Public release waits for a repeatable assembled-artifact test,
  while ordinary Windows CI remains fast. The budgets detect major regressions
  without pretending to be a microsecond GUI benchmark.
- Evidence: `TestWindowsNativeWSLPhase8Qualification`, extended `catctl probe`
  operations, the release workflow gate, and the local 1.126/2.140/1.728-second
  startup/functional/restore result.

## D-054 — Isolate distribution-wide disruption from owner-loss testing

- Date: 2026-08-09
- Status: Accepted
- Decision: Give Windows-owner loss and WSL distribution terminate/shutdown
  separate opt-in controls. Always run both on the dedicated idle release
  runner, but allow a local Phase 8 run to exercise owner loss without stopping
  unrelated WSL sessions.
- Context: `wsl --terminate` and `wsl --shutdown` affect every workload in their
  scope. The reference machine had two long-lived interactive Bash sessions
  during Phase 8 qualification.
- Alternatives: Run all disruptive cases behind one flag; skip all lifecycle
  disruption locally; terminate the shared distribution without inspection.
- Consequences: Release coverage remains fail-closed, while local validation
  does not destroy unrelated work. Distro termination, shutdown, reboot, and
  sleep/resume still need an idle/dedicated environment.
- Evidence: separate Go flags, separate candidate switches, mandatory release
  workflow arguments, and a passing isolated owner-loss run.

## D-055 — Treat WSL ext4 as the filesystem baseline and report drvfs ratios

- Date: 2026-08-09
- Status: Accepted
- Decision: Measure equal 64 MiB sequential write/read operations in an
  isolated WSL ext4 directory and an isolated `/mnt/c` directory during Phase 8
  qualification. Report raw values and describe the ratio; do not impose a
  universal drvfs ratio threshold.
- Context: The `/mnt/c` penalty depends on Windows build, storage, antivirus,
  and WSL filesystem implementation. A fixed multiplier would be a brittle
  release gate, but omitting the measurement leaves users without a defensible
  workspace recommendation.
- Alternatives: Benchmark only ext4; fail above a fixed ratio; publish an
  undocumented anecdotal warning.
- Consequences: Releases retain comparable measurements and documentation can
  recommend ext4 using current evidence. Temporary 64 MiB files are removed in
  a bounded `finally` path.
- Evidence: local ext4 143/106 ms versus drvfs 598/475 ms write/read results.

## D-056 — Bound the installed-Edge CDP harness and isolate its sandbox waiver

- Date: 2026-08-09
- Status: Accepted
- Decision: Put five-second bounds on CDP WebSocket/method operations and one-
  second bounds on endpoint polls. Run installed Edge headlessly with
  `--no-sandbox` only against a loopback server and a fresh disposable profile.
- Context: On the managed Windows session, sandboxed headless Edge accepted a
  CDP WebSocket but never answered `Runtime.enable`, leaving the old harness
  unbounded. A direct isolated `--no-sandbox` probe and the full regression both
  completed immediately.
- Alternatives: Keep the unbounded harness; disable browser integration; weaken
  production WebView navigation or process isolation.
- Consequences: This exception applies only to test Edge, never `Cats.exe` or
  its WebView2 runtime. Failures clean up deterministically and cannot hang CI.
- Evidence: the initial 120-second timeout, the bounded diagnostic failure, and
  the subsequent complete Edge regression pass in 3.5 seconds.

## D-057 — Browser-editable controls own their paste events

- Date: 2026-08-10
- Status: Accepted
- Decision: A paste event targeted at an `input`, `textarea`, or
  contenteditable region remains browser-owned. The document-level terminal
  paste fallback may cancel and forward clipboard text only when the event
  target is not editable.
- Context: Plugin Add and the other modal prompts intentionally let browser
  editing shortcuts operate normally, but their bubbling paste events were
  intercepted after keyboard routing and sent to the pane behind the overlay.
- Alternatives: Special-case only the plugin dialog; stop propagation in every
  individual form control; disable the document terminal paste fallback while
  any modal is open.
- Consequences: Native paste works consistently in plugin, rename, path,
  settings, chat, and future editable controls. Paste on the terminal surface
  keeps its existing fallback behavior, and new dialogs do not need bespoke
  clipboard listeners.
- Evidence: Pure policy tests cover editable and terminal targets; the
  installed-Edge regression opens Plugins → Add and observes an uncancelled
  input paste with no terminal paste message.

## D-058 — Synchronize canonical upstream with merge commits

- Date: 2026-08-10
- Status: Accepted
- Decision: Track `https://github.com/rohanthewiz/cats.git` as `upstream` and
  merge `upstream/main` into `wsl-ver`; do not rebase or squash the published WSL
  branch.
- Context: `origin/main` was exactly 30 commits behind canonical main, while
  `wsl-ver` already had 10 published Windows/WSL commits on their shared base.
- Alternatives: Rebase and force-push `wsl-ver`; cherry-pick 30 commits; replace
  the WSL branch with upstream and reapply platform work manually.
- Consequences: Both histories remain auditable and future syncs use ordinary
  three-way merges. The branch gains a merge commit and may require focused
  conflict resolution where both platforms changed the same code.
- Evidence: Merge base `44b0397`; upstream tip `5b92a5f`; pre-merge divergence
  was 10 WSL commits versus 30 upstream commits.

## D-059 — Resolve overlaps by composing platform behavior

- Date: 2026-08-10
- Status: Accepted
- Decision: Resolve sync conflicts at the smallest behavioral seam and retain
  both platform work and upstream features; never resolve a conflicted file by
  taking the whole `ours` or `theirs` version.
- Context: The dry run identified five conflicts in CI, launcher startup,
  resource usage, usage tests, and the web page. Each conflict contained valid
  work from both lines of development.
- Alternatives: Prefer WSL wholesale; prefer upstream wholesale; defer selected
  upstream commits indefinitely.
- Consequences: The launcher keeps `desktopWindow`, `sizeHintFixed`, and the
  platform descriptor while adopting Cats Mux branding. WSL labeling/tests
  coexist with upstream CPU behavior, and both CI matrices remain active.
- Evidence: The resolved staged tree has no conflict markers or whitespace
  errors; the full untagged WSL gate, native Windows seams, strict docs build,
  Node policy suite, and installed-Edge regression pass.

## D-060 — New global shortcuts use the shared platform policy

- Date: 2026-08-10
- Status: Accepted
- Decision: Represent the upstream sidebar fold shortcut as the
  `sidebar-toggle` action in `CatsPlatform.keyboardAction`; advertise and handle
  it through the same platform descriptor as palette, paste, and font actions.
- Context: Upstream handled `⌘B`/`Ctrl+Alt+B` directly in `index.html`, while
  the WSL implementation centralizes global key ownership to protect Windows
  editing chords, AltGr, IME, and server-side terminal encoding.
- Alternatives: Restore the direct inline handler; use `Ctrl+B` on Windows;
  omit the keyboard shortcut from the WSL version.
- Consequences: macOS retains `⌘B`, Windows uses `Ctrl+Alt+B`, AltGr is excluded,
  help text stays platform-correct, and unclaimed keys remain terminal-owned.
- Evidence: Dependency-free policy tests cover both platform actions and the
  Windows label; the full policy suite passes 10/10.

## D-061 — Upstream CPU readings share the WSL server boundary

- Date: 2026-08-10
- Status: Accepted
- Decision: Incorporate upstream host CPU history into the usage group while
  keeping the group name `WSL VM` under WSL and using server-neutral memory,
  CPU, and disk tooltip wording.
- Context: CPU, memory, and disk readings are collected by `catway` inside WSL;
  calling any of them total Windows host resources would violate D-051.
- Alternatives: Drop the upstream CPU row; label only memory/disk as WSL;
  implement a new Windows telemetry bridge during this sync.
- Consequences: The new CPU feature is available on WSL without changing the
  product’s resource boundary. A future Windows-host meter still requires a
  separate bridge and label.
- Evidence: Combined tests cover `Host` versus `WSL VM`, nil CPU startup, CPU
  row order/history/detail, and the WSL target suite passes.

## D-062 — CI and documentation validation are additive across branches

- Date: 2026-08-10
- Status: Accepted
- Decision: Keep the WSL branch’s Windows, WSL-host, protocol, Edge, and release
  gates and add upstream’s strict MkDocs job. Add the WSL2/Windows documentation
  corpus to the MkDocs navigation rather than suppressing omitted-file warnings.
- Context: Selecting either CI file wholesale would remove coverage required by
  the other platform. Upstream’s `omitted_files: warn` makes the existing WSL
  documentation an error under `mkdocs build --strict` unless it is navigable.
- Alternatives: Keep only upstream CI; keep only WSL CI; exclude the WSL docs
  from validation; weaken strict documentation checks.
- Consequences: Syncs cannot silently regress Windows/WSL behavior or docs link
  health, and the implementation/bugfix/decision/sync logs are available in the
  generated documentation.
- Evidence: The merged workflow contains all prior platform jobs plus `docs`;
  the navigation lists every WSL2/Windows document.

## D-063 — Preserve platform-neutral upstream visual patches intact

- Date: 2026-08-10
- Status: Accepted
- Decision: When an incremental upstream commit is confined to browser-rendered
  HTML/CSS/SVG, has no conflict, and does not alter a Windows/WSL ownership seam,
  merge it intact and verify the page contract, platform policy, and installed
  Windows browser rather than rewriting it into WSL-specific code.
- Context: Upstream `322117a` changes only the sidebar mark’s dimensions,
  aspect-ratio behavior, stroke, overflow, and explanatory comments. The same
  page is intentionally shared by macOS, ordinary browsers, and WebView2.
- Alternatives: Fork the visual design for Windows; manually reproduce the
  patch; reject all upstream UI changes unless they include WSL-specific tests.
- Consequences: The WSL version stays visually synchronized without creating a
  platform-only brand variant or needless future conflicts. Platform-boundary
  changes still require composed conflict resolution under D-059.
- Evidence: Both merge dry run and real merge were conflict-free; the resulting
  page passed all 10 platform-policy tests, focused `cmd/catway` tests, and the
  installed-Edge regression.

## D-064 — Shortcut creation is part of the install transaction

- Date: 2026-08-10
- Status: Accepted
- Decision: Publish Windows shortcuts only after the installed launcher/backend
  smoke succeeds. Commit a shortcut only after a fresh readback matches its
  target, arguments, working directory, and icon; retry one failed save from a
  removed/fresh `.lnk` and roll back if the second result is invalid.
- Context: The real `3a3f1e6` install produced nonempty Start-menu and desktop
  `.lnk` files whose embedded strings mentioned the versioned executable, but
  both WScript and Shell APIs resolved their target as empty. `Save()` did not
  signal an error, and existing integration checked only executable payloads
  and WSL command links.
- Alternatives: Accept file existence as success; repair only the current
  desktop link manually; launch the shortcut as verification and leave a GUI
  process running; retry without a bound.
- Consequences: A candidate that fails smoke never becomes a shell entry point,
  and the smoke cannot disturb links that have not yet been written. A transient
  serialization failure gets one safe recovery attempt, a persistent failure
  uses existing rollback, and a separate-process test covers both managed links
  without opening the GUI.
- Evidence: B-008 and the expanded isolated installer transaction.

## Open and recently closed decisions

### O-001 — Supported WSL distributions and glibc floor

- Status: Closed by D-013 for the initial baseline
- Recommendation: Support one current Ubuntu LTS distribution initially, build
  on the oldest supported baseline, then add distributions after automated
  payload/runtime qualification.
- Why it matters: The ghostty library is static, but the CGO Linux executables
  are dynamically linked to glibc. “Any WSL distro” is not a safe release claim.

### O-002 — Initial CPU architectures

- Status: Closed by D-013 for the initial baseline
- Recommendation: Ship `windows/amd64 + linux/amd64` first unless an ARM64 test
  device/runner is available; do not publish an untested ARM64 pairing merely
  because both toolchains can compile it.

### O-003 — Installer and signing format

- Status: Closed by D-017
- Recommendation: Start with a per-user archive plus Authenticode-signed
  PowerShell installer and signed `Cats.exe`; move to MSIX only after update,
  app identity, protocol activation, and WSL payload transaction behavior are
  proven.
- Trade-off: MSIX improves Windows identity/uninstall/signing UX but cannot by
  itself own the Linux-side installation transaction.

### O-004 — WebView2 notification implementation

- Status: Closed by D-038
- Recommendation: Use standards-based Web Notifications if the pinned WebView2
  produces reliable background notifications and click callbacks. Otherwise
  add a narrow Windows notification bridge.
- Acceptance: attention/finished notification appears while unfocused, click
  focuses CATS and reveals the correct pane, and no arbitrary activation data
  can be injected.

### O-005 — Windows accelerator mapping

- Status: Closed by D-035
- Recommendation: Preserve `Ctrl+Alt+K` as the cross-platform palette chord,
  put font sizing and editing in a native menu, and do not reserve common
  terminal Control chords. Validate AltGr and non-US layouts before finalizing.

### O-006 — Multiple local windows/instances

- Status: Open; may close after first local prototype
- Recommendation: Match current Mac behavior initially: independent launchers
  get private sockets and ports. Add single-instance activation only if Windows
  user testing shows duplicate sessions are confusing.

### O-007 — Host resource labels

- Status: Closed by D-051
- Recommendation: Keep existing Linux memory/disk readings but label/document
  them as WSL distribution/VM resources. Do not claim they represent total
  Windows host memory or disk without a separate Windows telemetry bridge.

## Decision update template

```text
## D-NNN — Title

- Date: YYYY-MM-DD
- Status: Proposed | Accepted | Superseded by D-NNN
- Decision:
- Context:
- Alternatives:
- Consequences:
- Evidence:
```
