# Windows 11/WSL2 parity checklist

- Baseline: Windows 11/amd64 + WSL2 Ubuntu 24.04.3 LTS/amd64
- Status values: Pending, Automated pass, Pass, Fail, or Not applicable

Each workflow names the boundary where it must execute. A Chrome pass proves
the backend/browser route but does not substitute for a WebView2/native bridge
pass where one is named.

| ID | Workflow | Execution boundary | Required observation | Status/evidence |
|---|---|---|---|---|
| P-01 | Start local application | Windows launcher → `wsl.exe` → `cats-wsl-host` | One window, matching versions, bounded readiness, no console window | Pass — assembled install/health/smoke and Phase 7 native window |
| P-02 | Open workspace | WebView2 page → `catway`; workspace and shell in WSL | Workspace renders with intended WSL start directory | Pass — real payload/page/PTY; dated visible capture pending |
| P-03 | Split pane | WebView2 page → `catway` → `cathost` | Horizontal and vertical splits preserve focus/layout | Pass — Phase 8 reaches eight panes with alternating split directions |
| P-04 | Type and resize | Windows keyboard → WebView2 → WebSocket → WSL PTY | Text/control keys arrive once; resize updates PTY/grid | Pass — real PTY/5,000 lines/160×48 resize plus key policy tests |
| P-05 | Selection/copy/paste | WebView2 selection + Windows launcher clipboard bridge | Unicode/emoji/CRLF round-trip; no terminal-control regression | Prior pass — Phase 7 native round-trip; current desktop clipboard service blocked |
| P-06 | Copy mode | WebView2 page → `catway`/pane scrollback | Navigate, select, copy, exit, and preserve pane input focus | Automated pass — installed-Edge selection/copy-mode regression |
| P-07 | Agent detection | WSL child processes + Linux `/proc` detector → `catway` → WebView2 | Agent identity/state follows start, attention, finish, and exit | Automated pass — manifest/detector/state suites; real CLI walkthrough pending |
| P-08 | Hook state/title | WSL agent hook → private WSL hook socket → `catway` → WebView2 | Hook updates correct pane without exposing socket to Windows | Automated pass — hook socket/title/state and private-runtime tests |
| P-09 | Worktree | WebView2 command → `catway` → WSL `git` and filesystem | Create/open/remove in WSL ext4; `/mnt/c` warning is non-blocking | Automated pass — real WSL backend plus worktree suite; visual flow pending |
| P-10 | Plugin | WebView2/`catctl` → `catway` → WSL child build/runtime | List/install/update/use/uninstall with hydrated Linux `PATH` | Automated pass — plugin/update/link/integration/PATH suites; visual flow pending |
| P-11 | Chat | WebView2 ACP panel → `catway` → WSL agent process | Send, stream, permission, cancel, reconnect | Automated pass — ACP suites; real credentialed agent pending |
| P-12 | Notification | WSL event → `catway` → WebView2/Windows notification | Appears unfocused; click focuses correct pane | Automated/prior pass — Edge focus/fallback and Phase 7 permission; Action Center visual pending |
| P-13 | External link | OSC 8/page `window.open` → Windows launcher policy | Opens default Windows browser; trusted view and bindings remain isolated | Pass — bounded Edge and native navigation-origin policy |
| P-14 | Save | WebView2 command/window close → `catway` → WSL XDG state | Final capture and layout/session state reach WSL storage | Pass — isolated XDG state and ordered stop |
| P-15 | Restart and restore | Windows launcher/helper lifecycle + WSL state | Close stops daemons; restart restores workspace/tabs/panes/history | Pass — Phase 7 and Phase 8 real cold restore |
| P-16 | Remote mode | Windows launcher → remote `catway` directly | Starts no WSL helper; login cookie and reconnect persist | Automated pass — saved/first-run remote and cookie/reconnect tests; live remote manual pending |
| P-17 | Shutdown resilience | Windows launcher ↔ helper control pipe | Requested stop and owner EOF cleanly exit; daemon/WSL failures show bounded error | Pass for stop/owner loss; terminate/shutdown are mandatory dedicated-runner gates |

For final parity sign-off, attach a dated screen recording or screenshots and
the relevant probe/test output to every Pass that depends on visible behavior.
The exact current automated evidence and remaining manual/environment gates are
recorded in [phase8_qualification.md](phase8_qualification.md).
