# Phase 8 qualification record

- Date: 2026-08-09
- Implementation base: `83593dff4cbe70ac0d757fa5f96db79eeaca77ac`
- Reference pair: Windows 11 build 26200/amd64 + WSL 2.7.11.0 + Ubuntu
  24.04.3 LTS/amd64 + WebView2 151.0.4129.72
- Networking during local tests: mirrored
- Overall status: qualification implementation complete; release sign-off pending

This record covers every item in Phase 8 of `detailed_implementation.md`. It
separates current automated evidence from checks that require a dedicated idle
distribution, a working interactive desktop session, external agent accounts,
different networking, or a reboot. A prior-phase result is named as such and is
not silently presented as a new Phase 8 pass.

## What Phase 8 added

- The signed release-candidate gate now runs an eight-pane, 5,000-line,
  two-client workflow through the installed Windows launcher and real WSL
  payload. It covers pane/tab/workspace create, focus, close, reorder, rename,
  zoom, resize, and workspace lock/unlock, then stops and cold-restores state.
- The test inspects the live WSL listener table and fails unless the selected
  `catway` port is bound only to `127.0.0.1`.
- `catctl probe` gained missing tab/workspace move, close, rename, and lock
  operations so the assembled system can exercise the full session vocabulary.
- Release qualification measures startup, the functional/many-pane workflow,
  restore, and equal 64 MiB sequential I/O on WSL ext4 and `/mnt/c`.
- Owner-loss and distribution-wide disruption have separate controls. The
  dedicated release runner requires owner kill, `wsl --terminate`, and
  `wsl --shutdown`; a shared development distribution can run owner loss alone.
- The Windows package has an exact file/directory allow-list. Payload tar
  entries are type/name allow-listed before extraction, rejecting symlinks,
  traversal, duplicates, additions, and omissions. Extraction does not restore
  archive owners or permissions.
- WSL memory/disk readings are labeled `WSL VM`; native Linux/macOS retain the
  `Host` label. Tooltips no longer imply that WSL values are total Windows
  resources.
- Installed-Edge CDP calls are bounded. Its test-only `--no-sandbox` waiver is
  limited to a loopback page and disposable profile and is never used by
  `Cats.exe` or WebView2.

## Functional qualification

| Area | Evidence | Status |
|---|---|---|
| Local mode | Isolated schema-2 install, real helper/daemons, live browser protocol, eight panes, two clients, restore, uninstall | Pass |
| Remote mode | Common launcher tests prove no WSL helper starts and saved/first-run remote targets navigate in the same window; remote TLS/password suites pass | Automated pass; live remote walkthrough pending |
| Workspace/tab/pane lifecycle | Real candidate exercises create/focus/close/reorder/rename/lock, horizontal/vertical split, resize, zoom, eight-pane peak; full command/layout unit suite passes | Pass |
| Terminal text/colors/Unicode/mouse/links/scrollback/copy mode | Ghostty suite passes; 5,000-line real PTY flow passes; Edge regression covers selection, copy mode, mouse link policy, OSC 8, OSC52, IME and AltGr policy | Automated pass; dated visual walkthrough pending |
| Win32 clipboard and OSC52 | UTF-16 validation/contention tests and Edge OSC52 route pass; Phase 7 native round-trip passed | Prior pass; current desktop session blocked |
| Windows keyboard/menu | Accelerator/menu tests, IME/dead-key/AltGr policy, native menu presence, and page shortcut ownership are automated | Automated pass; non-US layouts/RDP remain manual |
| Agents/integrations/resume | All shipped detection manifests/installers and resume argv/state suites pass in Ubuntu; helper login-PATH hydration passes | Automated pass; real external CLI/account matrix pending |
| Plugins/worktrees/path picker/completions/`catctl` | Complete Ghostty-tagged package tests pass, including install/update/link and command vocabulary | Automated pass; visual plugin workflow pending |
| ACP chat/credentials in WSL | ACP protocol/client/manager, streaming, permission, cancel, restart and credential non-disclosure tests pass | Automated pass; real credentialed agent pending |
| Themes/settings/live reload | Theme/config persistence tests pass; real probe sends `server.reload_config` in existing integration coverage | Pass |
| Notifications | Browser permission/fallback/focus behavior passes in Edge; native permission gate passed in Phase 7 | Automated/prior pass; visible Action Center click pending |
| Persistence and restore | Real candidate preserves one pane/output after stop and cold helper/backend relaunch; unit suites cover corrupt state and live reconnect | Pass |

The local Phase 8 candidate deliberately used
`-SkipNativeUIForDevelopment` after the desktop clipboard subsystem itself
failed `Set-Clipboard`/`Get-Clipboard`. An earlier unskipped candidate therefore
failed both native clipboard assertions, and a later isolated WebView process
also faulted during initialization. Phase 7 recorded a complete native
WebView2/clipboard pass on this same software baseline. The release workflow
does not pass the skip switch, so a current native result is still mandatory
before publication.

## Resilience qualification

| Scenario | Evidence | Status |
|---|---|---|
| Close/Quit and ordered cleanup | Helper stop record, page shutdown, `catway` then `cathost`, exact runtime removal | Pass |
| Windows owner/Task Manager equivalent | Breakaway owner subprocess is killed; stdin EOF stops payload and removes launch runtime | Pass (1.64 s local) |
| Helper/daemon crash | Fake-process integration covers either child exit, partial startup, malformed protocol, readiness timeout and unresponsive kill escalation | Pass |
| WSL cold start | Multiple local launches and post-stop cold restore pass | Pass |
| `wsl --terminate` / `wsl --shutdown` | Source tests and mandatory dedicated-runner invocation are wired | Release gate; not run locally because two unrelated Bash sessions were active |
| Windows reboot and sleep/resume | No safe in-process substitute is treated as evidence | Manual pending |
| Port conflicts/forwarding | Windows/WSL conflict probes and bounded retry classification passed under mirrored networking | Pass for mirrored; NAT pending |
| VPN/firewall interference | Requires selected products/policies outside the repository | Manual pending |
| Missing/renamed/reset distro, default user, corrupt config | Fail-closed target/config/health unit tests and repair paths pass | Automated pass; reset walkthrough pending |
| Missing/mismatched/partial payload and rollback | Exact layout/checksum/version/architecture/distro checks plus forced-smoke rollback and link-collision suite pass | Pass |
| Multiple instances/clients | Two simultaneous ordinary browser-protocol clients pass with one sizer | Clients pass; multiple launcher windows pending |

Distribution terminate/shutdown is intentionally still required in the release
workflow with `-IncludeDistributionShutdownQualification`. It was not invoked on
the shared reference distro after process inspection showed two long-lived
interactive Bash sessions.

## Security qualification

| Requirement | Evidence | Status |
|---|---|---|
| Local listener is loopback-only | Live `ss -ltnH` check matches candidate port only at `127.0.0.1`; launcher/helper also construct loopback addresses only | Pass |
| No passwords/tokens in launcher config/logs | Launcher config schema contains no secret field; redaction and credential error tests pass; usage code never broadcasts credentials | Pass |
| Helper protocol is bounded and non-commanding | 16 KiB limit, malformed/truncated/unknown tests, direction-specific records, only `stop` accepted from launcher | Pass |
| Runtime/socket ownership | user-owned mode-0700 XDG/temp allocation and exact non-recursive cleanup tests pass | Pass |
| Privileged WebView origin gate | trusted-origin navigation/permission policy and native Phase 7 cross-origin tests; external/file/javascript navigation denied | Pass |
| OSC 8/external link isolation | Edge allows only HTTP(S)/mailto and keeps trusted page in place; native policy opens external URLs outside the privileged view | Pass |
| Installer quoting/traversal/symlinks/uninstall | direct WSL argv, exact ZIP/package allow-list, pre-extraction tar type/name allow-list, reparse rejection, safe links, exact uninstall plan | Pass |
| WebSocket origin and remote TLS/password | same-origin/allow-list, opaque-origin denial, HMAC cookies, TLS and password tests pass | Pass |

The first rebuilt Windows archive exposed a pre-existing relative-output bug:
checksum paths contained part of the staging directory when
`-OutputDirectory dist` was relative. The stricter validator rejected it. The
builder now canonicalizes its output directory, and the rebuilt archive passed
both package validation and the real install transaction.

## Performance evidence

The local assembled-candidate output was:

| Measurement | Result | Budget/interpretation |
|---|---:|---|
| Backend startup | 1,126 ms | Pass; 20,000 ms budget |
| Eight-pane + 5,000-line + two-client workflow | 2,140 ms | Pass; 30,000 ms budget |
| Cold restore and content assertion | 1,728 ms | Pass; 20,000 ms budget |
| WSL ext4 64 MiB sequential write/read | 143 / 106 ms | Reference filesystem baseline |
| `/mnt/c` 64 MiB sequential write/read | 598 / 475 ms | 4.18× / 4.48× slower on this host |

These are end-to-end regression measurements, not low-level keystroke-to-frame
instrumentation. The functional duration includes process/probe overhead and
the scroll test asserts that line 5,000 reaches a frame. Memory/disk correctness
is covered by live reads and the new `WSL VM` scope label. A directly comparable
macOS GUI baseline, frame-timing capture, and sleep/resume resource sample were
not available and remain explicit sign-off work.

## Validation record

- PowerShell 5.1 parsed every Windows script/module and the installer policy
  suite passed its checksum, layout, traversal, symlink, config, and uninstall
  cases.
- The rebuilt unsigned package validator passed. The complete isolated Phase 6
  install/smoke/uninstall/rollback/link-collision transaction passed again.
- The extended assembled-candidate qualification passed in 63.1 seconds with
  the metrics above, owner-loss cleanup, and exact uninstall/temporary cleanup.
- Ubuntu `fmt-check`, vet, untagged build/all tests, headless catapp, Windows
  no-CGO compile, and Ghostty vet passed in 66.1 seconds.
- The complete non-race Ghostty-tagged suite passed in 21.7 seconds.
- Focused race tests passed for `cmd/catctl`, `internal/browserproto`, and
  Ghostty-tagged `cmd/catway`.
- Nine Node platform-policy cases passed. The first installed-Edge attempt hit
  the old unbounded CDP behavior and its exact new temporary profile was removed
  (not recoverable); after bounding CDP and limiting `--no-sandbox` to the
  isolated test, the full Edge regression passed in 3.5 seconds.
- actionlint 1.7.12 passed both workflows after the Phase 8 job changes.
- No CATS payload processes or Phase 8 install/performance roots remained after
  the passing transaction.

The local development artifacts are intentionally unsigned and were assembled
from the Phase 8 worktree while carrying the base commit's development stamp;
they are test evidence, not provenance-correct release files:

- `dist/cats-wsl-payload_83593df_ubuntu-24.04_linux_amd64.tar.gz`
  - size: 27,343,251 bytes
  - SHA-256: `0897e2eee31e3985a6a2cab1ce7455e0cc3b23ece86f02caeb5f5c542640dc00`
- `dist/cats_83593df_windows_amd64_wsl_ubuntu-24.04_amd64.zip`
  - size: 33,224,047 bytes
  - SHA-256: `f564a91bfe8d2d53a18b3292a6ca27b37bde12e46ed6e7dd2a18210e4b5eb807`

## Remaining sign-off gates

Phase 8 source/test implementation is complete, but the product qualification
checklist is not honestly all-Pass until these are recorded:

1. Run the signed archive on the dedicated idle runner without the development
   skip, including native WebView2/clipboard, `wsl --terminate`, and
   `wsl --shutdown`.
2. Perform Windows reboot and sleep/resume, NAT, selected VPN/firewall, distro
   reset/default-user change, and multiple-launcher walkthroughs.
3. Run real supported agent/plugin/ACP credential flows in isolated WSL homes.
4. Capture the dated visual parity walkthrough, including non-US keyboard/AltGr/
   IME, RDP clipboard, native Action Center click-to-focus, and external browser
   links.
5. Record comparable macOS/Linux GUI latency/memory/many-pane data.

The release workflow fail-closes on item 1. Items 2–5 are operator/manual
qualification evidence and cannot be manufactured by repository unit tests.
