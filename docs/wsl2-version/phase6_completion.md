# Phase 6 completion record

- Date: 2026-08-09
- Source revision identity: `b5ba4df`
- Supported pair: Windows 11 amd64 + Store WSL2 + Ubuntu 24.04 LTS amd64

This document is the completion audit for Phase 6 of
`detailed_implementation.md`. It records what survived the hard reset, what was
finished afterward, how the cross-platform transaction behaves, and which
claims are deliberately deferred to later phases.

## Reset-recovery audit

| Phase 6 requirement | Recovered state | Completion action and evidence | Status |
|---|---|---|---|
| 6.1 separate build entry points | Make targets and packaging scripts existed | Verified `wsl-payload-dist`, `windows-launcher`, `windows-dist`, and installer test target; normal `binaries` remains Linux-only | Complete |
| Shared build identity | Payload/launcher health already used `b5ba4df`; distribution builder required the matching archive name | Rebuilt final coupled archive and passed installed identity/smoke checks | Complete |
| 6.2 native Linux payload | Four ELF binaries and a payload archive already existed | Inspected ELF/runtime metadata, checksums, contents, licenses, release metadata, and GLIBC 2.34 symbol floor on Ubuntu 24.04 | Complete |
| 6.3 native Windows launcher | Resource builder, icon generator, manifest, GUI-subsystem build and optional signing existed | Rebuilt with Go 1.26.5 and MSYS2 UCRT64; verified package/signing metadata | Complete |
| 6.4 chronological installer | PowerShell module and thin entry points existed; clean transaction test passed | Closed WSL1/distro and managed-link gaps; verified real install through smoke and exact uninstall | Complete |
| 6.5 upgrade/rollback | Versioned directories, WSL `current`, config merge, smoke rollback and mismatch checks existed | Added existing-install rollback qualification and verified prior launcher, config, WSL pointer, links, and backend after forced failure | Complete |
| Default data preservation | Uninstall plan preserved data but omitted command links | Added conditionally owned link removal and verified config preservation plus exact payload removal | Complete |
| Cross-Windows/WSL verification | Focused tests and assembled artifact existed; final integration status was not recorded | Ran final prerequisite gate, package tests, real isolated transaction, Linux/headless suite, and Ghostty race matrix | Complete |

## Produced components

### Linux payload

`scripts/package-wsl-payload.sh` packages these native Linux/amd64 siblings:

- `catway`
- `cathost`
- `catctl`
- `cats-wsl-host`

It also includes `config.example.yaml`, `NOTICE`, the libghostty-vt license,
`release.json`, and `SHA256SUMS`. Packaging refuses missing/non-executable or
non-ELF-amd64 inputs and records the highest referenced GLIBC symbol version.
The archive is reproducible from the commit timestamp and stable tar ordering.

The final development payload is:

- file: `dist/cats-wsl-payload_b5ba4df_linux_amd64.tar.gz`
- size: 27,336,276 bytes
- SHA-256: `0601b6c6864a0113627eb49426ebe3abee48d886664340278fd701946581f48f`
- metadata: Linux/amd64, compatibility `b5ba4df`, GLIBC >= 2.34

The four binaries were inspected as x86-64 ELF executables. `ldd` on `catway`
showed the Linux loader and libc only; the Windows-native UI design introduces
no Linux GUI library dependency.

### Windows launcher and coupled archive

`Build-WindowsLauncher.ps1` finds the supported Go, GCC/G++, and windres tools;
generates the icon; embeds version, icon, and manifest resources; stamps the Git
identity; and builds `Cats.exe` with cgo and `-H windowsgui`. The manifest uses
`asInvoker`, Windows 11 compatibility, PerMonitorV2 DPI awareness, and long-path
awareness. A supplied CurrentUser certificate thumbprint signs and timestamps
the executable.

`Build-WindowsDist.ps1` accepts only the matching native payload, builds the
launcher, copies the thin PowerShell install/uninstall entry points and module,
adds all relevant notices/licenses, optionally signs all PowerShell files,
generates checksums and release metadata, and creates the zip.

The final development distribution is:

- file: `dist/cats_b5ba4df_windows_wsl_amd64.zip`
- size: 33,215,907 bytes
- SHA-256: `07c829821d9ea11d700227a01e5979ef210a5cc2f18911b8d045f32f8f4ae717`
- `Cats.exe`: 15,287,024 bytes; SHA-256
  `c65103aef32088b716da3fda44bf27cae2fe09809b99d9b573f762ae484d6bc5`
- signing metadata: launcher `NotSigned`, installer `NotSigned`,
  `release_candidate_signed=false`

The unsigned status is intentional for this local development artifact and is
recorded in `release.json`. D-017 still requires a signed launcher and signed
PowerShell installer for a publishable release candidate.

## Install transaction

`Install-Cats.ps1` contains only parameter handling and error presentation; the
transaction lives in `CatsInstaller.psm1`.

1. Verify outer package checksums and parse the coupled release manifest.
2. Verify 64-bit Windows/PowerShell context, amd64, Windows build, Store WSL
   version, Evergreen WebView2 version, installed distributions, explicit or
   selected non-root user, WSL2 kernel, Ubuntu 24.04 identity, and x86-64 Linux.
3. Preflight optional command-link ownership. An absent link is eligible, an
   exact existing CATS link is preserved, and every other collision fails.
4. Copy the complete Windows package into a new per-user version stage under
   `%LOCALAPPDATA%\Programs\Cats\versions`.
5. Ask WSL to copy the Linux archive from its translated Windows path into a
   fresh stage under `<home>/.local/lib/cats`; no payload executable runs from
   `/mnt/c`.
6. Compare archive SHA-256 across the boundary, extract in the new stage, run
   inner `sha256sum --check`, set executable permissions, compare release
   metadata, and execute staged `cats-wsl-host --health-json`.
7. Move any same-version prior directories to transaction-private backups,
   move both stages to versioned final directories, and atomically replace the
   relative Linux `current` symlink with `mv -Tf`.
8. Create only missing managed command links to
   `<home>/.local/lib/cats/current/<name>`.
9. Atomically merge `%LOCALAPPDATA%\Cats\app.json`. Local WSL fields change,
   while an existing remote target and unknown future fields remain intact.
10. Create the Start-menu shortcut and the optional desktop shortcut to the
    versioned Windows executable.
11. Run that installed executable with `--install-smoke`. It performs helper
    health, daemon startup, Windows-side HTTP product/version verification, and
    ordered shutdown without opening a UI.
12. Commit only after smoke succeeds, remove transaction backups, append the
    installer log, and optionally launch the application normally.

The installer does not edit `/etc/wsl.conf`, enable systemd, install/convert a
distribution, or alter firewall rules.

## Upgrade consistency and rollback

The WSL config stores the stable `current` path. On each launcher start,
`Cats.exe` resolves it once to the version directory and uses that pinned target
for both health and launch. The helper, launcher, and HTTP endpoint all enforce
the same compatibility identity. This prevents a concurrent upgrade from
mixing versions between health and daemon startup.

Before commit, every failure restores:

- the previous Windows version directory;
- the previous WSL version directory and atomic `current` target;
- `app.json` byte-for-byte (or removes it if it did not previously exist);
- prior Start-menu and desktop shortcut definitions;
- any command links created by the failed attempt.

The final integration first installed a working baseline, then replaced
`Cats.exe` in a checksum-consistent test package with an unrelated executable so
the installed smoke had to fail. It proved the installer restored the original
launcher digest, exact config bytes, WSL `current`, all managed links, and a
working real backend.

## Uninstall and preservation contract

The default uninstall prints the exact plan, requires `REMOVE` unless `-Force`
is explicit, and removes only:

- `%LOCALAPPDATA%\Programs\Cats`;
- the CATS Start-menu and desktop shortcuts;
- the selected user's `~/.local/lib/cats` payload root;
- `~/.local/bin/{catway,cathost,catctl}` only when each is still a symlink to
  the exact managed CATS target.

It preserves by default:

- `%LOCALAPPDATA%\Cats` launcher configuration and logs;
- WSL CATS configuration, session state, themes, and plugins;
- projects and worktrees;
- agent installations, credentials, configuration, and integrations outside
  the CATS-owned config/state roots.

`-RemoveAllData` separately adds `%LOCALAPPDATA%\Cats`, the effective WSL
`$XDG_CONFIG_HOME/cats`, and `$XDG_STATE_HOME/cats` to the printed plan and
warns that sessions and configuration will be unrecoverable. It still does not
remove projects, worktrees, or external agent data.

## Verification record

Passed:

- PowerShell 5.1 parser and installer policy tests: checksum success/tamper,
  unsafe checksum paths, additive config merge, WSL1 rejection, unsupported OS
  rejection, uninstall plans, and unexpected payload-root refusal.
- Actual package prerequisite gate on the reference Windows build, WSL
  2.7.11.0 floor, and WebView2 151.0.4129.72 floor.
- `go test -count=1 ./cmd/catapp ./internal/desktopproto
  ./internal/wslclient` on native Windows.
- Native cgo/UCRT64 GUI launcher build and final package checksum verification.
- Real isolated Windows/Ubuntu WSL transaction through a Windows path containing
  spaces and `ü`: install, helper/daemon/HTTP smoke, managed links, uninstall,
  default config preservation, existing-install forced failure, full rollback,
  post-rollback real smoke, and preservation of a conflicting non-CATS command
  path before staging. Final runtime: 25.0 seconds; exact temporary Windows and
  WSL roots were removed.
- Ubuntu WSL `fmt-check`, untagged vet/build/tests, headless catapp, stamped
  Windows no-cgo compile, and Ghostty vet.
- Complete `-tags ghostty -race -p 1 ./...` matrix; all packages passed and the
  helper package completed in 8.028 seconds.

Bounded/replaced checks:

- The parallel Ghostty race matrix on `/mnt/c` again left one helper test
  process running past the outer bound. It was terminated and replaced with the
  complete passing serial matrix, matching the known reliable qualification
  mode for this checkout.
- A native-wide `go test ./...` was not used as evidence because existing
  Windows `catctl` tests expect Unix-only integration installation and the
  installed-Edge child exceeded its five-minute outer bound. Phase 6 does not
  alter either area; the focused native packages and complete Ubuntu matrix are
  the relevant evidence.

## Boundary after Phase 6

Phase 6 implements and qualifies build, local archive assembly, per-user
installation, in-place replacement, failed-upgrade rollback, and uninstall. It
does not claim that the unsigned local archive is publishable. Certificate
provisioning, signed release-candidate production, provenance, tag gating,
hosted/dedicated release lanes, release notes, and publication policy remain
Phase 7. Full product parity/resilience/security/performance sign-off remains
Phase 8, and end-user documentation/release completion remains Phase 9.
