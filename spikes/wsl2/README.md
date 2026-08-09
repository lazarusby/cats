# WSL2 Phase 1 feasibility spikes

These programs are deliberately outside production packages. They exercise the
Windows/WSL seams before `cmd/catapp` is refactored. Remove each spike after its
result is captured in `docs/wsl2-version/decision_log.md` and equivalent
production tests exist.

## WSL execution, lifecycle pipes, and localhost

Build the Linux probe in WSL, then invoke the PowerShell driver from Windows or
through WSL interop:

```bash
go build -trimpath -o /tmp/cats-wsl-phase1-probe ./spikes/wsl2/hostprobe
powershell.exe -NoProfile -ExecutionPolicy Bypass -File \
  "$(wslpath -w "$PWD/spikes/wsl2/probe-wsl.ps1")" \
  -Distribution Ubuntu -LinuxUser "$USER" \
  -LinuxProbe /tmp/cats-wsl-phase1-probe
```

The driver enumerates distributions, launches `wsl.exe` directly (never through
`cmd.exe`), validates the versioned stdout records, probes the WSL HTTP listener
from Windows, sends a graceful stop record, and repeats startup with stdin EOF.
It writes diagnostics to stderr and JSON lifecycle records to stdout.

## WebView2

This spike must be built by the native Windows Go/cgo toolchain from a Windows
PowerShell prompt in the repository:

```powershell
go build -trimpath -ldflags "-H windowsgui" `
  -o .\bin\cats-webview2-spike.exe .\spikes\wsl2\webview2
.\bin\cats-webview2-spike.exe
```

The page automatically checks navigation, cookies, WebSocket, `Bind`,
`Dispatch`, `Eval`, and resize. It also exposes manual notification, keyboard,
new-window, and teardown checks. The last report is written to
`%TEMP%\cats-webview2-spike.json` because a `windowsgui` binary has no console.

The Windows Go and UCRT64 MinGW toolchains are prerequisites. A missing or
damaged WebView2 Runtime must be tested by a controlled runtime repair/removal
on a disposable Windows test image, not by damaging the developer workstation.
