[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Distribution,

    [Parameter(Mandatory = $true)]
    [string]$LinuxUser,

    [Parameter(Mandatory = $true)]
    [string]$LinuxProbe,

    [ValidateRange(1024, 65534)]
    [int]$Port = 18421
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Assert-SafeInputs {
    if ($Distribution -notmatch '^[A-Za-z0-9._-]{1,128}$') {
        throw 'This PowerShell 5.1 spike supports simple distribution names only'
    }
    if ($LinuxUser -notmatch '^[a-z_][a-z0-9_-]*[$]?$') {
        throw 'LinuxUser is not a safe Linux account name'
    }
    if ($LinuxProbe -notmatch '^/[A-Za-z0-9._/-]+$' -or $LinuxProbe.Contains('..')) {
        throw 'LinuxProbe must be a simple absolute Linux path'
    }
}

function Read-Record([System.Diagnostics.Process]$Process, [string]$ExpectedType) {
    $line = $Process.StandardOutput.ReadLine()
    if ($null -eq $line) {
        throw "wsl.exe closed stdout while waiting for $ExpectedType"
    }
    $record = $line | ConvertFrom-Json
    if ($record.v -ne 1 -or $record.type -ne $ExpectedType) {
        throw "unexpected lifecycle record: $line"
    }
    return $record
}

function Start-Probe([int]$ProbePort) {
    # ProcessStartInfo launches wsl.exe directly. PowerShell 5.1 lacks the
    # ProcessStartInfo.ArgumentList API, so this disposable spike restricts all
    # inputs to values that require no Windows command-line quoting. No cmd.exe,
    # PowerShell expression, or Linux shell parses these values.
    $arguments = "--distribution $Distribution --user $LinuxUser --exec $LinuxProbe --port $ProbePort"

    $start = New-Object System.Diagnostics.ProcessStartInfo
    $start.FileName = 'wsl.exe'
    $start.Arguments = $arguments
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true

    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $start
    if (-not $process.Start()) {
        throw 'failed to start wsl.exe'
    }
    return $process
}

function Wait-Exit([System.Diagnostics.Process]$Process) {
    if (-not $Process.WaitForExit(5000)) {
        $Process.Kill()
        throw 'wsl.exe did not exit within five seconds'
    }
    if ($Process.ExitCode -ne 0) {
        $stderr = $Process.StandardError.ReadToEnd()
        throw "wsl.exe exited $($Process.ExitCode): $stderr"
    }
}

Assert-SafeInputs

$distros = @(wsl.exe --list --quiet) | ForEach-Object { $_.Trim([char]0).Trim() } | Where-Object { $_ }
if ($distros -notcontains $Distribution) {
    throw "WSL distribution '$Distribution' is not installed: $($distros -join ', ')"
}

& wsl.exe --distribution $Distribution --user $LinuxUser --exec /bin/true
if ($LASTEXITCODE -ne 0) {
    throw 'explicit WSL distribution/user/exec check failed'
}

$process = Start-Probe -ProbePort $Port
$windowsBindWhileWslBusy = 'not-run'
try {
    $null = Read-Record -Process $process -ExpectedType 'starting'
    $ready = Read-Record -Process $process -ExpectedType 'ready'
    if ($ready.addr -ne "127.0.0.1:$Port") {
        throw "unexpected ready address: $($ready.addr)"
    }

    $response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 "http://127.0.0.1:$Port/"
    if ($response.StatusCode -ne 200 -or $response.Content -notmatch 'cats WSL Phase 1 probe') {
        throw 'Windows localhost request returned an unexpected response'
    }

    $parallelListener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, $Port)
    try {
        $parallelListener.Start()
        $windowsBindWhileWslBusy = 'allowed'
    }
    catch [System.Net.Sockets.SocketException] {
        $windowsBindWhileWslBusy = 'blocked'
    }
    finally {
        $parallelListener.Stop()
    }

    $process.StandardInput.WriteLine('{"v":1,"type":"stop"}')
    $process.StandardInput.Flush()
    $stopped = Read-Record -Process $process -ExpectedType 'stopped'
    if ($stopped.reason -ne 'requested') {
        throw "unexpected graceful-stop reason: $($stopped.reason)"
    }
    Wait-Exit -Process $process
}
finally {
    if (-not $process.HasExited) {
        $process.Kill()
    }
    $process.Dispose()
}

$eofPort = $Port + 1
$process = Start-Probe -ProbePort $eofPort
try {
    $null = Read-Record -Process $process -ExpectedType 'starting'
    $null = Read-Record -Process $process -ExpectedType 'ready'
    $process.StandardInput.Close()
    $stopped = Read-Record -Process $process -ExpectedType 'stopped'
    if ($stopped.reason -ne 'stdin_eof') {
        throw "unexpected EOF-stop reason: $($stopped.reason)"
    }
    Wait-Exit -Process $process
}
finally {
    if (-not $process.HasExited) {
        $process.Kill()
    }
    $process.Dispose()
}

$windowsPort = $Port + 2
$windowsListener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, $windowsPort)
$wslBindWhileWindowsBusy = 'not-run'
$windowsListener.Start()
try {
    $process = Start-Probe -ProbePort $windowsPort
    try {
        $null = Read-Record -Process $process -ExpectedType 'starting'
        $line = $process.StandardOutput.ReadLine()
        if ($null -eq $line) {
            throw 'wsl.exe closed stdout during the Windows-port conflict probe'
        }
        $record = $line | ConvertFrom-Json
        if ($record.v -ne 1) {
            throw "unexpected conflict lifecycle record: $line"
        }
        if ($record.type -eq 'error' -and $record.stage -eq 'listen') {
            $wslBindWhileWindowsBusy = 'blocked'
        }
        elseif ($record.type -eq 'ready') {
            $wslBindWhileWindowsBusy = 'allowed'
            $process.StandardInput.WriteLine('{"v":1,"type":"stop"}')
            $process.StandardInput.Flush()
            $null = Read-Record -Process $process -ExpectedType 'stopped'
        }
        else {
            throw "unexpected conflict lifecycle record: $line"
        }
        if (-not $process.WaitForExit(5000)) {
            $process.Kill()
            throw 'conflict probe did not exit within five seconds'
        }
    }
    finally {
        if (-not $process.HasExited) {
            $process.Kill()
        }
        $process.Dispose()
    }
}
finally {
    $windowsListener.Stop()
}

[ordered]@{
    distribution = $Distribution
    user = $LinuxUser
    localhost = "http://127.0.0.1:$Port/"
    graceful_stop = 'pass'
    stdin_eof = 'pass'
    windows_bind_while_wsl_busy = $windowsBindWhileWslBusy
    wsl_bind_while_windows_busy = $wslBindWhileWindowsBusy
} | ConvertTo-Json
