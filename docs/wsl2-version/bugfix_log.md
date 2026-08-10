# WSL2/Windows bugfix log

This is an append-only running record of reported bugs, their root causes,
fixes, and regression evidence. Implementation work is also summarized in
[implementation_log.md](implementation_log.md); behavioral and architectural
choices belong in [decision_log.md](decision_log.md).

## B-001 — Plugin Add dialog paste was sent to the terminal

- Date: 2026-08-10
- Bug: After opening **Plugins**, choosing **Add**, and pressing `Ctrl+V` in
  the source field, the clipboard text was pasted into the terminal behind the
  dialog instead of the focused field.
- Root cause: Modal keyboard routing correctly returned browser control of
  `Ctrl+V`, but the resulting bubbling `paste` event reached the document-level
  terminal paste fallback. That fallback exempted only the chat composer, so it
  cancelled the input's native paste and sent a terminal `paste` WebSocket
  message.
- Fix: Added one platform policy that recognizes `input`, `textarea`, and
  contenteditable event targets as browser-owned. The document paste fallback
  now returns immediately for those targets and continues forwarding paste only
  from non-editable terminal surfaces. This covers the plugin prompt and all
  other current/future text dialogs.
- Regression coverage: Added dependency-free policy cases for inputs,
  textareas, contenteditable descendants, terminal canvas targets, and missing
  targets. Extended the installed-Edge browser regression to open the Plugins
  Add prompt and prove its bubbling paste remains uncancelled and emits no
  terminal paste message.
- Verification: `node --test cmd/catway/web/platform.test.cjs` passed 10/10;
  `go test ./cmd/catway` passed; `node scripts/test-webui-edge.mjs` passed.

## B-002 — Sidebar headings read like footnotes to their rows

- Date incorporated: 2026-08-10
- Upstream commit: `7571640`
- Bug: Sidebar section and group headings were visually quieter than the rows
  they named, reversing the intended information hierarchy.
- Root cause: Section titles and group subtitles were both 10px while their
  rows were 12px, so typography made the labels trail their content.
- Fix: Raised section titles to 12px, set group subtitles and rows to 11px,
  and pinned the age stamp and caret sizes that should not inherit the change.
- Regression evidence: Incorporated with the upstream web UI and covered by
  the installed-Edge page load/regression harness.

## B-003 — Busy sidebar rebuilds swallowed clicks

- Date incorporated: 2026-08-10
- Upstream commit: `79f4473`
- Bug: Workspace, tab, pane, and agent rows intermittently ignored clicks while
  agent state was changing.
- Root cause: Those lists are rebuilt wholesale on frequent rollups. When a row
  was replaced between `mousedown` and `mouseup`, the browser emitted `click`
  on the nearest surviving ancestor, where no row handler existed.
- Fix: Capture row identity at press time and resolve activation from a
  window-level `mouseup`; use one drag threshold so a gesture becomes exactly
  one activation or drag even if its original DOM node is rebuilt.
- Regression evidence: Upstream’s session record and browser interaction
  coverage were incorporated unchanged.

## B-004 — A malformed CSS comment removed the logo sizing rule

- Date incorporated: 2026-08-10
- Upstream commit: `e1f5462`
- Bug: The sidebar cat mark rendered at an unbounded default SVG size and was
  clipped to its ears.
- Root cause: Explanatory prose was appended after a CSS comment terminator.
  CSS recovery consumed the following `#brand .mark` rule, including its width,
  height, and baseline adjustment.
- Fix: Moved the prose inside one valid comment so the mark rule parses.
- Regression evidence: The corrected stylesheet is exercised by the installed-
  Edge page regression.

## B-005 — Folding the sidebar collapsed the terminal grid

- Date incorporated: 2026-08-10
- Upstream commit: `02270e6`
- Bug: Hiding the sidebar moved the terminal area into the five-pixel gutter
  and left no useful reveal target.
- Root cause: CSS Grid auto-placement ignored the `display:none` sidebar and
  shifted every later child one column left.
- Fix: Pin each application child to an explicit grid column and give the
  folded gutter a visible, full-height reveal affordance.
- Regression evidence: Upstream’s fold/reveal browser coverage was incorporated;
  the installed-Edge harness verifies the merged page loads and operates.

## B-006 — Short windows clipped the sidebar brand and logo

- Date incorporated: 2026-08-10
- Upstream commit: `ac971d7`
- Bug: On a short viewport, the brand row collapsed to a sliver instead of the
  sidebar scrolling.
- Root cause: `overflow:hidden` removed the flex item’s automatic minimum
  height, making the brand the only child allowed to shrink under pressure.
- Fix: Set `flex: 0 0 auto` on the brand row so its content keeps its height and
  overflow is handled by the sidebar scroller.
- Regression evidence: Upstream reproduced the result in headless Chrome at a
  constrained viewport; the resulting CSS is incorporated unchanged.

## B-007 — Panes retained stale geometry during sidebar resizing

- Date incorporated: 2026-08-10
- Upstream commit: `b1ed11a`
- Bug: Folding, revealing, or resizing the sidebar briefly left unused terminal
  space or clipped panes until the server returned a new layout.
- Root cause: Pane rectangles remained scaled for the previous client grid
  during the browser/server resize round trip.
- Fix: Refit the last server-authored layout onto the newly measured grid as a
  provisional frame, preserving fixed chrome insets and replacing it with the
  next authoritative server layout without compounding estimates.
- Regression evidence: Upstream measured complete pane-area coverage across
  fold/reveal and both split orientations; the merged WSL page keeps that logic.

## B-008 — Installed Windows shortcuts could have no resolvable target

- Date: 2026-08-10
- Bug: The production installer reported success and created `Cats.lnk` files,
  but Windows shortcut APIs returned an empty target for both the Start-menu and
  requested desktop shortcut, so the desktop launch acceptance check failed.
- Root cause: `Set-CatsShortcut` treated `WScript.Shell.Save()` as proof of a
  valid link and never read the saved file back. The installer integration test
  did not request a desktop shortcut or inspect either shortcut’s launch
  properties, allowing a structurally present but unresolved `.lnk` through.
- Fix: Read back and compare target, arguments, working directory, and icon
  after every save. If the first serialization is invalid, remove the exact link
  and retry once from a fresh file; if the retry is invalid, fail the install so
  its existing rollback restores the previous state. Treat a previously
  unresolved link as absent during rollback rather than trying to recreate it.
- Regression coverage: The isolated real Windows/WSL installer transaction now
  requests `-DesktopShortcut` and verifies both Start-menu and desktop links
  point to the installed versioned `Cats.exe` with the exact launch properties.
