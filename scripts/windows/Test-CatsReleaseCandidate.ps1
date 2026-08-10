[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PackageArchive,
    [Parameter(Mandatory = $true)][string]$Distribution,
    [Parameter(Mandatory = $true)][string]$User,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion,
    [switch]$AllowUnsignedDevelopment,
    [switch]$IncludePhase8Qualification,
    [switch]$IncludeDistributionShutdownQualification,
    [switch]$SkipNativeUIForDevelopment
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$PackageArchive = (Resolve-Path -LiteralPath $PackageArchive).Path
$verify = @{ PackageArchive = $PackageArchive; ExpectedVersion = $ExpectedVersion }
if (-not $AllowUnsignedDevelopment) { $verify.RequireSigned = $true }
if ($SkipNativeUIForDevelopment -and -not $AllowUnsignedDevelopment) {
    throw '-SkipNativeUIForDevelopment is allowed only for an explicitly unsigned local development run'
}
if ($IncludeDistributionShutdownQualification -and -not $IncludePhase8Qualification) {
    throw '-IncludeDistributionShutdownQualification requires -IncludePhase8Qualification'
}
& (Join-Path $PSScriptRoot 'Test-CatsReleasePackage.ps1') @verify

$token = [Guid]::NewGuid().ToString('N')
$windowsRoot = Join-Path ([IO.Path]::GetTempPath()) ("cats phase7 gui $([char]0x00fc)nicode-$token")
$wslBase = "/tmp/cats-phase7-gui-$token"
if ($wslBase -notmatch '^/tmp/cats-phase7-gui-[0-9a-f]{32}$') { throw 'unsafe WSL test root' }
$packageRoot = Join-Path $windowsRoot 'package'
$localAppData = Join-Path $windowsRoot 'LocalAppData'
$startMenu = Join-Path $windowsRoot 'StartMenu'
$desktop = Join-Path $windowsRoot 'Desktop'
$payloadRoot = "$wslBase/.local/lib/cats"
$oldLocalAppData = $env:LOCALAPPDATA
$oldWslEnv = $env:WSLENV
$oldXdgConfig = $env:XDG_CONFIG_HOME
$oldXdgState = $env:XDG_STATE_HOME
$installed = $null

function Measure-CatsPhase8Filesystem {
    param([string]$Label, [string]$Path)
    & wsl.exe --distribution $Distribution --user $User --exec /bin/mkdir -p -- $Path
    if ($LASTEXITCODE -ne 0) { throw "could not create $Label performance root" }
    $file = "$Path/io.bin"
    $write = Measure-Command {
        & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/dd if=/dev/zero "of=$file" bs=1M count=64 conv=fdatasync status=none
        if ($LASTEXITCODE -ne 0) { throw "$Label sequential write failed" }
    }
    $read = Measure-Command {
        & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/dd "if=$file" of=/dev/null bs=1M status=none
        if ($LASTEXITCODE -ne 0) { throw "$Label sequential read failed" }
    }
    [ordered]@{ label = $Label; mib = 64; write_ms = [math]::Round($write.TotalMilliseconds); read_ms = [math]::Round($read.TotalMilliseconds) }
}

try {
    [IO.Directory]::CreateDirectory($packageRoot) | Out-Null
    Expand-Archive -LiteralPath $PackageArchive -DestinationPath $packageRoot
    $roots = @(Get-ChildItem -LiteralPath $packageRoot -Directory)
    if ($roots.Count -ne 1) { throw 'release archive must contain exactly one package root' }
    $package = $roots[0].FullName
    $release = Get-Content -LiteralPath (Join-Path $package 'release.json') -Raw | ConvertFrom-Json
    $stampPackage = 'github.com/rohanthewiz/cats/internal/buildinfo'
    $ldflags = "-X $stampPackage.hash=$($release.compatibility_version)"
    $env:XDG_CONFIG_HOME = "$wslBase/config"
    $env:XDG_STATE_HOME = "$wslBase/state"
    $env:WSLENV = (@($oldWslEnv, 'XDG_CONFIG_HOME', 'XDG_STATE_HOME') | Where-Object { $_ }) -join ':'
    & (Join-Path $PSScriptRoot 'Test-CatsInstallerIntegration.ps1') -PackageRoot $package -Distribution $Distribution -User $User
    Import-Module (Join-Path $package 'CatsInstaller.psm1') -Force

    $installed = Invoke-CatsInstall -PackageRoot $package -Distribution $Distribution -User $User `
        -NoLaunch -PayloadRootOverride $payloadRoot -LocalAppDataOverride $localAppData `
        -StartMenuOverride $startMenu -DesktopOverride $desktop

    $env:LOCALAPPDATA = $localAppData
    if (-not $SkipNativeUIForDevelopment) {
        & go test -count=1 -ldflags $ldflags .\cmd\catapp `
            -run '^TestWindowsNative(WebViewIntegration|WSLEndToEnd)$' `
            -args -cats-windows-ui-integration -cats-wsl-integration `
            -cats-wsl-distro $Distribution -cats-wsl-user $User -cats-wsl-payload $installed.PayloadPath
        if ($LASTEXITCODE -ne 0) { throw "native Windows/WSL GUI test exited with code $LASTEXITCODE" }
    }

    if ($IncludePhase8Qualification) {
        & go test -v -count=1 -ldflags $ldflags .\cmd\catapp `
            -run '^TestWindowsNativeWSLPhase8Qualification$' `
            -args -cats-wsl-integration -cats-phase8-qualification `
            -cats-wsl-distro $Distribution -cats-wsl-user $User -cats-wsl-payload $installed.PayloadPath
        if ($LASTEXITCODE -ne 0) { throw "Phase 8 functional/performance qualification exited with code $LASTEXITCODE" }

        $wslRepo = (& wsl.exe --distribution $Distribution --user $User --exec /usr/bin/wslpath -a $repo)
        if ($LASTEXITCODE -ne 0) { throw 'could not translate repository path for the drvfs comparison' }
        $wslRepo = (($wslRepo | ForEach-Object { $_.ToString() }) -join '').Trim()
        $ext4Perf = "$wslBase/perf-ext4"
        $drvfsPerf = "$wslRepo/.cats-phase8-perf-$token"
        if ($drvfsPerf -notmatch '/\.cats-phase8-perf-[0-9a-f]{32}$') { throw 'unsafe drvfs performance root' }
        try {
            $filesystemMetrics = @(
                (Measure-CatsPhase8Filesystem 'wsl-ext4' $ext4Perf),
                (Measure-CatsPhase8Filesystem 'drvfs-mnt-c' $drvfsPerf)
            )
            Write-Host ("PHASE8_FILESYSTEM_METRICS " + ($filesystemMetrics | ConvertTo-Json -Compress))
        }
        finally {
            & wsl.exe --distribution $Distribution --user $User --exec /bin/rm -rf -- $ext4Perf
            & wsl.exe --distribution $Distribution --user $User --exec /bin/rm -rf -- $drvfsPerf
        }

        & go test -v -count=1 -ldflags $ldflags .\cmd\catapp `
            -run '^TestWindowsWSL(BackendIntegration|OwnerLossIntegration)$' `
            -args -cats-wsl-integration -cats-wsl-destructive-integration `
            -cats-wsl-distro $Distribution -cats-wsl-user $User -cats-wsl-payload $installed.PayloadPath
        if ($LASTEXITCODE -ne 0) { throw "Phase 8 owner-loss qualification exited with code $LASTEXITCODE" }

        if ($IncludeDistributionShutdownQualification) {
            & go test -v -count=1 -ldflags $ldflags .\cmd\catapp `
                -run '^TestWindowsWSLDisruptionIntegration$' `
                -args -cats-wsl-integration -cats-wsl-distribution-disruption-integration `
                -cats-wsl-distro $Distribution -cats-wsl-user $User -cats-wsl-payload $installed.PayloadPath
            if ($LASTEXITCODE -ne 0) { throw "Phase 8 distribution terminate/shutdown qualification exited with code $LASTEXITCODE" }
        }
    }

    Invoke-CatsUninstall -Distribution $Distribution -User $User -Force -PayloadRootOverride $payloadRoot `
        -LocalAppDataOverride $localAppData -StartMenuOverride $startMenu -DesktopOverride $desktop | Out-Null
    $installed = $null
    $candidateLabel = if ($AllowUnsignedDevelopment) { 'development candidate' } else { 'signed release candidate' }
    $phase8Label = if ($IncludePhase8Qualification) { ', Phase 8 qualification' } else { '' }
    $nativeLabel = if ($SkipNativeUIForDevelopment) { ', native UI skipped for local development' } else { ', WebView2 and clipboard' }
    Write-Host "PASS: $candidateLabel install$nativeLabel, pane command, close, relaunch, restore$phase8Label, and uninstall"
}
finally {
    $env:LOCALAPPDATA = $oldLocalAppData
    $env:WSLENV = $oldWslEnv
    $env:XDG_CONFIG_HOME = $oldXdgConfig
    $env:XDG_STATE_HOME = $oldXdgState
    if ($installed) {
        try {
            Invoke-CatsUninstall -Distribution $Distribution -User $User -Force -PayloadRootOverride $payloadRoot `
                -LocalAppDataOverride $localAppData -StartMenuOverride $startMenu -DesktopOverride $desktop | Out-Null
        }
        catch { Write-Warning "release-candidate cleanup uninstall failed: $_" }
    }
    & wsl.exe --distribution $Distribution --user $User --exec /bin/rm -rf -- $wslBase
    if ($LASTEXITCODE -ne 0) { Write-Warning 'could not remove isolated WSL GUI test root' }
    Remove-Item -LiteralPath $windowsRoot -Recurse -Force -ErrorAction SilentlyContinue
}
