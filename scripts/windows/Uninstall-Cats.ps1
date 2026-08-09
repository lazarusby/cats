[CmdletBinding()]
param(
    [string]$Distribution,
    [string]$User,
    [switch]$RemoveAllData,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'CatsInstaller.psm1') -Force
try {
    Invoke-CatsUninstall -Distribution $Distribution -User $User -RemoveAllData:$RemoveAllData -Force:$Force | Out-Null
    Write-Host 'CATS executable payloads and shortcuts were removed.'
    if (-not $RemoveAllData) { Write-Host 'CATS configuration, sessions, plugins, worktrees, and agent configuration were preserved.' }
}
catch {
    Write-Error $_
    exit 1
}
