# High-level WSL2/Windows implementation plan

- Status: proposed
- Target architecture: native Windows Go/WebView2 launcher; CATS backend in WSL2

The filename preserves the requested `implmentation` spelling. This plan is in
strict chronological order. A later phase starts only after the preceding exit
criteria pass.

## 0. Freeze scope and acceptance criteria

1. Define “feature parity” using the matrix in
   [Architecture_differences.md](Architecture_differences.md).
2. Confirm that panes run Linux shells and Linux-installed agents inside WSL;
   native Windows shells/agents are outside this version.
3. Select the initially supported WSL distributions and CPU architectures.
4. Select the first installer format and code-signing policy.
5. Record decisions in [decision_log.md](decision_log.md).

Exit: product scope, supported host matrix, and release acceptance checklist are
approved.

## 1. Prove the risky platform seams

Build disposable spikes before refactoring production code:

1. Build the pinned Go webview dependency as a Windows GUI and load a local
   page in WebView2.
2. Start a trivial WSL HTTP server with `wsl.exe`, connect to it from WebView2
   through `127.0.0.1`, and test both NAT and mirrored WSL networking.
3. Verify a long-running `wsl.exe --exec` child, bidirectional pipes, EOF on
   launcher death, graceful stop, and behavior during `wsl --shutdown`.
4. Prove Unicode clipboard read/write through a Go Win32 binding.
5. Verify background notification and click behavior in WebView2; decide
   whether a native notification bridge is needed.
6. Exercise Windows keyboard layouts and the terminal-critical Control chords.
7. Verify external-link/new-window handling and binding isolation in WebView2.

Exit: each seam has a recorded result and the architecture has no unknown that
would force a different framework.

## 2. Refactor `catapp` without changing macOS behavior

1. Move platform-neutral window, connect/error pages, remote mode, lifecycle
   state, and shared tests into common Go files.
2. Keep Cocoa menu, pasteboard, macOS config path, signals, and direct daemon
   supervision in Darwin-specific files.
3. Add narrow interfaces for platform menu, clipboard, app-data path,
   notification, and local-backend supervision.
4. Run the full existing macOS test/build/package path and compare behavior.

Exit: `make macapp` and `make macapp-client` remain unchanged from a user's
perspective, and Windows-specific code is not imported on Darwin.

## 3. Add the WSL host helper

1. Add a small Linux Go binary, `cats-wsl-host`, packaged beside `catway`,
   `cathost`, and `catctl` inside WSL.
2. Define and test its versioned JSON-lines lifecycle protocol.
3. Add Linux login-environment hydration, private runtime socket allocation,
   daemon startup/readiness, signal and pipe-EOF handling, ordered shutdown,
   and cleanup.
4. Add version/architecture/WSL health reporting for the Windows launcher and
   installer.
5. Keep terminal data on existing protocols; do not turn the helper into a new
   proxy.

Exit: the helper can be run from a Linux terminal, reports ready, serves a full
CATS session, and reliably cleans up after stop, EOF, daemon failure, and signal.

## 4. Implement the Windows launcher

1. Add Windows build files to `cmd/catapp` using WebView2.
2. Add WSL distribution discovery, saved target selection, payload health and
   version checks, safe `wsl.exe` argument construction, startup retry, and
   Windows-side readiness probing.
3. Add Win32 clipboard bindings under the existing JavaScript API.
4. Add Windows menus/accelerators, error UI, logs, and graceful cleanup.
5. Preserve remote-client mode; it must not start WSL.
6. Add native notification fallback if Phase 1 showed it is necessary.
7. Route external links to the Windows default browser and reject unsafe
   top-level navigation away from the configured CATS origin.

Exit: a developer build provides one-window startup, full local operation, and
clean shutdown on a Windows 11 test machine.

## 5. Make the web UI platform-aware

1. Inject a trusted platform descriptor from `catapp`.
2. Replace hard-coded Command-key labels and Mac-only shortcut branches with
   Mac and Windows mappings.
3. Preserve terminal Control sequences and browser paste behavior.
4. Prefer launcher bridges for clipboard and notifications when available;
   retain browser fallbacks for ordinary browsers.
5. Add browser-level regression tests for input, clipboard, menus, font size,
   focus, notifications, and reconnect.

Exit: the same embedded page behaves correctly in WKWebView, WebView2, and a
normal browser.

## 6. Build installation, upgrade, and removal

1. Add Windows icon/resource generation and a Windows GUI build target.
2. Add native Linux payload builds for supported WSL architectures.
3. Implement an installer that discovers/selects a WSL2 distribution, installs
   the payload in its Linux filesystem, performs a health check, writes launcher
   config, and creates Start-menu shortcuts.
4. Make upgrades atomic and reject launcher/backend version mismatches.
5. Preserve WSL config, session state, plugins, worktrees, and integrations on
   upgrade and normal uninstall; offer explicit data removal separately.
6. Test paths containing spaces, non-ASCII usernames, multiple distributions,
   non-default WSL users, and existing installations.

Exit: clean install, in-place upgrade, rollback after a failed health check,
and uninstall all behave deterministically.

## 7. Extend CI and release automation

1. Add Windows unit/build jobs for `catapp` and platform adapters.
2. Keep Linux ghostty race tests and add `cats-wsl-host` tests.
3. Produce coupled Windows/WSL artifacts from the same tag and build stamp.
4. Add a Windows 11 + WSL2 end-to-end lane, using a dedicated/self-hosted
   runner if hosted CI cannot provide nested WSL2 and GUI automation.
5. Generate checksums and provenance; add signing when the distribution policy
   is decided.

Exit: a tag cannot publish the Windows package unless both the Windows launcher
and matching WSL payload pass their required checks.

## 8. Run parity, resilience, security, and performance qualification

1. Execute the full feature-parity checklist on every supported Windows/WSL
   combination.
2. Exercise launcher crash, helper crash, daemon crash, Windows sleep/resume,
   `wsl --shutdown`, Windows reboot, network-mode changes, and stale payloads.
3. Confirm local mode is loopback-only and that no Unix control/hook socket is
   exported to Windows or the LAN.
4. Test large scrollback, many panes/agents, clipboard limits, long paths,
   Unicode, worktrees, `/mnt/c`, and WSL ext4 performance.
5. Verify Mac CI and packaging again to prevent a cross-platform refactor from
   regressing the existing product.

Exit: no open parity blocker; known platform limitations are documented and
accepted.

## 9. Document and release

1. Add Windows/WSL installation, configuration, troubleshooting, architecture,
   and update pages to the main documentation navigation.
2. Document that agents, credentials, hooks, plugins, projects, and `catctl`
   live inside the selected WSL distribution.
3. Publish a release candidate, collect diagnostics from clean machines, and
   fix release-blocking issues.
4. Publish the first supported Windows/WSL release with exact prerequisites,
   checksums, signing status, limitations, and rollback instructions.

Exit: a new user can install, launch, use, update, diagnose, and remove CATS
without repository knowledge.
