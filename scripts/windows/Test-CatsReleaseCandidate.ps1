[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PackageArchive,
    [Parameter(Mandatory = $true)][string]$Distribution,
    [Parameter(Mandatory = $true)][string]$User,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion,
    [switch]$AllowUnsignedDevelopment
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$PackageArchive = (Resolve-Path -LiteralPath $PackageArchive).Path
$verify = @{ PackageArchive = $PackageArchive; ExpectedVersion = $ExpectedVersion }
if (-not $AllowUnsignedDevelopment) { $verify.RequireSigned = $true }
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
    & go test -count=1 -ldflags $ldflags .\cmd\catapp `
        -run '^TestWindowsNative(WebViewIntegration|WSLEndToEnd)$' `
        -args -cats-windows-ui-integration -cats-wsl-integration `
        -cats-wsl-distro $Distribution -cats-wsl-user $User -cats-wsl-payload $installed.PayloadPath
    if ($LASTEXITCODE -ne 0) { throw "native Windows/WSL GUI test exited with code $LASTEXITCODE" }

    Invoke-CatsUninstall -Distribution $Distribution -User $User -Force -PayloadRootOverride $payloadRoot `
        -LocalAppDataOverride $localAppData -StartMenuOverride $startMenu -DesktopOverride $desktop | Out-Null
    $installed = $null
    $candidateLabel = if ($AllowUnsignedDevelopment) { 'development candidate' } else { 'signed release candidate' }
    Write-Host "PASS: $candidateLabel install, WebView2, pane command, clipboard, close, relaunch, restore, and uninstall"
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
