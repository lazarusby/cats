# Phase 0 baseline record

- Captured: 2026-08-09
- Repository commit: `44b0397827b973c031e67937c04ffb332e5c2206`
- Commit subject: `docs(session): drop ready-probe timeout fix`

The WSL2 documentation directory was already an untracked working-tree addition
when implementation began. It is intentionally part of this implementation
work; unrelated repository files were not modified.

## Reference host

| Item | Captured value | Initial status |
|---|---|---|
| Processor | AMD Ryzen AI 7 PRO 350, 8 cores/16 logical processors | Baseline |
| Windows | Windows 11 Pro, amd64, build 26200 | Baseline; no lower build claim yet |
| WSL package | Store WSL 2.7.11.0 | Baseline; no lower version claim yet |
| WSL kernel | 6.18.33.2-2 / `microsoft-standard-WSL2` | Baseline |
| Distribution | Ubuntu 24.04.3 LTS | Initial supported distribution |
| Linux architecture | x86_64 (`linux/amd64`) | Initial supported architecture |
| Windows architecture | X64 (`windows/amd64`) | Initial supported architecture |
| WebView2 | Evergreen Runtime 151.0.4129.72 | Installed baseline; native spike pending |
| Networking | Mirrored (`networkingMode=mirrored`) | Phase 1 probe passed |
| Lifecycle | Windows window owns foreground helper/backend | Accepted in D-009 |

The initial matrix is deliberately one pairing: Windows 11/amd64 plus Ubuntu
24.04.3 LTS/amd64. Other WSL distributions, ARM64, and lower Windows, WSL, or
WebView2 version floors require their own qualification evidence.

## Source/build baseline

| Command/artifact | Result on reference WSL host |
|---|---|
| `make check` | Pass: formatting, untagged vet/build/tests, ghostty vet, and ghostty race tests |
| `make binaries` | Pass: `catway`, `cathost`, and `catctl` are Linux x86-64 ELF executables |
| `make macapp` | Not run: requires a supported Darwin/Cocoa builder |
| `make macapp-client` | Not run: requires a supported Darwin/Cocoa builder |
| `go test ./internal/desktopproto` | Pass |
| Windows Chrome → WSL `catway` | Pass per section 4.5 owner verification |

The macOS commands are not treated as failures or as passes. Their output still
needs to be captured on a macOS builder before the Windows port refactors
`cmd/catapp` in Phase 2.

## Browser protocol probe archive

The current browser-protocol walkthrough implementation is
`cmd/catctl/probe.go` at the commit above. Its SHA-256 is:

```text
1eb94ac281d6ddc7128e1910c21f661bfb83c1003ca1a6311c98844b50c68c50
```

Keeping the commit plus content hash is the archive identity; copying the large
source file into documentation would create a stale duplicate. The human parity
walkthrough derived from it is [windows_parity_checklist.md](windows_parity_checklist.md).
A representative Windows visual recording remains to be captured after the
WebView2 spike can run.
