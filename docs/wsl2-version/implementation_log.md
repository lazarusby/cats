# WSL2 implementation log

This is an append-only running record. Decisions belong in
[decision_log.md](decision_log.md); this file records work performed, evidence,
and remaining blockers.

## 2026-08-09 — Phase 0 baseline and lifecycle contract

- Confirmed repository baseline commit
  `44b0397827b973c031e67937c04ffb332e5c2206`.
- Captured the reference Windows/WSL/Ubuntu/CPU/WebView2/network facts in
  [phase0_baseline.md](phase0_baseline.md).
- Fixed the initial implementation matrix to Windows 11/amd64 and WSL2 Ubuntu
  24.04.3 LTS/amd64. Current measured Windows/WSL versions are qualification
  baselines rather than promises about older versions.
- Archived the current browser probe by commit, path, and SHA-256 and derived
  [windows_parity_checklist.md](windows_parity_checklist.md).
- Implemented `internal/desktopproto`: protocol v1, directional helper/launcher
  records, strict record validation, loopback-ready validation, mandatory
  newline framing, 16 KiB bound, and forward-compatible additive fields.
- Added tests for all Phase 0 cases plus encoder short writes and cross-record
  field validation.
- `make check`: pass on WSL Ubuntu 24.04.3 LTS.
- `make binaries`: pass; all three shipped binaries are Linux x86-64 ELF.
- `make macapp` and `make macapp-client`: not run because this host is not a
  Darwin/Cocoa builder. This is outstanding Phase 0 evidence.
- Visual walkthrough recording: pending until the native WebView2 spike runs.

## 2026-08-09 — Phase 1 WSL execution and networking spike

- Added disposable spike sources under `spikes/wsl2`.
- Built the Linux `hostprobe` with Go 1.26.5 and launched it from Windows using
  direct `wsl.exe --distribution Ubuntu --user bylaz --exec ...` arguments.
- Windows `Invoke-WebRequest` reached the WSL listener at
  `http://127.0.0.1:18421/` and received HTTP 200.
- Version 1 starting/ready/stopped records crossed redirected stdout; the stop
  JSON crossed stdin. Requested graceful stop passed.
- Repeated startup and closed the owning stdin pipe. The helper observed EOF,
  shut down, reported `reason=stdin_eof`, and exited within the bound. Pass.
- With mirrored networking, a WSL loopback listener blocked a Windows bind to
  the same port, and a Windows loopback listener blocked the WSL bind. Pass as
  conflict evidence; the future launcher still needs bounded whole-start retry.
- Owner-confirmed existing-backend check: the current WSL implementation was
  usable through a Windows 11 Chrome tab as documented in installation section
  4.5.
- NAT mode was not tested because the active host is configured for mirrored
  networking and switching modes requires a disruptive WSL shutdown.
- Task Manager kill, `wsl --terminate`, and `wsl --shutdown` remain pending for
  the production helper/resilience qualification; they were not run against the
  shared development distribution.

## 2026-08-09 — Phase 1 Windows desktop spike status

- Confirmed Windows 11 has Evergreen WebView2 Runtime 151.0.4129.72.
- Confirmed the native Windows `go.exe`, UCRT64 `gcc.exe`, `g++.exe`, and
  `windres.exe` are not currently installed/on `PATH`.
- Added a build-tagged WebView2 spike covering navigation, `Bind`, `Dispatch`,
  `Eval`, resize, cookies, WebSocket, manual notifications, keyboard events,
  new-window behavior, and teardown.
- WebView2 build/run, damaged-runtime behavior, Go/Win32 clipboard, keyboard
  layout/IME, notification focus, and external-link binding isolation are
  pending the Windows developer toolchain and interactive Windows testing.
- Retained the pinned `webview_go` dependency because there is no failed native
  behavior justifying an update.

## 2026-08-09 — Phase 2 common launcher refactor

- Split `cmd/catapp` common mode/window/config/lifecycle flow from Darwin menu,
  clipboard, signal, shell-environment, app-data, and supervision adapters.
- Added the narrow `localBackend` (`URL`, `Stop`) seam. The existing Darwin
  daemon pair implements it without changing sibling resolution, loopback
  arguments, readiness polling, process groups, stop order, or socket cleanup.
- Added a launcher-owned `desktopWindow` interface and a direct `webview_go`
  adapter. Common files no longer import the cgo webview package, Cocoa,
  `syscall`, or Windows APIs.
- Preserved the `defaultMode` link-time variable and common local/remote flow.
  Unknown modes still fall back to local mode.
- Split app-data paths: Darwin remains
  `~/Library/Application Support/cats`; Windows resolves
  `%LOCALAPPDATA%\Cats` and fails explicitly if `LOCALAPPDATA` is unavailable.
- Added Windows Phase 2 adapters for console interrupt and compilation shape.
  Native menu/bridges are no-ops and local mode fails closed with a visible
  not-implemented error; real WSL startup and privileged bridges remain Phase 4.
- Renamed Darwin source files to `clipboard_darwin.go`,
  `shellenv_darwin.go`, and `supervise_darwin.go`; Cocoa Objective-C remains
  Darwin-only.
- Added `catapp_headless` test infrastructure and `make test-catapp-common`, now
  included in `make check`. Headless tests pass for config, mode selection,
  pages, remote flow, local flow, lifecycle idempotence, cleanup order, and zoom.
- Full `make check`: pass after the refactor, including the new headless catapp
  target and existing ghostty race suite.
- `make binaries`: pass; the shipped Linux daemon/CLI set remains the same
  `catway`, `cathost`, and `catctl` payload.
- Added Darwin-only tests for app-data path, clipboard binding names, TCP
  readiness, and real helper-process stop order. They are committed as coverage
  but were not executable on this WSL host.
- Downloaded only the already-pinned `webview_go` module for compile inspection;
  `go.mod` and `go.sum` were unchanged.
- Native Windows cross-compilation remains blocked by the absent MinGW cgo
  compiler. Native macOS builds/tests remain blocked by the absent Darwin SDK
  and Cocoa environment.
- Retained the owner-confirmed Windows Chrome → existing WSL backend result in
  installation section 4.5; Phase 2 does not alter that backend/browser path.

## 2026-08-09 — Phase 3 WSL host helper

- Began from Phase 2 commit `d5a824a` on the `wsl-ver` branch; the existing
  WSL2 documentation directory remains part of the untracked documentation
  work rather than an unrelated repository deletion or replacement.
- Added Linux-only `cmd/cats-wsl-host` with the narrow port, launch-ID,
  start-directory, idle-timeout, health, and explicit development-override
  flags. Ordinary startup refuses non-WSL Linux and malformed or relative
  values.
- Kept `make binaries` and the personal Linux install at three public
  binaries. Added `WSL_BINS := cats-wsl-host` and `make wsl-payload`, which
  builds `catway`, `cathost`, `catctl`, and the helper with one build stamp.
- Implemented `--health-json`. On the reference host it reported architecture
  `amd64`, home `/home/bylaz`, payload path `/home/bylaz/project/cats/bin`, and
  helper version `d5a824a-dirty`; all three sibling payload executables passed
  the regular-file/executable checks.
- Moved marker extraction and PATH merging into standard-library-only
  `internal/shellenv`; Darwin `catapp` now uses the same tested primitives.
  WSL user hydration selects `SHELL`, passwd shell, or `/bin/sh`, performs one
  five-second marker-fenced login/interactive probe, puts derived Linux PATH
  entries before inherited interop entries, and normalizes `HOME`, `USER`,
  `LOGNAME`, and `SHELL`.
- Implemented private per-launch runtime allocation. Secure, selected-UID-owned
  `XDG_RUNTIME_DIR` uses `cats/<launch-id>`; all other cases use a mode-0700
  temp path containing the numeric UID and random suffix. Socket names are
  checked against the Linux Unix-socket limit. Cleanup is exact and
  non-recursive.
- Implemented daemon supervision with sibling-only resolution, separate
  process groups, child stdout/stderr redirected to helper stderr, loopback TCP
  readiness plus concurrent early-exit detection, protocol/EOF/signal/child
  monitoring, one wait owner per child, and idempotent reverse-order teardown.
  `catway` receives a five-second graceful save/final-capture budget before
  escalation; `cathost` receives three seconds. Unexpected child exit is
  reported and does not restart automatically.
- Added tests for flag/token/path validation, WSL detection and explicit
  override, payload checks, health inputs, shell selection and marker parsing,
  PATH/environment normalization, secure XDG selection and insecure fallback,
  socket length, exact cleanup with real Unix sockets, protocol stop and EOF,
  signal shutdown, partial startup, readiness timeout, unexpected child exit,
  duplicate stop, malformed control input, and forced kill of an unresponsive
  fake child. `go test -count=1 ./cmd/cats-wsl-host ./internal/shellenv
  ./internal/desktopproto` and focused vet passed.
- `go test -race -count=1 ./cmd/cats-wsl-host`: pass. Full `make check`: pass,
  including untagged vet/build/tests, headless common `catapp`, ghostty vet, and
  the complete ghostty-tagged race suite.
- `make wsl-payload`: pass. The output contains four Linux/amd64 ELF payload
  binaries and the helper's health check passed from that sibling layout.
- Ran a bounded real-payload smoke on `127.0.0.1:18429` with the repository as
  its explicit start directory. The helper emitted protocol-only `starting`,
  `ready`, and `stopped` (`reason=requested`) records; real `catway`/`cathost`
  logs remained on stderr. `catway` performed its graceful persistence path,
  `cathost` was reaped second, `/run/user/1000/cats/phase3-smoke` was removed,
  and no payload process remained.
- The owner-confirmed Windows Chrome → WSL result remains in installation
  section 4.5. The production helper is now verified inside WSL2, but Windows
  `wsl.exe` launcher integration and destructive distribution/owner-loss cases
  remain Phase 4/release qualification rather than Phase 3 claims.

## 2026-08-09 — Phase 4 Windows launcher core

- Began from committed Phase 3 baseline `d91aa1d` and preserved the existing
  untracked WSL2 documentation work.
- Extended `appConfig` with distribution, Linux user, and absolute WSL payload
  path while leaving remote URL/label compatibility unchanged. Replaced direct
  config writes with same-directory staging, file sync, and platform replace;
  Windows uses `MoveFileExW(REPLACE_EXISTING|WRITE_THROUGH)` under inherited
  LocalAppData DACLs.
- Added standard-library `internal/wslclient` for target/path validation,
  direct `wsl.exe` argument vectors, UTF-8/UTF-16LE distribution decoding,
  bounded helper-health decoding, desktop-protocol startup ordering, retry
  classification, strict loopback HTTP identity, JSON bounds, and log
  redaction. No `cmd.exe`, PowerShell, `sh -c`, or assembled command string is
  used by normal startup.
- Added a Windows first-run/repair form. It displays discovered distributions,
  accepts user/payload identity, validates again, shows a local starting page,
  launches the backend asynchronously, and saves only after health and startup
  succeed. It never installs or mutates a distribution automatically.
- Replaced the Phase 2 fail-closed Windows backend stub with real `wsl.exe`
  ownership: 15-second discovery/health bounds, cryptographic launch IDs,
  Windows-reserved loopback ports, redirected protocol pipes, 20-second helper
  readiness, exact version/PID/address checks, Windows-side HTTP verification,
  three-attempt classified bind/forwarding retry, protocol stop, 12-second proxy
  bound, and one-shot cleanup.
- Added a 1 MiB rotating launcher log under
  `%LOCALAPPDATA%\Cats\logs\launcher.log`. Helper stderr goes there rather than
  protocol stdout; token/password/secret/authorization-shaped assignments are
  redacted. User-facing startup errors contain stage, distribution, stable
  payload hash, and log path, not environment dumps or clipboard content.
- Added `/.well-known/cats/health` to catway with `{product,version}` JSON.
  Windows verification rejects redirects, non-HTTP/non-loopback/userinfo/query
  URLs, invalid ports, non-200 status, oversized/extra JSON, wrong product, and
  mismatched build hash before navigation.
- During the real cross-boundary test, helper health initially reported
  `d91aa1d-dirty` while the cross-compiled Windows test reported `d91aa1d`.
  This exposed that toolchain `vcs.modified` metadata is not stable across build
  modes. `buildinfo.Version()` now uses only the shared stamped hash; dirty
  remains separate diagnostic metadata. Rebuilt Windows and Linux artifacts
  then matched at `d91aa1d`.
- Added `make test-catapp-windows-compile`, which builds a stamped Windows/amd64
  PE test executable without cgo/WebView2. Executed that PE on the reference
  Windows host through WSL interop: Windows LocalAppData resolution, atomic
  replacement, config/fallback tests, bounded output, retry policy, lifecycle,
  pages, and clipboard binding installation all passed.
- Re-ran the complete no-cgo Windows PE suite after the final security gate;
  every executable Windows/common test passed and the opt-in live integration
  skipped by default as intended. The product bridge-gating test confirms no
  privileged binding is exposed before navigation enforcement exists.
- Added an opt-in Windows/WSL integration test and ran it against distribution
  `Ubuntu`, user `bylaz`, and `/home/bylaz/project/cats/bin`. The Windows process
  discovered the distribution, ran helper health, started the real helper via
  direct arguments, received the Phase 3 lifecycle records, verified catway's
  build-stamped health endpoint from Windows, and stopped cleanly in 1.88s.
  No `catway`/`cathost` process or per-launch runtime directory remained.
- After the final line-buffered log redaction and split-write bind-classifier
  changes, rebuilt all four stamped WSL payload binaries, reran `make check`,
  executed the complete Windows PE suite, and reran the live Windows-to-WSL
  integration. All passed; the final live run completed in 1.20s with clean
  ordered shutdown.
- Implemented the Win32 `CF_UNICODETEXT` clipboard bridge and pure UTF-16
  validation tests. The Windows compile and binding-name tests pass, but no
  interactive clipboard mutation was performed, and the bridge remains gated
  from product WebViews until top-level/new-window origin enforcement exists.
- Preserved the section 4.5 owner-confirmed Windows Chrome → WSL result. The
  Phase 4 Windows HTTP test adds launcher evidence; it does not claim native
  WebView2, menu, clipboard, cookie, or navigation-boundary completion.
- Full `make check`: pass after Phase 4, including untagged vet/build/tests,
  headless common launcher tests, the stamped Windows/amd64 compile target,
  ghostty vet, and the full ghostty-tagged race suite. `make wsl-payload`,
  formatting, and diff whitespace checks also pass.

## 2026-08-09 — Native Windows completion pass for Phases 3 and 4

- Moved execution from VS Code/WSL2 to the native Windows checkout at
  `C:\Users\bylaz\source\cats`. Confirmed the native prerequisites were already
  installed but absent from the inherited `PATH`: Go 1.26.5 at
  `C:\Program Files\Go` and MSYS2 UCRT64 GCC 16.1.0 at `C:\msys64\ucrt64`.
- Built the existing Phase 1 WebView2 spike as a native `windows/amd64` GUI PE.
  The installed pinned WebView2 wrapper and Evergreen runtime compiled without
  a dependency update.
- Queried the official Go module and confirmed its latest revision is still the
  pinned 2024 commit and still does not expose navigation or new-window
  callbacks. Copied that 1.02 MiB MIT-licensed wrapper into
  `third_party/webview_go`, retained its version with a local `replace`, and
  added only Windows controller hooks for navigation, new windows, and
  accelerator events.
- Added a normalized trusted-origin policy. Same-origin HTTP(S) routes remain in
  the main view, cross-origin HTTP(S) and every HTTP(S) new window go to the
  default browser, and `file:`, `data:` after trust, `javascript:`, userinfo,
  malformed, and other unowned schemes fail closed. Remote configuration now
  validates the URL before saving or navigating.
- Enabled `catsClipRead`/`catsClipWrite` only after those native handlers attach.
  A hidden real WebView2 called the read bridge successfully, and a real
  `OpenClipboard` contention test passed the bounded retry path. The suite did
  not overwrite the user's system clipboard to test writes because restoring
  Unicode text cannot preserve arbitrary non-text clipboard formats.
- Added native Windows File/Edit/View menus with About, idempotent Quit,
  undo/redo/cut/copy/paste/select-all, text sizing, and keyboard help. Added
  controller accelerators for `Ctrl+Shift+C/V/X/A`, `Ctrl+Plus/Minus/0`, and F1;
  ordinary terminal Control chords remain page-owned. AltGr events fail the
  accelerator policy, and the web palette handler now checks `AltGraph`.
- Subclassed the native window so title close, menu Quit,
  `WM_QUERYENDSESSION`, and `WM_ENDSESSION` converge on the existing one-shot
  backend cleanup. The hidden Windows UI runner verified a real native menu and
  an actual `WM_QUERYENDSESSION` delivery.
- Replaced synchronous Windows local startup with one persistent asynchronous
  WebView window. Cold/slow WSL health and startup show a cancellable progress
  page; error/setup states expose dedicated Retry, Repair, Change Distribution,
  and Open Logs actions. Repair starts a sibling packaged `Install-Cats.ps1`
  only after an explicit click and reports when the future Phase 6 asset is not
  present. All launcher-only action bindings become inert before the trusted
  CATS page is navigated.
- Added hidden WebView2 integration coverage for same-origin navigation,
  `file:` and `javascript:` denial, `window.open` handling, clipboard binding,
  menu installation, session ending, and a persistent HTTP cookie across two
  destroyed/recreated views. The combined native suite passed.
- Added and passed a native end-to-end test that starts the build-matched Phase
  3 payload through `wsl.exe`, verifies Windows-side backend identity, loads the
  real CATS page in hidden WebView2 with the guarded clipboard binding, and
  performs ordered shutdown. The build-matched payload reported `4735d36`.
- Added repeatable Task-Manager-equivalent owner-loss, `wsl --terminate`, and
  `wsl --shutdown` qualification tests. The first owner-loss run exposed that
  closed Windows stdout caused default Linux `SIGPIPE` termination after child
  teardown but before the runtime-directory defer. `cats-wsl-host` now ignores
  `SIGPIPE`, receives `EPIPE`, and completes all defers.
- Rebuilt the WSL helper from the Windows checkout and reran owner loss. It
  passed in 2.43 seconds: the exact packaged helper/daemons exited, the backend
  URL closed, and the exact per-launch runtime directory disappeared. Removed
  the three verified-empty runtime directories created by the diagnostic runs.
- The execution safety reviewer rejected `wsl --terminate Ubuntu` plus
  `wsl --shutdown` because those commands intentionally interrupt potentially
  unrelated distribution workloads. Their tests remain opt-in and unexecuted
  pending explicit owner approval on an idle WSL host; no workaround was used.
- The native Windows checkout exposed CRLF-sensitive generated-golden and
  key-table tests. Updated the golden comparison and both key-table row loaders
  to normalize CRLF/LF, then reran `go test -count=1 ./cmd/catgen-dart` and the
  race-enabled Ghostty key-chain test successfully. The generated Dart files
  retain their original content.
- Final native validation passed: full Windows unit suite, hidden WebView2
  security/cookie/menu/session/clipboard coverage, real Windows→WSL backend,
  real WSL-backed CATS page inside WebView2, persistent-progress/actions test,
  final 2.24-second owner-loss regression, and a stamped 15,246,068-byte
  GUI-subsystem `bin/Cats.exe` build.
- Final cross-platform validation passed from the Windows checkout: untagged
  vet/build/tests, headless catapp, no-cgo Windows compile, ghostty vet, focused
  helper race tests, and the complete ghostty-tagged race matrix. The fully
  parallel race invocation on `/mnt/c` once outlived its five-minute outer
  harness and was terminated by exact PID; serial package execution (`-p 1`)
  retained the same coverage and passed all packages in 69 seconds.

## 2026-08-09 — Phase 5 web UI platform behavior

- Started Phase 5 from committed native-Windows baseline `a9dd4dc`. The Phase 3/4
  code is clean; the pre-existing `docs/wsl2-version` directory remains an
  untracked documentation working set.
- Audited the page and native shell seams. The page already prefers
  `catsClipRead`/`catsClipWrite`, suppresses visible-pane notifications, opens
  OSC 8 links in a new window, and retries WebSockets, but platform policy is
  scattered: help text is Mac-first, IME state is not explicitly rejected, the
  Windows menu's `pasteText`/`openHelp` hooks are not exported, and no browser
  behavior harness exists.
- Chose a dependency-free platform-policy module exercised by Node's built-in
  test runner, plus a real installed-Edge regression runner for DOM/WebSocket
  integration. This avoids adding a package manager or downloaded browser to
  the Go repository while still testing the actual served page on Windows.
- Kept the existing standards-based Notification API rather than adding a
  privileged native notification bridge. Its payload is already limited to
  title/body/pane identity and its click path focuses the window and sends only
  `agent.focus`; the completed Phase 5 path adds bounds, failure handling, and
  regression tests below.
- Added `desktopWindow.Init` and platform adapters that publish a frozen
  `catsDesktop` descriptor before navigation. The clipboard capability reflects
  actual bridge setup success rather than the OS name. Headless tests validate
  the injection contract, and hidden WebView2 reported `windows` plus native
  clipboard access from inside the loaded page.
- Added the dependency-free `web/platform.js` policy module and inline it into
  every rendered catway page. Centralized Windows/macOS/browser labels, palette,
  paste/font/edit ownership, AltGr detection, IME/dead/process-key rejection,
  native-first clipboard selection, notification decisions/bounds, safe external
  URLs, same-origin classification, and bounded reconnect delays.
- Updated the page to export the narrow `pasteText` and `openHelp` hooks used by
  the Windows native menu. Replaced hard-coded Mac help/toolbar strings with
  policy labels. Selection, copy mode, scrollback, and OSC52 continue through
  the shared clipboard writer; native read/write rejection produces a toast.
- Hardened OSC8 handling to open only credential-free HTTP(S) URLs. Unsafe
  `javascript:`, `file:`, malformed, oversized, and credential-bearing links are
  rejected in the page before they reach the platform new-window policy.
- Microsoft WebView2 documentation confirmed support for non-persistent Web
  Notifications and documented that the host must decide Notification
  permission. Extended the local wrapper hooks with `PermissionRequested`:
  allow only the Notifications kind for the current trusted CATS origin and
  deny all other kinds/origins. The hidden native test received `granted`.
- Added nine Node policy cases covering descriptor absence/presence, Mac/Windows
  labels, printable/Control/Alt/AltGr/IME/modifyOtherKeys routing, global/edit
  ownership, clipboard selection/failure, notification fallback/bounds,
  OSC8/origins, and reconnect backoff. All nine pass on Node 24.12.0.
- Added `scripts/test-webui-edge.mjs`, which uses the installed Edge and CDP with
  an isolated temporary profile—no npm dependencies or downloaded browser. The
  real page passed transient WebSocket recovery, keyboard/IME/AltGr, palette and
  font actions, native paste plus failure, paste context action, drag selection,
  copy mode, OSC52, notification suppression/fallback/click focus, and safe and
  blocked OSC8 cases. The temporary profile is removed and Edge is closed via
  the browser protocol after each run.
- Focused native validation passed: catapp compile/unit tests and the hidden
  Windows UI suite, including descriptor timing, trusted-origin notification
  permission, navigation, menu/session delivery, cookies, and clipboard. The Go
  tool reported a benign Windows temporary-test-executable unlink warning after
  the passing processes exited; it did not change test results.
- Final validation passed: native `cmd/catway` (including installed Edge), native
  `cmd/catapp`, the full untagged Linux suite, headless catapp, no-cgo Windows
  compile, and the Ghostty-tagged catway race suite. Rebuilt the GUI-subsystem
  `bin/Cats.exe` with the Phase 5 wrapper hooks (15,262,497 bytes). The final
  standalone Node policy and Edge runs also passed; no isolated Edge process or
  temporary profile remained.

## 2026-08-09 — Phase 6 build, install, update, and uninstall

- Recovered the interrupted working set after the hard reset before changing
  it. The checkout already contained separate Make targets, native payload and
  Windows distribution builders, resource/icon/manifest inputs, a PowerShell
  5.1 installer module and thin entry points, installer tests, the launcher
  `--install-smoke` gate, atomic `current` resolution, and an assembled matched
  `b5ba4df` package. The recovered PowerShell unit tests and focused Go tests
  passed. The existing isolated clean-install/smoke/uninstall/fresh-failure
  rollback test also passed, establishing the last known completed point.
- Added and verified the release-oriented entry points without changing normal
  Linux `make binaries`: `wsl-payload-dist`, `windows-launcher`,
  `windows-dist`, and `test-windows-installer`. The Linux packager accepts only
  Linux/amd64 ELF inputs and creates a reproducible archive containing the four
  stamped executables, example configuration, NOTICE, libghostty-vt license,
  checksums, and machine-readable release metadata.
- Confirmed the native Ubuntu 24.04 payload consists of four x86-64 ELF
  executables and that `catway` links only the ordinary Linux loader/libc path,
  not Linux GUI libraries. The archive reports release/compatibility
  `b5ba4df`, architecture `amd64`, and a measured maximum required GLIBC symbol
  version of 2.34.
- Completed the native Windows builder. It uses the installed Go/UCRT64 cgo
  toolchain, generates an icon, embeds version/icon/manifest resources, marks
  the executable `asInvoker`, Per-Monitor-V2 DPI aware and long-path aware,
  selects the GUI subsystem, stamps the same build identity as the payload,
  and supports optional CurrentUser Authenticode signing with timestamping.
- Completed the coupled distribution builder. It refuses a payload archive
  whose hash-bearing name differs from the Windows source revision, assembles
  launcher/payload/scripts/notices/licenses, generates package checksums and a
  compatibility manifest, records launcher/installer signature status, and
  emits `cats_<hash>_windows_wsl_amd64.zip`.
- Completed the installer prerequisite and target gates: 64-bit Windows/PowerShell
  context, Windows 11 reference build, amd64, Store WSL version, Evergreen
  WebView2 floor, explicit installed distribution/user resolution, non-root
  identity, an actual WSL2 kernel, x86-64 Linux, and the accepted Ubuntu 24.04
  LTS baseline. WSL1 and unsupported distro metadata now fail before staging.
  Uninstall intentionally does not require the old Ubuntu release identity, so
  a later distro upgrade cannot strand installed files.
- Completed the per-user install transaction. It verifies the outer package,
  stages both Windows and WSL halves in private new directories, copies the
  archive through WSL rather than executing from `/mnt/c`, verifies the copied
  archive and inner checksums, validates payload metadata/permissions/helper
  health, moves the versioned payload into place, atomically replaces the
  relative `current` symlink, merges `app.json` while preserving remote and
  unknown fields, creates Start-menu/optional desktop shortcuts, and commits
  only after the installed GUI-subsystem launcher starts, verifies, and stops
  the real backend through `--install-smoke`.
- Made upgrades race-safe by resolving the configured WSL `current` link once
  to a versioned directory before helper health and launch. The launcher and
  helper therefore cannot land on opposite sides of a concurrent symlink
  switch, and existing health/version checks reject a mixed Windows/Linux pair.
- Completed failure rollback for both fresh and existing installations.
  Config bytes and shortcut definitions are restored, Windows and WSL version
  directories are moved back, `current` is atomically restored, newly created
  command links are removed, and the installer log retains the failure and
  repair context. Prior version directories remain available rather than being
  garbage-collected during the upgrade transaction.
- Tightened optional `~/.local/bin` ownership after the recovery audit. Stable
  absolute links target `~/.local/lib/cats/current/<command>`; installation
  preserves an exact existing CATS link and refuses to overwrite any other
  file/link. Normal uninstall enumerates and removes only links whose targets
  still match CATS, preventing broken managed links without deleting a path the
  user subsequently reclaimed.
- Completed uninstall/data-preservation behavior. The default plan prints and
  removes only Windows versioned program files, Start-menu/desktop shortcuts,
  managed WSL command links, and the WSL payload root. It preserves Windows
  launcher config/logs, WSL config/state/plugins, worktrees, projects, and agent
  configuration. `-RemoveAllData` separately enumerates the three additional
  owned data roots, warns that sessions/configuration are unrecoverable, and
  still leaves projects/worktrees/external agent configuration alone.
- Expanded PowerShell policy coverage for checksum tampering/traversal paths,
  additive config merge, WSL1 and unsupported-distribution rejection, managed
  link enumeration, exact data-removal plans, and unexpected-root refusal. The
  PowerShell 5.1 test passes.
- Expanded the real Windows/WSL integration to use isolated roots, an explicit
  non-root user, and a Windows path containing spaces and `ü`. It verifies
  install plus real launcher/backend smoke, all managed command links, normal
  uninstall and config preservation, then installs a valid baseline, forces a
  replacement launcher to fail smoke, and proves byte-for-byte config,
  launcher digest, WSL `current`, command links, and working backend are all
  restored. A third isolated case proves a non-CATS `catway` path is preserved
  and fails before either platform is staged. The final rebuilt package passed
  in 25.0 seconds and removed every exact test root.
- Added `.gitattributes` rules for LF-only Go/Make/POSIX inputs and conventional
  CRLF PowerShell release inputs. This makes the native Windows checkout usable
  directly from WSL without `core.autocrlf` causing every Go file to fail the
  Linux formatting gate.
- Final focused native validation passed: installer policy tests; PowerShell
  parser checks; `go test -count=1 ./cmd/catapp ./internal/desktopproto
  ./internal/wslclient`; native Windows launcher build; package checksums; and
  the actual Windows 11/WSL/WebView2 prerequisite gate.
- Final cross-platform validation passed: `fmt-check`, untagged vet/build/tests,
  headless catapp, stamped no-cgo Windows compile, Ghostty vet, and the complete
  Ghostty-tagged race matrix in reliable serial mode (`-p 1`). The parallel
  `/mnt/c` race invocation again exceeded the bounded outer run with one helper
  test process; it was terminated and replaced by the serial matrix, which
  passed all packages with the helper completing in 8.028 seconds.
- A broad native `go test ./...` was not counted as a Phase 6 pass: the existing
  Windows `catctl` suite expects Unix-only integration installation to succeed,
  and the installed-Edge child exceeded the five-minute outer bound. Neither
  path is changed by Phase 6; focused native coverage and the complete WSL
  matrix above are the qualification evidence.
- Final development artifacts: `cats_b5ba4df_windows_wsl_amd64.zip` is
  33,215,907 bytes with SHA-256
  `07c829821d9ea11d700227a01e5979ef210a5cc2f18911b8d045f32f8f4ae717`;
  its payload archive is 27,336,276 bytes with SHA-256
  `0601b6c6864a0113627eb49426ebe3abee48d886664340278fd701946581f48f`.
  This local development build correctly records `NotSigned` and
  `release_candidate_signed=false`; D-017 still makes signing infrastructure a
  release prerequisite, so this is Phase 6 implementation evidence, not a
  publishable release candidate.
- The complete requirement/evidence matrix and operational details are in
  `phase6_completion.md`. Phase 7 workflow edits found in the recovered working
  set were preserved, but Phase 7 release qualification is not claimed here.

## 2026-08-09 — Phase 7 CI and release

- Started Phase 7 from commit `0316d46` and audited the Phase 6 handoff before
  editing. The recovered CI skeleton already had a native Windows launcher job,
  an Ubuntu 24.04 WSL-payload build, and a dependent Windows archive build.
- Identified three release-blocking gaps in that skeleton: matrix jobs published
  independently before all artifacts were known-good, unsigned Windows archives
  were publishable, and no dedicated Windows 11/WSL2/WebView2 result gated the
  release. The Phase 7 implementation therefore uses one final publish job after
  build, signature, manifest, provenance, and end-to-end gates.
- Began adding shared protocol-golden coverage, explicit Linux and Windows
  integration entry points, version/distro-qualified artifact names, and a
  machine-readable launcher-to-payload release manifest. Material policy choices
  are recorded as D-045 and later entries in `decision_log.md`.
- Added LF-only helper/launcher NDJSON goldens and one Go test that both encodes
  and decodes those exact files. The native Windows run passed in 6.943 seconds
  and the Ubuntu run passed in 0.011 seconds, giving both protocol participants
  one byte-for-byte compatibility contract.
- Added `test-wsl-host-integration.sh` and Make entry points for the golden and
  real integration. The script runs the actual helper on ordinary hosted Linux
  through its explicit development override, starts the real `cathost` and
  `catway`, uses `catctl probe` to run commands in two panes, closes the second
  pane, sends the version-1 stop record, and verifies starting/ready/stopped
  records. It passed in Ubuntu WSL in 6.1 seconds with ordered cleanup.
- Reworked ordinary CI into Ubuntu quick, native Windows fast, Linux WSL-host
  integration, and the existing three-platform Ghostty matrix. Windows now
  checks Go formatting, PowerShell parseability, focused vet/unit/platform
  seams, installer policy, shared goldens, and a real UCRT64/WebView2 CGO build
  without pretending the hosted runner provides persistent WSL2 GUI coverage.
- Added actionlint 1.7.12 to quick CI plus `.github/actionlint.yaml` for the
  intentional `cats-wsl2-gui` custom runner label. Both new workflows passed
  actionlint with no findings; PyYAML separately parsed the four CI jobs and six
  release jobs.
- Introduced schema-2 release identities. The semantic tag is the release/install
  id, the stamped short hash remains the runtime compatibility identity, and the
  full commit anchors provenance. Builders validate the tag against `HEAD` from
  full history and name the public Windows/WSL artifacts with version, Windows
  arch, Ubuntu 24.04 floor, and WSL arch. The installer remains compatible with
  Phase 6 schema-1 hash releases.
- Expanded the coupled Windows manifest with full source commit, explicit Windows
  and WSL architectures, distro floor, launcher/payload digests, compatibility
  mapping, all installer signature states, signer identity, and timestamp server.
  `Test-CatsReleasePackage.ps1` verifies outer and inner checksums/manifests,
  component mapping, actual Authenticode status, and the complete file name.
- Made public signing fail closed. `Build-WindowsDist.ps1 -RequireSigned` refuses
  to run without a certificate and refuses any invalid/unsigned launcher or
  installer file. The release workflow imports a base64 PFX/password from GitHub
  secrets, passes only its thumbprint to the builder, and removes it from the
  current-user store in an `always()` step. Unsigned local candidates remain
  explicitly available for development testing and cannot reach publish.
- Replaced matrix-local GitHub Release calls with one dependency-gated release
  transaction. Native jobs only attest and upload workflow artifacts. Publication
  requires every native archive, the signed Windows assembly, and the dedicated
  WSL2 GUI job; it generates a top-level JSON map and `SHA256SUMS`, attests them,
  replaces only a same-tag draft to exclude stale assets, uploads the complete
  set with generated notes, then publishes.
- Added the dedicated `self-hosted, Windows, X64, cats-wsl2-gui` job and
  `Test-CatsReleaseCandidate.ps1`. It validates the signed archive, reruns the
  Phase 6 clean install/rollback suite, installs into isolated roots, drives real
  WebView2 and WSL, creates/closes a pane, runs commands, round-trips the native
  clipboard, relaunches, confirms state/output restore, and uninstalls. XDG roots
  pass through `WSLENV` so the persistent dedicated user is not polluted.
- The first local candidate run correctly failed because `go test` produced an
  unstamped `dev` launcher while the payload reported `0316d46`. Updated the
  qualification harness to take `-ldflags` from the package's compatibility
  manifest, preserving the same mismatch enforcement as production. The next
  full development-candidate run passed in 53 seconds; the native WebView2/WSL
  tests took 8.421 seconds and all exact temporary roots were removed. After the
  installer gained final schema-2 source/architecture/distro/component checks,
  the definitive rebuilt archive repeated the full transaction in 55.5 seconds
  with native tests taking 8.687 seconds and exact cleanup passing again.
- Rebuilt the new development payload and coupled archive and passed the package
  validator plus the complete Phase 6 install/smoke/uninstall/existing-upgrade
  rollback/link-collision integration in 27.3 seconds. The manifest/checksum
  generator also produced the expected two-artifact map and its exact temporary
  test directory was removed.
- Broader Ubuntu validation passed: format, untagged vet/build/all tests, headless
  catapp, stamped no-CGO Windows compile, and Ghostty vet. PowerShell parser and
  installer-policy checks passed. Native focused Go results were all `ok`; one
  combined invocation hit a transient test-executable deletion lock only after
  results were emitted, and the protocol package rerun exited cleanly.
- A full Ghostty race rerun on `/mnt/c` was not counted. A 60-second bound and a
  second 184-second serial bound repeated the known helper-timeout hang without
  an assertion. The five exact orphan helper/fake-daemon PIDs were terminated;
  a later test process exited independently; final process inspection was empty.
  Phase 6 already records the complete serial pass at this implementation base,
  and hosted Linux CI runs the matrix on its native filesystem.
- Final unsigned development artifacts: payload
  `cats-wsl-payload_0316d46_ubuntu-24.04_linux_amd64.tar.gz` is 27,337,121 bytes
  with SHA-256 `95ca60696bd3904f7dde9237e5e7fd8d7c637f2db3b5e6b169e7dd111a8bb482`;
  coupled archive `cats_0316d46_windows_amd64_wsl_ubuntu-24.04_amd64.zip` is
  33,218,777 bytes with SHA-256
  `106ec627d23f2406eb4e5a83743be2eb8ec984022a2b1a21453bfdac06b9479a`.
- No local code-signing certificate was present and no tag was pushed. Phase 7
  source implementation is complete, but an operator must configure the two PFX
  secrets, distro/user variables, and labeled runner before the fail-closed
  workflow can create a signed, attested public release. Full evidence and the
  Phase 8/9 boundary are in `phase7_completion.md`; decisions are D-045–D-050.

## 2026-08-09 — Phase 8 qualification

- Started from committed Phase 7 head `83593df` with only the previously
  untracked `docs/wsl2-version` directory in the worktree. Read the complete
  functional, resilience, security, and performance checklist and mapped it to
  existing unit, browser, helper, installer, package, and native evidence before
  adding new coverage.
- Found that Linux memory and disk were still headed `Host` inside WSL. Added a
  WSL environment/kernel classifier so the group reads `WSL VM`, made tooltips
  server-boundary-neutral, and added deterministic ordinary-Linux/Windows/WSL
  cases. This closes O-007 through D-051.
- Audited archive handling. Existing checksum-path traversal protection did not
  exclude extra package members or a payload symlink that could redirect a later
  extraction write. Added an exact Windows package file/directory allow-list,
  reparse-point rejection, ZIP-entry preflight in the release validator, and a
  pre-extraction `LC_ALL=C tar -tvzf` type/name allow-list for the payload.
  Payload extraction now also uses `--no-same-owner --no-same-permissions`.
  PowerShell tests cover valid layout, unexpected file, traversal member, and
  symlink replacement; the real package install passed the same checks (D-052).
- Extended `catctl probe` with `tabmove`, `wsclose`, `wsrename`, `wsmove`, and
  `wslock` operations and re-exported the missing workspace-lock browser command
  type. This lets an assembled candidate exercise rather than merely unit-test
  the full workspace/tab session vocabulary.
- Added `TestWindowsNativeWSLPhase8Qualification`. It verifies the live listener
  is exactly loopback, runs two simultaneous clients, creates alternating
  horizontal/vertical splits to an eight-pane peak, resizes, zooms, renames,
  writes 5,000 terminal lines, closes back to one pane, creates/reorders/closes
  tabs and workspaces, locks/unlocks a workspace, stops, relaunches, and asserts
  persisted terminal output. It emits JSON metrics and enforces 20-second
  startup/restore plus 30-second functional budgets (D-053).
- Extended the release-candidate script with Phase 8 timing and equal 64 MiB
  ext4/`/mnt/c` sequential I/O measurements. All files live under GUID-qualified
  roots and are removed in `finally`. The release job now requires this phase and
  has a 45-minute bound (D-055).
- Separated simulated Windows-owner loss from distribution-wide terminate and
  shutdown controls. The dedicated `cats-wsl2-gui` release job requires both;
  local qualification can test owner loss without stopping a shared distro.
  Before the local run, bounded process inspection found two long-lived Bash
  sessions, so `wsl --terminate`/`--shutdown` were correctly not invoked
  locally (D-054).
- Made the installed-Edge harness bounded: endpoint fetches have a one-second
  abort and CDP open/method calls have five-second bounds. The first run reached
  the old 120-second outer timeout; its exact newly orphaned temporary Edge
  profile was verified under `%TEMP%` and removed (not recoverable). The bounded
  rerun identified `Runtime.enable` as the stuck operation. Direct Edge probing
  showed this managed session needed `--no-sandbox`; limiting that switch to the
  loopback/disposable-profile test produced a complete pass in 3.5 seconds.
  Production `Cats.exe`/WebView2 behavior is unchanged (D-056).
- PowerShell parser checks and the expanded installer policy suite passed. Native
  compilation of `cmd/catctl`, `internal/browserproto`, and `cmd/catapp` passed.
  Ubuntu focused tests passed for the probe/protocol/app packages and the
  Ghostty-tagged gateway/resource-label path.
- Rebuilt the WSL payload for development stamp `83593df`. The first coupled
  Windows archive was rejected by the new validator: a relative
  `-OutputDirectory dist` caused its checksum paths to contain a substring of
  the staging-directory name. Canonicalized the builder output root, rebuilt,
  and passed the complete outer/inner release-package validator. This was a
  real pre-existing packaging defect found by Phase 8 hardening.
- The first unskipped assembled-candidate run passed package validation and the
  full Phase 6 install/smoke/uninstall/rollback suite, then failed both native
  clipboard assertions. `Set-Clipboard` and `Get-Clipboard` also failed outside
  CATS in the current desktop session, proving the environment's clipboard
  subsystem was unavailable. A separate native WebView rerun later faulted in
  WebView initialization. These are recorded as current-session blockers; the
  release workflow has no skip and therefore remains fail-closed. Phase 7's
  successful native WebView2/clipboard result is retained only as prior evidence.
- Added a development-only native-UI skip that is rejected unless
  `-AllowUnsignedDevelopment` is also explicit. Using it, the complete isolated
  Phase 8 backend transaction passed in 63.1 seconds: startup 1,126 ms;
  eight-pane/5,000-line/two-client workflow 2,140 ms; restore 1,728 ms; owner
  loss 1.64 seconds. Ext4 write/read measured 143/106 ms and `/mnt/c` measured
  598/475 ms, 4.18×/4.48× slower. Exact install and performance roots were
  removed.
- Repository-wide Ubuntu validation passed in 66.1 seconds: `fmt-check`, vet,
  untagged build/all tests, headless catapp, stamped Windows no-CGO compile, and
  Ghostty vet. The complete Ghostty-tagged non-race suite passed in 21.7 seconds.
  Focused race tests passed for `cmd/catctl`, `internal/browserproto`, and
  Ghostty-tagged `cmd/catway`. Nine platform-policy Node tests passed. actionlint
  1.7.12 passed both workflows after the final CI/release changes.
- Added the Edge regression to ordinary native Windows CI. The public release
  remains dependent on the extended signed candidate and its dedicated idle
  WSL distribution, including native UI, owner loss, terminate, and shutdown.
- Updated the parity checklist, detailed plan status, decisions D-051–D-056,
  and added `phase8_qualification.md` with every automated pass, prior result,
  current blocker, manual matrix item, metric, artifact, and cleanup result.
  Phase 8 source/test implementation is complete. Honest product sign-off still
  requires the signed dedicated-runner result plus reboot, sleep/resume, NAT,
  VPN/firewall, real external agent/account, multiple-launcher, dated visual,
  and comparable macOS performance evidence.

## 2026-08-10 — Editable-control paste routing bugfix

- Reproduced the reported Plugins → Add failure in the event flow: modal
  keyboard handling left `Ctrl+V` to the browser, then the bubbling document
  `paste` listener cancelled the field's native paste and forwarded the text to
  the terminal.
- Added `CatsPlatform.pasteTargetOwnsEvent` as the shared ownership policy for
  inputs, textareas, and contenteditable regions. The document terminal-paste
  fallback now applies only to non-editable targets, preserving terminal paste
  behavior outside form controls.
- Added Node policy coverage and a page-contract assertion. Extended the real
  installed-Edge harness to open Plugins, return an empty `plugin.list`, open
  Add, focus its input, and assert a bubbling paste is accepted with zero
  terminal paste messages.
- Verification passed: 10 Node policy tests, focused `go test ./cmd/catway`,
  and the complete installed-Edge browser regression.
- Added [bugfix_log.md](bugfix_log.md) as the requested append-only bug record;
  the full root cause and fix are B-001. Decision D-057 records paste ownership.

## 2026-08-10 — Canonical upstream sync through `5b92a5f`

- Added `https://github.com/rohanthewiz/cats.git` as the `upstream` remote and
  fetched canonical `main` at `5b92a5f`. Confirmed the fork’s `origin/main` was
  exactly 30 commits behind and that `wsl-ver` carried 10 unique WSL commits on
  the shared base `44b0397`.
- Merged `upstream/main` into the published `wsl-ver` history without rebasing,
  preserving both the 30 canonical commits and the existing Windows/WSL commit
  identities. The detailed commit inventory is in [sync_log.md](sync_log.md).
- Resolved five textual conflicts by combining behavior: retained the native
  launcher abstraction with upstream Cats Mux branding; added upstream CPU
  usage to the existing `WSL VM` resource scope; retained all WSL tests beside
  upstream CPU tests; combined Windows/WSL CI with upstream strict docs CI; and
  merged the sidebar/UI work into the platform-aware keyboard and clipboard
  layer.
- Added `sidebar-toggle` to `CatsPlatform.keyboardAction`, using `⌘B` on macOS
  and `Ctrl+Alt+B` on Windows while preserving AltGr and terminal ownership.
  Added policy assertions and generated the help label from the same descriptor.
- Kept WSL resource tooltips server-boundary-neutral and added the upstream CPU
  meter to the same policy, so Linux `/proc` readings are not presented as
  total Windows host resources.
- Added all WSL2/Windows records to the MkDocs navigation because upstream’s new
  strict `omitted_files` validation otherwise treats the existing documentation
  directory as a build error.
- Documented all six upstream `fix(...)` commits as B-002 through B-007 and
  recorded the synchronization policy and conflict decisions as D-058–D-062.
- Focused verification passed: 10/10 Node platform-policy tests, native Windows
  `cmd/catway`, and WSL `cmd/catway`, `internal/app`, `internal/browserproto`,
  and `internal/startdir`. Native Windows failures in the two Unix path suites
  were confirmed as their pre-existing target assumption and are recorded in
  the sync log.
- The complete untagged WSL gate found three stale WSL-only title assertions
  after the upstream Cats Mux rename. Updated those expected values, then passed
  `fmt-check`, vet, build, all Go tests, headless `catapp`, and the stamped
  Windows no-CGO compile.
- Final validation also passed the strict MkDocs build in a disposable Python
  container, installed-Edge regression, and native Windows `cmd/catapp`,
  `internal/desktopproto`, and `internal/wslclient` tests. The first native run
  emitted all packages as `ok` but hit the known transient executable cleanup
  lock; a clean-directory rerun exited successfully.
- Concluded the non-rewriting integration as merge commit `40a624f` with parents
  `b6f3e90` (the complete WSL implementation) and `5b92a5f` (canonical main).
  Ancestry verification shows no canonical upstream commits remain outstanding.

## 2026-08-10 — Incremental upstream sync through `322117a`

- Fetched canonical `upstream/main` and confirmed exactly one commit had landed
  since S-001: `322117a`, a sidebar mark proportion/weight refinement confined
  to `cmd/catway/web/index.html`. The fork’s `origin/main` was 31 commits behind
  canonical main; `wsl-ver` diverged by 12 local commits versus one incoming.
- Dry-run and real three-way merges both completed without conflicts. Accepted
  the upstream SVG/CSS patch intact because it is browser-rendered, contains no
  platform branching, and does not touch the WSL lifecycle, native launcher,
  clipboard, shortcuts, protocols, persistence, packaging, or CI seams.
- Verified that the merged page preserves the Windows/WSL platform layer:
  10/10 Node policy tests passed, focused `go test -count=1 ./cmd/catway`
  passed, and the installed-Edge browser regression passed.
- No bug was introduced, discovered, or repaired during this sync, so no new
  bugfix-log identifier was created. The incoming commit is an intentional
  visual style change rather than a correction with a bug/root-cause/fix chain.
- Decision D-063 records why platform-neutral visual commits may be merged
  intact after boundary and browser verification. Full topology, evidence, and
  repeatable procedure are recorded as S-002 in [sync_log.md](sync_log.md).
- Concluded the integration as merge commit `506f11e` with parents `55e9509`
  (the previously published WSL sync) and `322117a` (the new canonical commit).
  Both lines are ancestors and no canonical commits remain outstanding.

## 2026-08-10 — Rebuild, distribution, installation, and shortcut verification

- Built all four WSL/amd64 executables at `3a3f1e6` inside Ubuntu WSL2 and
  assembled the matching Ubuntu 24.04 payload. Built native Windows/amd64
  `Cats.exe` with the UCRT64/WebView2 toolchain and assembled the schema-2
  coupled distribution. No code-signing certificate was available, so this was
  correctly marked as an unsigned development package.
- The release-package validator passed outer and nested layout, checksums,
  source/compatibility identity, architecture/distro mapping, and signing-state
  checks. Copied the validated launcher to `bin/Cats.exe`.
- Installed the coupled package transactionally for `Ubuntu/bylaz` with
  `-DesktopShortcut -NoLaunch`; payload staging, helper health, atomic current
  link, launcher/backend smoke, command links, and commit all succeeded.
- The final shortcut readback then exposed B-008: both installed `.lnk` files
  existed but returned an empty target. A disposable control shortcut to the
  same executable round-tripped correctly, isolating the missing installer
  verification rather than a path or executable incompatibility.
- Added verified shortcut serialization with a bounded fresh-file retry and
  expanded the isolated real installer test to assert both shortcuts’ complete
  launch definitions. Decision D-064 records the transactional rule. The build
  must be repeated after this fix so final artifacts identify the corrected
  source commit.
- Independent verification after the first rebuilt install showed the same
  empty target. Cross-process probes proved standalone shortcut serialization
  persisted correctly and exposed that the same-process verifier had observed
  cached COM state. Moved shortcut publication after the installed smoke and
  changed integration verification to a separate PowerShell process; another
  source commit and rebuild are required before final delivery.
- Independent probes in the real Desktop folder then isolated the remaining
  behavior to managed shell policy: the unsigned CATS target was removed, while
  signed Notepad and Explorer targets retained their complete definitions.
  Unsigned development packages now publish an Explorer trampoline with the
  quoted, versioned CATS executable as its argument; signed packages continue to
  use a direct target. Integration expectations derive from the package signing
  claim and validate the full shortcut definition in a separate process.
- Committed the final shortcut fix as `037cd09`, rebuilt `Cats.exe`, `catway`,
  `cathost`, `catctl`, and `cats-wsl-host`, and produced
  `cats_037cd09_windows_amd64_wsl_ubuntu-24.04_amd64.zip` plus the standalone
  Ubuntu payload archive. The release-package validator passed and correctly
  classified the result as an unsigned development distribution.
- Installed release `037cd09` for `Ubuntu/bylaz`. Independent shortcut readback
  passed for both real shell locations, helper health reported `amd64` and
  `037cd09`, all four WSL programs were x86-64 ELF executables, and every
  installed payload entry passed `SHA256SUMS --check`. The requested Desktop
  shortcut remains; the two diagnostic probe shortcuts were removed.
