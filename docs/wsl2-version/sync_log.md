# WSL2/Windows upstream synchronization log

This is an append-only running record of canonical upstream synchronization:
the starting topology, incoming commits, integration strategy, conflicts,
compatibility work, verification, and resulting branch state. Architectural
choices are also recorded in [decision_log.md](decision_log.md), and code work
is summarized in [implementation_log.md](implementation_log.md).

## S-001 — Synchronize canonical `main` through `5b92a5f`

- Date: 2026-08-10
- Local branch: `wsl-ver`
- Fork remote: `origin` → `https://github.com/lazarusby/cats.git`
- Canonical remote added: `upstream` → `https://github.com/rohanthewiz/cats.git`
- Shared base: `44b0397`
- Starting WSL tip: `b6f3e90`
- Incoming canonical tip: `5b92a5f`
- Starting divergence: 10 commits unique to `wsl-ver`; 30 commits unique to
  `upstream/main`; zero commits unique to the fork’s `origin/main` relative to
  canonical main.

### Strategy

- Fetch canonical `main` through the dedicated `upstream` remote.
- Merge it into `wsl-ver` with a non-fast-forward merge rather than rewriting
  the already-published WSL history (D-058).
- Dry-run the three-way merge first. Resolve each overlap by composing behavior
  at the smallest platform seam, then test on native Windows and inside WSL
  (D-059).

### Incoming 30-commit inventory

1. `e6bf954` — offer to create a missing workspace start path.
2. `7571640` — correct sidebar heading hierarchy (B-002).
3. `ef57af9` — fold usage groups like pane groups.
4. `054d098` — repair admonitions and document push/orphan recovery.
5. `219b52f` — MkDocs documentation review.
6. `88332b5` — strict docs/link CI.
7. `f827d06` — document docs-CI verification status.
8. `79f4473` — preserve click meaning across list rebuilds (B-003).
9. `832d53d` — align fold controls at section-heading edges.
10. `950fec0` — folded usage summaries and host CPU.
11. `6c5e8a4` — document usage headlines/CPU work.
12. `c402af5` — fold an ungrouped workspaces list to its count.
13. `1fce293` — add the cat mark beside the wordmark.
14. `6eb8410` — enlarge and refine the mark.
15. `5f25cc4` — adopt the Cats Mux product name.
16. `e1f5462` — repair the CSS comment that swallowed logo sizing (B-004).
17. `df7c142` — reduce build-hash visual emphasis.
18. `cbdb092` — pace usage polling by reader attention and add fold horizon.
19. `25415a2` — turn near-expiry countdowns amber.
20. `a1e1f3b` — close the stale cats-mobile documentation item.
21. `26001fa` — fold/reveal the sidebar and auto-reveal for agent attention.
22. `92aa955` — document sidebar folding and mobile regeneration.
23. `c311f6f` — replace the icon with a traced reference.
24. `f452d30` — add the MIT license.
25. `c6a0129` — merge upstream pull request #1 for licensing.
26. `02270e6` — pin grid columns when the sidebar is folded (B-005).
27. `5831af8` — update the edge-painted application icon.
28. `ac971d7` — preserve the brand row in short windows (B-006).
29. `b1ed11a` — provisionally refit pane geometry during resize (B-007).
30. `5b92a5f` — add a fold button at the logo row’s right edge.

### Conflict and compatibility work

| File | Conflict | Combined resolution |
| --- | --- | --- |
| `.github/workflows/ci.yml` | Upstream added strict docs CI while WSL had expanded Windows/WSL jobs. | Retained every WSL job and added the docs job; rewrote only the overview comment. |
| `cmd/catapp/main.go` | Branding used the old direct `webview` API while WSL uses a platform-neutral window seam. | Adopted `Cats Mux` titles and kept `desktopWindow`, `sizeHintFixed`, and `initPlatformDescriptor`. |
| `cmd/catway/usage.go` | Upstream added a CPU sampler to a group that WSL had renamed by resource boundary. | Accepted the CPU sampler/signature and kept `hostResourceScope` for `WSL VM`. |
| `cmd/catway/hostmem_test.go` | WSL scope tests and upstream CPU group tests occupied the same insertion point. | Kept both complete test functions. |
| `cmd/catway/web/index.html` | CPU wording, direct shortcut code, and platform-aware keyboard/help code overlapped. | Added a neutral CPU tooltip and routed sidebar folding through `CatsPlatform`; retained editable paste ownership. |

Additional non-conflicting integration:

- Added `sidebar-toggle`, platform labels, and macOS/Windows policy assertions
  to `platform.js` and its Node suite (D-060).
- Applied `WSL VM` semantics to upstream CPU as well as memory/disk (D-061).
- Added every WSL2/Windows document and running log to MkDocs navigation so the
  new strict omitted-file validation can remain enabled (D-062).
- Added the entire previously local WSL2 documentation corpus to the branch so
  a clone contains the design and qualification evidence referenced by the logs.

### Verification

- `git merge-tree --write-tree wsl-ver upstream/main` predicted exactly five
  content conflicts; the real merge produced the same five.
- Conflict markers: none after resolution.
- `git diff --cached --check`: passed.
- `node --test cmd/catway/web/platform.test.cjs`: 10/10 passed.
- Native Windows `go test -count=1 ./cmd/catway`: passed.
- Native Windows `internal/app` and `internal/startdir` tests were not counted:
  imported cases use Unix `/`, `~`, and home semantics and are intentionally
  absent from the WSL branch’s `windows-fast` job.
- WSL `go test -count=1 ./cmd/catway ./internal/app ./internal/browserproto
  ./internal/startdir`: passed all four packages.
- The first complete WSL run incorrectly put `GOTMPDIR` on `/mnt/c`; DrvFS
  cannot provide the Unix executable-mode and socket semantics required by the
  helper/integration tests. Repeating with Go’s normal ext4 `/tmp` made every Go
  package pass and exposed only three stale WSL-only `cats — ...` title
  assertions after upstream’s Cats Mux rename.
- Updated those three expected titles, then passed the complete untagged WSL
  gate: `fmt-check`, `go vet ./...`, `go build ./...`, `go test ./...`, headless
  `cmd/catapp`, and the stamped Windows/amd64 no-CGO test compile.
- `python -m mkdocs build --strict`: passed in a disposable read-only-mounted
  `python:3.12-slim` container after installing `requirements-docs.txt`; no
  host or WSL Python packages were changed.
- `node scripts/test-webui-edge.mjs`: passed the installed-Edge browser
  regression, including Plugins → Add editable paste behavior.
- Native Windows `go test -count=1 ./cmd/catapp ./internal/desktopproto
  ./internal/wslclient`: all packages passed. The first run reported `ok` for
  all three but encountered the known transient `.test.exe` cleanup lock; a
  fresh-temp rerun exited 0.
- Final `git diff --cached --check`: passed after normalizing Markdown metadata
  headers from hard-break spaces to list items.

### Procedure for the next sync

1. Ensure `wsl-ver` has no unrelated working-tree changes.
2. Run `git fetch upstream main` and record the new upstream object ID.
3. Compare `git rev-list --left-right --count wsl-ver...upstream/main` and list
   `HEAD..upstream/main` before merging.
4. Dry-run with `git merge-tree --write-tree wsl-ver upstream/main`.
5. Merge without rebasing; resolve overlaps by behavior and platform boundary.
6. Run Node platform policy, native Windows seams, WSL quick/full suites, docs,
   and installed-Edge coverage appropriate to the changed files.
7. Append a new `S-NNN` entry and update the implementation, bugfix, and
   decision logs before committing and pushing.
