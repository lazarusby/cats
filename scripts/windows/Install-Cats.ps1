[CmdletBinding()]
param(
    [string]$PackageRoot = $PSScriptRoot,
    [string]$Distribution,
    [string]$User,
    [switch]$DesktopShortcut,
    [switch]$NoCommandLinks,
    [switch]$NoLaunch
)

$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'CatsInstaller.psm1') -Force
try {
    $result = Invoke-CatsInstall -PackageRoot $PackageRoot -Distribution $Distribution -User $User `
        -DesktopShortcut:$DesktopShortcut -NoCommandLinks:$NoCommandLinks -NoLaunch:$NoLaunch
    Write-Host "CATS $($result.Release) installed for $($result.Distribution)/$($result.User)."
    Write-Host "Windows: $($result.WindowsPath)"
    Write-Host "WSL: $($result.PayloadPath)"
}
catch {
    Write-Error $_
    exit 1
}
