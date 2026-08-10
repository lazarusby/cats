# Windows 11/WSL2 installation and verification

- Status: implementation prerequisite specification
- Last verified: 2026-08-09
- Reference development host: Windows 11, WSL2, Ubuntu 24.04.3 LTS, amd64

> **Important:** the Windows launcher and `cats-wsl-host` described in this
> directory are planned but are not yet implemented. The instructions below can
> prepare and verify a machine now, and can build/run the existing Linux backend
> in WSL. Commands labeled **after implementation** will become usable when the
> Windows port is built.

The initial implementation matrix is Ubuntu 24.04.3 LTS on amd64 paired with
Windows 11 on amd64 (D-013). Other distributions, architectures, and lower
Windows/WSL version floors require separate qualification before they become a
support promise.

## 1. Package responsibility summary

| Component | End user | Developer | Installed on |
|---|---:|---:|---|
| Windows 11 updates | Required | Required | Windows |
| Current Store-delivered WSL | Required | Required | Windows |
| Supported WSL2 Linux distribution | Required | Required | Windows/WSL |
| Evergreen WebView2 Runtime | Required | Required | Windows |
| Windows PowerShell 5.1+ | Required for installer | Required | Windows |
| CATS `Cats.exe` | Required after implementation | Required | Windows |
| CATS Linux payload (`catway`, `cathost`, `catctl`, `cats-wsl-host`) | Required | Required | WSL Linux filesystem |
| `ca-certificates`, `git`, glibc | Required for full runtime features | Required | WSL |
| Go 1.26 or newer | Needed for source builds and Go-based plugin builds | Required | WSL; also Windows for launcher development |
| GCC/C++/make/pkg-config/curl/xz build tools | No, with prebuilt payload | Required | WSL |
| Zig 0.15.2 | No system install | Downloaded automatically by `make vt` into `.tools/` in WSL |
| MSYS2 UCRT64 MinGW-w64 toolchain | No | Required for native Windows launcher build | Windows |
| Git for Windows | No | Recommended for Windows-side checkout/build | Windows |
| Windows SDK/signing tools | No | Optional until installer/signing work | Windows |
| Agent CLIs and their runtimes | Only for agents the user chooses | As needed | WSL, not Windows |

## 2. End-user Windows installation

Run the commands in this section from **64-bit PowerShell**. Only the initial
WSL installation needs an Administrator window.

### 2.1 Verify Windows 11 and architecture

```powershell
Get-ComputerInfo |
  Select-Object WindowsProductName, WindowsVersion, OsBuildNumber, OsArchitecture

[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
```

Required result:

- product is Windows 11;
- Windows Update has applied current servicing/security updates;
- architecture matches the future CATS package (`X64` for the initial release;
  ARM64 requires a later matrix decision and qualified hardware).

If hardware virtualization is disabled, enable Intel VT-x/AMD-V in firmware.
`wsl --install` normally enables the Windows optional components required by
WSL2; do not manually toggle them unless the WSL installer reports a problem.

### 2.2 Install or update WSL

On a machine without WSL, open **PowerShell as Administrator**:

```powershell
wsl --list --online
wsl --install -d <ExactDistributionNameFromTheList>
```

For the reference environment, select the Ubuntu 24.04 LTS entry shown by
`wsl --list --online` (normally `wsl --install -d Ubuntu-24.04`). Use the name
actually printed on the machine rather than assuming the example is unchanged.
Restart Windows if requested, launch the distribution, and create its normal
non-root Linux user and password.

On a machine that already has WSL:

```powershell
wsl --update
wsl --version
wsl --status
wsl --list --verbose
```

The chosen distribution must show version `2`. Convert it if necessary:

```powershell
wsl --set-version <DistributionName> 2
wsl --list --verbose
```

Do not use a WSL1 distribution. The official installation and verification
commands are documented in [Install WSL](https://learn.microsoft.com/windows/wsl/install)
and [Basic WSL commands](https://learn.microsoft.com/windows/wsl/basic-commands).

### 2.3 Verify Windows-to-WSL execution

Replace the placeholders with the exact distribution and Linux username:

```powershell
wsl.exe --distribution <DistributionName> --user <LinuxUser> --exec /bin/true
if ($LASTEXITCODE -ne 0) { throw "WSL exec verification failed" }

wsl.exe --distribution <DistributionName> --user <LinuxUser> --exec /usr/bin/id
wsl.exe --distribution <DistributionName> --exec /bin/uname -a
```

Required result:

- `/bin/true` exits `0`;
- `id` identifies the intended non-root user;
- `uname` reports a Linux kernel containing the WSL/Microsoft identity.

The Windows launcher depends on this exact `wsl.exe --distribution … --user …
--exec …` capability.

### 2.4 Verify or install WebView2 Runtime

Windows 11 normally has the Evergreen WebView2 Runtime, but CATS must verify it.
This PowerShell check examines both supported 64-bit registry locations:

```powershell
$webView2Id = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
$webView2Paths = @(
  "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$webView2Id",
  "HKCU:\Software\Microsoft\EdgeUpdate\Clients\$webView2Id"
)
$webView2Versions = $webView2Paths | ForEach-Object {
  (Get-ItemProperty -Path $_ -Name pv -ErrorAction SilentlyContinue).pv
} | Where-Object { $_ -and $_ -ne '0.0.0.0' }

$webView2Versions
if (-not $webView2Versions) { throw 'Evergreen WebView2 Runtime is not installed' }
```

If absent, download the **Evergreen Bootstrapper** (online install) or the
architecture-specific **Evergreen Standalone Installer** (offline install) from
Microsoft's [WebView2 download page](https://developer.microsoft.com/microsoft-edge/webview2).
For a downloaded bootstrapper:

```powershell
.\MicrosoftEdgeWebview2Setup.exe /silent /install
```

For an offline standalone installer, use its actual downloaded filename:

```powershell
.\MicrosoftEdgeWebView2RuntimeInstallerX64.exe /silent /install
```

Re-run the registry check. Evergreen is preferred because it receives automatic
runtime/security updates. Microsoft documents both deployment choices in
[Evergreen versus Fixed Version](https://learn.microsoft.com/microsoft-edge/webview2/concepts/evergreen-vs-fixed-version).

### 2.5 Verify PowerShell

```powershell
$PSVersionTable.PSVersion
Get-Command powershell.exe
```

The planned installer should remain compatible with inbox Windows PowerShell
5.1. PowerShell 7 is useful but not a base requirement unless the installer
decision later changes.

### 2.6 Items not required on Windows for end users

Do **not** install these merely to run CATS:

- Visual Studio or Visual Studio Build Tools;
- Go, GCC, MSYS2, Zig, Git for Windows, Node.js, Python, Rust, or Docker;
- WSLg, GTK, WebKitGTK, an X server, or Wayland utilities;
- a separate Microsoft Edge browser installation when the WebView2 Runtime is
  present;
- systemd configuration or Windows firewall/port-proxy rules.

The local launcher will keep a foreground WSL helper alive. Microsoft notes
that systemd services do not themselves keep a WSL instance alive, so systemd
is not the local desktop lifecycle mechanism.

## 3. End-user WSL runtime installation

Open the selected distribution as the intended non-root user.

### 3.1 Verify the distribution, WSL2 kernel, user, and architecture

```bash
. /etc/os-release
printf 'distribution=%s version=%s\n' "$ID" "$VERSION_ID"
uname -r
uname -m
id
printf 'home=%s shell=%s\n' "$HOME" "$SHELL"
```

For the reference setup, expect Ubuntu `24.04`, an `x86_64` kernel name
containing `microsoft-standard-WSL2`, and the intended non-root UID/home.

### 3.2 Install minimum runtime packages

A prebuilt CATS payload carries the static libghostty-vt library, so it does not
need Zig or libghostty installed. Install the packages used by core network and
repository features:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates git
sudo update-ca-certificates
```

Why these are present:

- `ca-certificates`: HTTPS trust for manifest updates, plugin source access,
  Go/agent downloads, ACP tools, and optional push endpoints;
- `git`: worktrees and plugin install/update are first-class CATS features;
- `libc6`: already part of Ubuntu and required by the CGO Linux binaries.

Verify:

```bash
git --version
test -s /etc/ssl/certs/ca-certificates.crt
ldd --version | sed -n '1p'
```

### 3.3 Keep projects in the Linux filesystem

```bash
mkdir -p "$HOME/project"
cd "$HOME/project"
findmnt -T . -o TARGET,SOURCE,FSTYPE,OPTIONS
```

Prefer a WSL Linux filesystem such as `/home/<user>/project`, normally backed
by ext4. Avoid making `/mnt/c/...` the default workspace for Linux agents and
build tools; it works, but cross-OS filesystem access can be substantially
slower. Windows can still browse the Linux files at
`\\wsl.localhost\<DistributionName>\home\<user>\project`.

### 3.4 Install the CATS Linux payload — after implementation

The Windows installer will perform this transaction. The intended result is:

```text
~/.local/lib/cats/<version>/
  catway
  cathost
  catctl
  cats-wsl-host
~/.local/lib/cats/current -> <version>
~/.local/bin/catctl -> ~/.local/lib/cats/current/catctl
```

Verification commands once the helper exists:

```bash
test -x "$HOME/.local/lib/cats/current/catway"
test -x "$HOME/.local/lib/cats/current/cathost"
test -x "$HOME/.local/lib/cats/current/catctl"
test -x "$HOME/.local/lib/cats/current/cats-wsl-host"

file "$HOME/.local/lib/cats/current/"{catway,cathost,catctl,cats-wsl-host}
"$HOME/.local/lib/cats/current/cats-wsl-host" --health-json
```

All four must be Linux executables for the same architecture and report the
same CATS build/version. They must be installed in the WSL filesystem, not
executed from `/mnt/c`.

## 4. WSL source-build/developer installation

This section is required when building the Linux payload from the repository.
It is not required for users of a prebuilt payload.

### 4.1 Install Linux build packages

```bash
sudo apt-get update
sudo apt-get install -y \
  bash \
  binutils \
  build-essential \
  ca-certificates \
  coreutils \
  curl \
  file \
  findutils \
  git \
  make \
  mawk \
  pkg-config \
  tar \
  xz-utils
```

Package purposes:

| Package | Used for |
|---|---|
| `build-essential` | GCC/G++, libc headers, and standard native build tools for CGO/race builds |
| `pkg-config` | Finds the vendored static libghostty-vt output for tagged Go builds |
| `curl`, `ca-certificates` | Downloads verified Go/Zig/dependencies over HTTPS |
| `xz-utils`, `tar` | Extracts the pinned Zig `.tar.xz` archive |
| `coreutils`, `mawk`, `findutils`, `bash` | Existing build-script utilities (`sha256sum`, `base64`, `awk`, `find`, Bash) |
| `git` | checkout, build stamps, plugins, and worktrees |
| `file`, `binutils` | release verification (`file`, `readelf`, `ldd` companion checks) |

Verify them:

```bash
for command in bash gcc g++ make pkg-config curl git tar xz sha256sum awk find file readelf; do
  command -v "$command" || exit 1
done
gcc --version | sed -n '1p'
pkg-config --version
git --version
```

### 4.2 Install Go 1.26 or newer

Ubuntu 24.04's base repository may not provide the Go version declared by this
repository (`go 1.26.0`). Use an official Go binary release or another managed
source that provides Go 1.26+.

As of this document's verification date, the current patch is Go 1.26.5. For
Linux amd64:

```bash
cd /tmp
curl -fLO https://go.dev/dl/go1.26.5.linux-amd64.tar.gz
printf '%s  %s\n' \
  '5c2c3b16caefa1d968a94c1daca04a7ca301a496d9b086e17ad77bb81393f053' \
  'go1.26.5.linux-amd64.tar.gz' | sha256sum -c -

sudo mkdir -p /opt/go1.26.5
sudo tar -C /opt/go1.26.5 --strip-components=1 \
  -xzf go1.26.5.linux-amd64.tar.gz
sudo ln -sfn /opt/go1.26.5 /opt/go
```

Add it to the login environment used by the future host helper:

```bash
printf '\nexport PATH=/opt/go/bin:$HOME/go/bin:$PATH\n' >> "$HOME/.profile"
export PATH=/opt/go/bin:$HOME/go/bin:$PATH
```

For ARM64, select the current `linux-arm64` archive and checksum from the
[official Go downloads JSON](https://go.dev/dl/?mode=json); do not reuse the
amd64 filename/checksum. Before copying these pinned commands later, re-check
the downloads page for a supported Go 1.26 security patch.

Verify:

```bash
go version
go env GOOS GOARCH CGO_ENABLED CC CXX
```

Expected for the reference build: Go `1.26.x`, `linux`, `amd64`,
`CGO_ENABLED=1`, and usable GCC/G++ commands.

### 4.3 Do not install system Zig

The repository pins Zig 0.15.2 and `scripts/build-libghostty-vt.sh` downloads it
to `.tools/`, verifies its SHA-256, and selects the correct Linux architecture.
A separately installed Zig may be ignored and can make manual builds diverge.

After `make vt`, verify:

```bash
test -x .tools/zig-x86_64-linux-0.15.2/zig
.tools/zig-x86_64-linux-0.15.2/zig version
test -f third_party/libghostty-vt/zig-out/lib/libghostty-vt.a
test -f third_party/libghostty-vt/zig-out/share/pkgconfig/libghostty-vt-static.pc
```

Use the `aarch64-linux` tool directory on ARM64.

### 4.4 Build and verify the existing Linux application

Keep the checkout in the WSL filesystem:

```bash
cd "$HOME/project/cats"
make vt
make wsl-payload
make check
```

Then verify artifacts and dynamic dependencies:

```bash
for binary in catway cathost catctl cats-wsl-host; do
  test -x "bin/$binary" || exit 1
  file "bin/$binary"
done

ldd bin/catway
ldd bin/cathost
bin/catway -h >/dev/null
bin/cathost -h >/dev/null
bin/catctl help >/dev/null
bin/cats-wsl-host --health-json
```

Expected:

- `make vt`, `make wsl-payload`, and `make check` exit `0`;
- the binaries are Linux executables for the expected architecture;
- libghostty-vt is statically incorporated (there is no runtime
  `libghostty-vt.so` dependency);
- normal glibc/system-library dependencies resolve.

### 4.5 Current backend smoke test through Windows localhost

**Verified on the reference Windows 11/WSL2 host (2026-08-09).** The existing
WSL implementation was reachable at its loopback URL from a Chrome tab running
on Windows 11, and a pane could be created and used. This is evidence for the
current backend/browser and Windows-to-WSL localhost path; it does not yet
verify the planned native WebView2 launcher. The Phase 2 `catapp` structural
refactor and Phase 3 helper do not invalidate this existing backend/browser
smoke-test result. Phase 3 separately exercised the same backend through the
production helper on WSL2; Windows launcher ownership begins in Phase 4.

This tests the backend and WSL networking before the native launcher exists.
Use two WSL terminals.

WSL terminal 1:

```bash
cd "$HOME/project/cats"
bin/cathost -socket /tmp/cats-cathost.sock -persistent
```

WSL terminal 2:

```bash
cd "$HOME/project/cats"
bin/catway --addr 127.0.0.1:8421 --auth none \
  --socket /tmp/cats-cathost.sock
```

From Windows PowerShell:

```powershell
$response = Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8421/
$response.StatusCode
if ($response.StatusCode -ne 200) { throw 'Windows-to-WSL localhost test failed' }
```

Open `http://127.0.0.1:8421/` in a Windows browser and create/type in a pane.
Stop the two WSL processes after the test. Do not change the address to
`0.0.0.0`; this unauthenticated smoke test is intentionally loopback-only.

Microsoft documents Windows access to WSL services through localhost in
[WSL networking](https://learn.microsoft.com/windows/wsl/networking).

### 4.6 Phase 4 launcher-core integration test

**Verified on the reference Windows 11/WSL2 host (2026-08-09).** This exercises
the Windows process/config/network implementation without constructing the
native WebView2 window. From the WSL repository after `make wsl-payload`:

```bash
make test-catapp-windows-compile
/tmp/cats-catapp-windows.test.exe -test.v \
  -test.run '^TestWindowsWSLBackendIntegration$' \
  -cats-wsl-integration \
  -cats-wsl-distro '<DistributionName>' \
  -cats-wsl-user '<LinuxUser>' \
  -cats-wsl-payload '/absolute/linux/payload/path'
```

The PE executable runs as a Windows process through WSL interop. It must list
the configured distribution, validate helper health and the shared build hash,
start the helper with direct `wsl.exe` arguments, verify
`/.well-known/cats/health` from Windows loopback, send protocol stop, and leave
no helper/daemon/runtime directory behind. This passed with `Ubuntu`, user
`bylaz`, payload `/home/bylaz/project/cats/bin`, and build `d91aa1d` in 1.88s;
the final rebuilt-payload verification passed again in 1.20s.

This is not a substitute for the native cgo/WebView2 tests in section 5.5. It
does not exercise the real window, menu, cookies, clipboard contents, keyboard,
notifications, or navigation callbacks.

## 5. Windows launcher developer installation

This section is for implementing/building `Cats.exe`. End users do not need it.

### 5.1 Install Go on Windows

Install the current supported Go 1.26 patch using the amd64 MSI from the
[official Go downloads](https://go.dev/dl/). At this document's verification
date that is `go1.26.5.windows-amd64.msi`.

Open a **new** PowerShell window and verify:

```powershell
go version
go env GOOS GOARCH CGO_ENABLED CC CXX
```

Expected: `windows`, `amd64`, Go `1.26.x`, and `CGO_ENABLED=1` after the compiler
toolchain below is on `PATH`.

### 5.2 Install MSYS2 and the UCRT64 MinGW-w64 toolchain

Go's cgo support on Windows requires GCC in `PATH`; Go 1.25+ additionally
requires a compiler/binutils combination with DWARF 5 support. Use a current
MSYS2 UCRT64 toolchain rather than an old standalone MinGW installation.

1. Install MSYS2 from the [official MSYS2 installer](https://www.msys2.org/docs/installer/)
   using its default `C:\msys64` location.
2. Open the **MSYS2 UCRT64** terminal, not the plain MSYS shell.
3. Update it, closing/reopening the UCRT64 terminal if `pacman` requests it:

```bash
pacman -Syu
pacman -Su
pacman -S --needed mingw-w64-ucrt-x86_64-toolchain
```

4. Add `C:\msys64\ucrt64\bin` to the Windows user `PATH`, then open a new
   PowerShell window.

Verify from PowerShell:

```powershell
where.exe gcc
where.exe g++
where.exe windres
gcc --version
g++ --version
go env CGO_ENABLED CC CXX

$libsync = gcc --print-file-name libsynchronization.a
$libsync
if ($libsync -eq 'libsynchronization.a') {
  throw 'MinGW runtime is too old for the Go race detector'
}
```

Do not put multiple MinGW/Cygwin compiler directories ahead of UCRT64 in
`PATH`. The Go project documents the Windows GCC requirement in its
[cgo guidance](https://go.dev/wiki/cgo), and MSYS2 publishes the
[UCRT64 toolchain package](https://packages.msys2.org/groups/mingw-w64-ucrt-x86_64-toolchain).

### 5.3 Install Git for Windows

Git is optional if the Windows build receives a source archive, but recommended
for a native Windows checkout and build stamps. Install from
[git-scm.com](https://git-scm.com/download/win), then verify:

```powershell
git --version
git config --get core.autocrlf
```

Keep generated shell scripts with LF endings. Avoid maintaining unrelated edits
in two independent checkouts; build both Windows and WSL artifacts from the same
commit.

### 5.4 WebView2 development inputs

The current `github.com/webview/webview_go` module carries the WebView/WebView2
headers used by cgo. Do not separately add a .NET WebView2 NuGet package to this
Go application. The machine still needs the Evergreen runtime from section 2.4
to execute the built launcher.

The upstream webview library requires C++14 and maps Windows to WebView2. The
selected current MSYS2 toolchain satisfies that compiler requirement.

### 5.5 Native WebView2 build verification — pending toolchain installation

From a Windows checkout at the same commit as the WSL payload:

```powershell
go mod download
go test .\cmd\catapp
go test .\internal\desktopproto
go test .\internal\wslclient

New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath `
  -ldflags '-H windowsgui -X main.defaultMode=local' `
  -o bin\Cats.exe .\cmd\catapp

if ($LASTEXITCODE -ne 0) { throw 'Windows launcher build failed' }
Get-Item .\bin\Cats.exe | Select-Object FullName, Length, LastWriteTime
```

Launch the executable only after the matching Linux payload and selected WSL
target have been installed. The final Makefile/script may wrap these commands;
that wrapper becomes authoritative once implemented.

### 5.6 Optional Windows developer tools

Install these only for the corresponding work:

- Windows SDK `signtool.exe`: Authenticode signing and signature verification;
- MSIX Packaging Tool/WiX: only if the D-017 per-user archive decision is later
  superseded;
- Visual Studio 2022 C++ workload: useful for WebView2 native debugging, but not
  a replacement for the validated MinGW compiler used by Go cgo unless the
  build system is deliberately changed;
- Edge WebDriver: automated WebView2 GUI tests;
- PowerShell 7: installer development convenience, not a runtime prerequisite.

## 6. Optional agent and feature packages

CATS does not install AI agents or authenticate them. Install each desired
agent **inside WSL** using that vendor's current instructions, then sign in
there. Examples include Claude Code, Codex, GitHub Copilot CLI, Gemini, and the
other manifests in `internal/detect/manifests`.

Verification pattern:

```bash
command -v claude || true
command -v codex || true
command -v copilot || true
catctl integration status
```

Only install an integration after its agent is installed in WSL:

```bash
catctl integration install <agent>
catctl integration status
```

Additional runtimes such as Node.js belong to the selected agent/plugin, not to
CATS core. Likewise, `zsh`, `fish`, fonts, VPN clients, Tailscale, and push
notification services are optional features.

## 7. Complete verification checklist

### Windows runtime

- [ ] Windows 11 and supported architecture confirmed.
- [ ] `wsl --version` and `wsl --status` succeed.
- [ ] Selected distribution shows WSL version `2`.
- [ ] Explicit distro/user `/bin/true` execution exits `0`.
- [ ] Evergreen WebView2 registry version is present and not `0.0.0.0`.
- [ ] Windows PowerShell 5.1+ is available.
- [ ] Windows-to-WSL localhost smoke test returns HTTP 200.

### WSL runtime

- [ ] Supported distro/version, WSL2 kernel, non-root user, and architecture confirmed.
- [ ] `ca-certificates` and `git` installed and verified.
- [ ] Projects stored under the WSL Linux filesystem by default.
- [ ] All four final payload binaries exist, are executable, match architecture,
      and report one version (**after implementation**).
- [ ] Local server listens only on `127.0.0.1`.
- [ ] Agent CLIs, credentials, and hooks are installed inside WSL as needed.

### WSL source build

- [ ] Linux build-command loop finds every required command.
- [ ] Go is 1.26.x or newer and `CGO_ENABLED=1`.
- [ ] `make vt`, `make binaries`, and `make check` pass.
- [ ] Static libghostty-vt output and pkg-config file exist.
- [ ] `file`/`ldd` show the expected architecture and no missing libraries.

### Windows launcher build

- [ ] Windows Go is 1.26.x or newer.
- [ ] Current MSYS2 UCRT64 `gcc`, `g++`, and `windres` are first on `PATH`.
- [ ] MinGW runtime/race-detector verification passes.
- [ ] WebView2 runtime check passes.
- [ ] Windows unit tests and `Cats.exe` build pass (**after implementation**).
- [ ] Launcher and WSL payload build stamps match.

## 8. Troubleshooting quick reference

| Symptom | Verification/action |
|---|---|
| `wsl --version` is not recognized | Run `wsl --update`; install/update the Store-delivered WSL package |
| Distribution shows version 1 | `wsl --set-version <DistributionName> 2` |
| `0x80370102`/virtual machine failure | Enable CPU virtualization and verify required Windows virtualization features |
| WebView2 registry check returns nothing | Install Evergreen Bootstrapper/Standalone Runtime and recheck |
| Windows cannot reach `127.0.0.1:8421` | Confirm catway is running/bound to loopback; check `wsl --status`, VPN/firewall policy, and WSL networking mode |
| `go: command not found` in WSL | Install Go 1.26+, add `/opt/go/bin` to `~/.profile`, reopen WSL |
| `package libghostty-vt not found` | Run `make vt`; build through Makefile so `PKG_CONFIG_PATH` is set |
| Zig download/extract fails | Verify `curl`, CA trust, `xz`, disk space, proxy/VPN access, then rerun `make vt` |
| Windows cgo cannot find GCC | Put `C:\msys64\ucrt64\bin` on Windows `PATH`; reopen PowerShell |
| Windows executable is non-working with new Go | Remove old MinGW paths; verify current UCRT64 binutils/DWARF support |
| Agent/plugin command missing | Install it inside WSL and ensure the login shell `PATH` exposes it |
| Builds are slow under `/mnt/c` | Move the checkout/project into `/home/<user>/...` |
