$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0
Import-Module (Join-Path $PSScriptRoot 'CatsInstaller.psm1') -Force
$installerModule = Get-Module CatsInstaller

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

function Assert-Throws {
    param([scriptblock]$Action, [string]$Message)
    try { & $Action; throw "expected failure: $Message" }
    catch {
        if ($_.Exception.Message -eq "expected failure: $Message") { throw }
    }
}

$temporary = Join-Path ([IO.Path]::GetTempPath()) ('cats-installer-test-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($temporary) | Out-Null
try {
    $package = Join-Path $temporary 'package'
    [IO.Directory]::CreateDirectory((Join-Path $package 'nested')) | Out-Null
    [IO.File]::WriteAllText((Join-Path $package 'nested\asset.txt'), 'payload', (New-Object Text.UTF8Encoding($false)))
    $digest = (Get-FileHash -LiteralPath (Join-Path $package 'nested\asset.txt') -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $package 'SHA256SUMS'), "$digest  nested/asset.txt`n", (New-Object Text.UTF8Encoding($false)))
    Assert-True (Test-CatsPackageChecksums $package) 'valid package checksums were rejected'
    Add-Content -LiteralPath (Join-Path $package 'nested\asset.txt') -Value 'tampered'
    Assert-Throws { Test-CatsPackageChecksums $package | Out-Null } 'tampering should fail'
    [IO.File]::WriteAllText((Join-Path $package 'SHA256SUMS'), "$digest  ../escape.txt`n", (New-Object Text.UTF8Encoding($false)))
    Assert-Throws { Test-CatsPackageChecksums $package | Out-Null } 'traversal should fail'

    & $installerModule { param($kernel) Assert-CatsWsl2Kernel $kernel } '6.18.33.2-microsoft-standard-WSL2'
    & $installerModule { param($osRelease) Assert-CatsSupportedDistribution $osRelease } "ID=ubuntu`nVERSION_ID=`"24.04`""
    Assert-Throws {
        & $installerModule { param($kernel) Assert-CatsWsl2Kernel $kernel } '4.4.0-Microsoft'
    } 'WSL1 should fail'
    Assert-Throws {
        & $installerModule { param($osRelease) Assert-CatsSupportedDistribution $osRelease } "ID=debian`nVERSION_ID=`"13`""
    } 'unsupported distribution should fail'

    $configPath = Join-Path $temporary 'config\app.json'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $configPath)) | Out-Null
    [IO.File]::WriteAllText($configPath, '{"mode":"remote","remote":{"url":"https://cats.example","label":"home"},"future":{"keep":true}}', (New-Object Text.UTF8Encoding($false)))
    Save-CatsAppConfig $configPath 'Ubuntu' 'alice' '/home/alice/.local/lib/cats/current' | Out-Null
    $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
    Assert-True ($config.mode -eq 'local') 'installer did not select local mode'
    Assert-True ($config.remote.url -eq 'https://cats.example') 'remote target was not preserved'
    Assert-True ([bool]$config.future.keep) 'additive launcher configuration was not preserved'
    Assert-True ($config.wsl.payload_path -eq '/home/alice/.local/lib/cats/current') 'WSL target was not written'

    $defaultPlan = @(Get-CatsUninstallPlan 'C:\Users\alice\AppData\Local' 'C:\Users\alice\AppData\Roaming\Microsoft\Windows\Start Menu' 'C:\Users\alice\Desktop' '/home/alice/.local/lib/cats')
    Assert-True ($defaultPlan.Count -eq 7) 'default uninstall should enumerate three managed links and four executable/shortcut paths'
    Assert-True (@($defaultPlan | Where-Object Kind -eq 'managed-link').Count -eq 3) 'default uninstall omitted managed command links'
    Assert-True (@($defaultPlan | Where-Object Kind -eq 'data').Count -eq 0) 'default uninstall included user data'
    $allPlan = @(Get-CatsUninstallPlan 'C:\Users\alice\AppData\Local' 'C:\Start' 'C:\Desktop' '/home/alice/.local/lib/cats' -RemoveAllData -ConfigRoot '/home/alice/.config/cats' -StateRoot '/home/alice/.local/state/cats')
    Assert-True (@($allPlan | Where-Object Kind -eq 'data').Count -eq 3) 'remove-all plan did not enumerate all CATS-owned data roots'
    Assert-Throws { Get-CatsUninstallPlan 'C:\Local' 'C:\Start' 'C:\Desktop' '/home/alice/project' | Out-Null } 'unexpected payload root should fail closed'

    Write-Host 'PASS: installer checksum, config-merge, and uninstall-plan tests'
}
finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
