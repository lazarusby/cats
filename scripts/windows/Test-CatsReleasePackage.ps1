[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PackageArchive,
    [string]$ExpectedVersion,
    [switch]$RequireSigned
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Assert-CatsRelease {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

function Invoke-CatsNative {
    param([string]$FilePath, [string[]]$Arguments)
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$FilePath exited with code $LASTEXITCODE" }
}

$PackageArchive = (Resolve-Path -LiteralPath $PackageArchive).Path
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('cats-release-test-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($temporary) | Out-Null
try {
    Expand-Archive -LiteralPath $PackageArchive -DestinationPath $temporary
    $roots = @(Get-ChildItem -LiteralPath $temporary -Directory)
    Assert-CatsRelease ($roots.Count -eq 1) 'release archive must contain exactly one root directory'
    $root = $roots[0].FullName
    Import-Module (Join-Path $root 'CatsInstaller.psm1') -Force
    Test-CatsPackageChecksums $root | Out-Null

    $release = Get-Content -LiteralPath (Join-Path $root 'release.json') -Raw | ConvertFrom-Json
    Assert-CatsRelease ($release.schema -eq 2 -and $release.product -eq 'cats-windows-wsl') 'unsupported release manifest'
    if ($ExpectedVersion) { Assert-CatsRelease ($release.release_id -eq $ExpectedVersion) 'release version does not match the requested tag' }
    Assert-CatsRelease ([string]$release.source_commit -match '^[0-9a-f]{40}$') 'source commit is not a full Git SHA'
    Assert-CatsRelease ([string]$release.compatibility_version -match '^[0-9a-f]{7,40}$') 'compatibility version is not a Git hash'
    Assert-CatsRelease ($release.source_commit.StartsWith([string]$release.compatibility_version)) 'compatibility hash does not identify the source commit'
    Assert-CatsRelease ($release.windows_architecture -eq 'amd64' -and $release.wsl_architecture -eq 'amd64') 'release architecture pair is not windows/amd64 + WSL/amd64'
    Assert-CatsRelease ($release.supported_distribution.id -eq 'ubuntu' -and $release.supported_distribution.version -eq '24.04') 'release distribution floor is not Ubuntu 24.04'

    $launcher = Join-Path $root ([string]$release.components.launcher.file)
    $payload = Join-Path $root ([string]$release.components.payload.file)
    Assert-CatsRelease ((Get-FileHash -LiteralPath $launcher -Algorithm SHA256).Hash.ToLowerInvariant() -eq $release.components.launcher.sha256) 'launcher digest does not match component mapping'
    Assert-CatsRelease ((Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant() -eq $release.components.payload.sha256) 'payload digest does not match component mapping'
    Assert-CatsRelease ($release.components.launcher.compatibility_version -eq $release.compatibility_version) 'launcher compatibility mapping is inconsistent'
    Assert-CatsRelease ($release.components.payload.compatibility_version -eq $release.compatibility_version) 'payload compatibility mapping is inconsistent'

    $payloadExtract = Join-Path $temporary 'payload'
    [IO.Directory]::CreateDirectory($payloadExtract) | Out-Null
    Invoke-CatsNative 'tar.exe' @('-xzf', $payload, '-C', $payloadExtract)
    $payloadRoots = @(Get-ChildItem -LiteralPath $payloadExtract -Directory)
    Assert-CatsRelease ($payloadRoots.Count -eq 1) 'payload archive must contain exactly one root directory'
    $payloadRoot = $payloadRoots[0].FullName
    Test-CatsPackageChecksums $payloadRoot | Out-Null
    $payloadRelease = Get-Content -LiteralPath (Join-Path $payloadRoot 'release.json') -Raw | ConvertFrom-Json
    Assert-CatsRelease ($payloadRelease.schema -eq 2 -and $payloadRelease.product -eq 'cats-wsl-payload') 'unsupported payload manifest'
    foreach ($property in @('release_id', 'compatibility_version', 'source_commit')) {
        Assert-CatsRelease ($payloadRelease.$property -eq $release.$property) "payload $property does not match launcher package"
    }
    Assert-CatsRelease ($payloadRelease.distribution.id -eq 'ubuntu' -and $payloadRelease.distribution.version -eq '24.04') 'payload distribution floor is inconsistent'

    $signaturePaths = @($launcher, (Join-Path $root 'Install-Cats.ps1'), (Join-Path $root 'Uninstall-Cats.ps1'), (Join-Path $root 'CatsInstaller.psm1'))
    $statuses = @($signaturePaths | ForEach-Object { (Get-AuthenticodeSignature -FilePath $_).Status.ToString() })
    $allSigned = @($statuses | Where-Object { $_ -ne 'Valid' }).Count -eq 0
    Assert-CatsRelease ([bool]$release.authenticode.release_candidate_signed -eq $allSigned) 'manifest signing claim does not match package files'
    if ($RequireSigned) { Assert-CatsRelease $allSigned "release candidate signatures are not all valid: $($statuses -join ', ')" }

    $expectedName = "cats_$($release.release_id)_windows_amd64_wsl_ubuntu-24.04_amd64.zip"
    Assert-CatsRelease ((Split-Path -Leaf $PackageArchive) -eq $expectedName) 'archive name does not encode version, Windows arch, WSL distro floor, and WSL arch'
    $signingLabel = if ($allSigned) { 'signed' } else { 'development/unsigned' }
    Write-Host "PASS: $signingLabel release package $expectedName maps its launcher and Ubuntu 24.04/amd64 payload to $($release.source_commit)"
}
finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
