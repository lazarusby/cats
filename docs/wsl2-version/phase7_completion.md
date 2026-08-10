# Phase 7 completion record

- Date: 2026-08-09
- Implementation base: `0316d469ca67d77b2a0aa7a6e89379bb2bff7704`
- Supported Windows pair: Windows 11 amd64 + Store WSL2 + Ubuntu 24.04 LTS amd64

This record closes the implementation work in Phase 7 of
`detailed_implementation.md`. It describes the recovered Phase 6 workflow
skeleton, the completed CI/release transaction, local validation, and the
external repository/runner configuration required before cutting a public tag.

## Requirement completion matrix

| Phase 7 requirement | Implementation and evidence | Status |
|---|---|---|
| Preserve Ubuntu quick and Linux/macOS Ghostty jobs | `quick` retains format/vet/build/test; `ghostty` retains Ubuntu, Apple Silicon macOS, and Intel macOS vet/race coverage | Complete |
| Native Windows fast coverage | `windows-fast` checks Go formatting, parses all PowerShell, vets platform-neutral seams, runs launcher/protocol/WSL/installer tests, and builds the native UCRT64 CGO WebView2 executable | Complete |
| Shared protocol goldens | One pair of LF-only NDJSON fixtures is encoded and decoded by the same Go test on Windows and Linux | Complete |
| Linux helper integration | CI runs fake-daemon supervision tests, then the real helper, `cathost`, `catway`, terminal panes, and `catctl probe` | Complete |
| Windows integration | Native tests cover config, distro decoding, argument vectors, retry classification, clipboard UTF-16/bindings, navigation policy, helper protocol, and the native launcher build | Complete |
| Windows 11/WSL2 GUI integration | A labeled dedicated runner validates the signed zip, clean install, rollback, real WebView2, pane create/command/close, clipboard round trip, relaunch/restore, and uninstall | Complete; runner configuration required operationally |
| Same tag and full history | Every release build checks out full history; the exact semantic tag must resolve to `GITHUB_SHA`/`HEAD` | Complete |
| Qualified artifact names | Windows archive names version, Windows arch, WSL distro floor, and WSL arch; payload names version, distro floor, OS, and arch | Complete |
| Checksums and launcher/payload map | Inner package checksums, component digests, full source commit, top-level `release-manifest.json`, and release `SHA256SUMS` are generated and validated | Complete |
| No partial publication | Build jobs only upload workflow artifacts; the sole publish job requires all native builds, signatures, and dedicated GUI success, then promotes a complete draft | Complete |
| Signing, provenance, notes | Tag builds require Authenticode for the executable and all installer scripts, create GitHub attestations for every artifact, and generate GitHub release notes | Complete; credentials are external by design |

## CI topology

The ordinary CI workflow has four independent signals:

1. `quick` runs the existing untagged Ubuntu checks, the cross-OS lifecycle
   golden, and actionlint 1.7.12.
2. `windows-fast` uses Windows 2025, Go, and MSYS2 UCRT64 to exercise the native
   launcher without requiring WSL on the hosted runner.
3. `linux-wsl-host-integration` runs helper supervision against test daemons and
   then a real four-binary payload. The probe creates a second pane, runs a
   command in each pane, closes the second pane, and requests ordered shutdown.
4. `ghostty` retains the real tagged terminal path on Ubuntu, Apple Silicon
   macOS, and Intel macOS.

The exact lifecycle fixtures are:

- `internal/desktopproto/testdata/golden/helper.ndjson`;
- `internal/desktopproto/testdata/golden/launcher.ndjson`.

They pin JSON field order, values, direction, and the terminating LF. Additive
decode compatibility remains covered separately.

## Dedicated Windows 11/WSL2 transaction

Hosted Windows runners are used for fast tests and signing/assembly, but not as
WSL2 GUI evidence. The release workflow routes the assembled archive to a
self-hosted runner with these labels:

```text
self-hosted, Windows, X64, cats-wsl2-gui
```

The runner must have Windows 11 amd64, current Store WSL2, Evergreen WebView2,
the qualified Ubuntu 24.04 amd64 distribution/non-root test user, Go, Git, and
MSYS2 UCRT64. Configure these repository variables:

```text
CATS_WSL_DISTRO=<exact installed distribution name>
CATS_WSL_USER=<non-root Linux username>
```

`Test-CatsReleaseCandidate.ps1` first validates the archive and runs the Phase
6 isolated install/rollback suite. It then uses isolated Windows and WSL roots,
passes isolated XDG config/state through `WSLENV`, installs the package, and
stamps the native Go test executable with the package compatibility hash. The
test drives real `catway` through `catctl probe`, loads the real backend in a
hidden native WebView2 window, verifies native clipboard write/read, stops the
backend, starts a new helper/backend, confirms the surviving pane and captured
command output restored, and uninstalls. Exact temporary roots are removed in a
`finally` block.

## Release transaction

1. Validate a `vMAJOR.MINOR.PATCH...` tag against the full 40-character commit.
2. Build/test each existing native archive on its native runner and attest it.
3. Build the four-binary payload on Ubuntu 24.04 amd64 with the tag as
   `release_id`, short hash as `compatibility_version`, and full commit as
   `source_commit`; run the real helper integration; attest/upload the payload.
4. On native Windows, require the PFX secrets, import the certificate into the
   current-user store, run policy/protocol tests, build both halves into one
   archive, sign `Cats.exe` plus `Install-Cats.ps1`, `Uninstall-Cats.ps1`, and
   `CatsInstaller.psm1`, validate actual signatures and both inner manifests,
   attest/upload the archive, then remove the imported certificate even on
   failure.
5. Run the signed archive on the dedicated Windows 11/WSL2 GUI runner.
6. Download every successful workflow artifact into one directory. Generate a
   top-level manifest and checksum file and attest both.
7. Delete only an existing draft for the same tag, recreate it with the complete
   file set and generated notes, and only then change the draft to public. This
   prevents stale assets from a failed prior attempt surviving a rerun.

The publish job cannot run when any prerequisite job fails or remains queued.
If asset upload fails, the release remains a recoverable draft rather than a
public partial release.

## Identity, naming, and manifest schema

For a tag such as `v1.2.3`, the paired files are:

```text
cats-wsl-payload_v1.2.3_ubuntu-24.04_linux_amd64.tar.gz
cats_v1.2.3_windows_amd64_wsl_ubuntu-24.04_amd64.zip
```

Schema 2 separates three identities:

- `release_id`: the exact semantic tag, used for versioned install directories;
- `compatibility_version`: the short Git hash used by launcher/helper/backend
  runtime checks;
- `source_commit`: the full commit used by provenance and release tooling.

The Windows manifest records each component's file, OS, architecture, SHA-256,
and compatibility identity, so it directly maps the launcher to its payload.
The attached top-level release manifest adds every public artifact's name, size,
digest, source commit, Windows/WSL pair, distro floor, and provenance provider.
Installer schema 1 remains readable for already-built Phase 6 development
packages; new builds emit schema 2.

## Signing and provenance configuration

Configure these repository secrets only in GitHub's secret store:

```text
CATS_SIGNING_PFX_BASE64=<base64 PKCS#12/PFX bytes>
CATS_SIGNING_PFX_PASSWORD=<PFX password>
```

No private key, decoded PFX, password, or certificate is committed or uploaded.
The release build fails closed when secrets are absent or when any actual
Authenticode status is not `Valid`. Local development may use
`-AllowUnsignedDevelopment`; this switch is not passed anywhere in the release
workflow.

GitHub artifact attestations require `id-token: write` and
`attestations: write`; the workflow grants those scopes and uses
`actions/attest@v4` for each release subject. GitHub documents artifact
attestations at
<https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations>.
The dedicated-label behavior follows GitHub's self-hosted runner routing model:
<https://docs.github.com/en/actions/reference/runners/self-hosted-runners>.

## Local verification record

The following completed on the reference Windows 11/Ubuntu 24.04 WSL2 machine:

- PowerShell 5.1 parser: all Windows scripts/modules passed.
- Installer policy unit tests: passed.
- actionlint 1.7.12: both workflows passed after declaring the custom runner
  label; PyYAML independently parsed all job graphs.
- Protocol golden: native Windows passed in 6.943 seconds; Ubuntu passed in
  0.011 seconds.
- Real Linux integration: helper, daemons, two panes, commands, close, probe,
  and ordered stop passed in 6.1 seconds.
- Native focused Go packages passed. The first combined invocation reported a
  transient Windows deletion lock only after all three package results were
  `ok`; the protocol package then exited cleanly when rerun alone.
- Ubuntu `fmt-check`, vet, build, all untagged tests, headless catapp, stamped
  no-CGO Windows compile, and Ghostty vet passed in 55.5 seconds.
- The schema-2 package validator passed and its deliberate `-RequireSigned`
  invocation without a certificate failed closed before assembly.
- The rebuilt package passed the complete Phase 6 isolated install/smoke/
  uninstall/rollback/collision suite in 27.3 seconds.
- The first full GUI-candidate attempt exposed an unstamped `dev` test launcher
  and was correctly rejected by the `0316d46` helper. After the harness took its
  stamp from the package manifest, the entire candidate transaction passed in
  53 seconds. After final installer schema hardening changed the archive bytes,
  the definitive archive repeated the complete transaction in 55.5 seconds;
  native WebView2/WSL tests took 8.687 seconds. Failed and successful attempts
  removed their exact Windows/WSL roots.
- The top-level release manifest/checksum generator produced and verified the
  expected mapping, then its temporary test directory was removed.

The full Ghostty race command on the `/mnt/c` checkout repeated the known helper
test hang: a 60-second wrapper and then a 184-second serial wrapper reached no
assertion but left the helper timeout test/fake children alive. The five exact
PIDs were terminated, a later transient test process exited itself, and a final
process query was empty. This is not counted as a Phase 7 pass. Phase 6 already
recorded a complete serial race pass for the implementation base; hosted Linux
CI runs the race matrix on a native runner filesystem rather than `/mnt/c`.

The final local development artifacts are intentionally unsigned:

- `dist/cats-wsl-payload_0316d46_ubuntu-24.04_linux_amd64.tar.gz`
  - size: 27,337,121 bytes
  - SHA-256: `95ca60696bd3904f7dde9237e5e7fd8d7c637f2db3b5e6b169e7dd111a8bb482`
- `dist/cats_0316d46_windows_amd64_wsl_ubuntu-24.04_amd64.zip`
  - size: 33,218,777 bytes
  - SHA-256: `106ec627d23f2406eb4e5a83743be2eb8ec984022a2b1a21453bfdac06b9479a`

No code-signing certificate exists in the reference user's certificate store,
and no release tag was pushed, so this record does not claim that GitHub built,
attested, signed, or published a public release. It establishes that Phase 7's
implementation and fail-closed release gates are complete. Supplying repository
secrets/variables, registering the labeled runner, and pushing a release tag are
operator actions, not source-code gaps.

## Boundary after Phase 7

Phase 8 remains the complete product qualification checklist across functional,
resilience, security, and performance behavior. Phase 9 remains end-user/main
documentation and final release completion. Phase 7 does not mark either later
phase complete.
