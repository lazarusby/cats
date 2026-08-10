[CmdletBinding()]
param(
    [string]$PayloadArchive,
    [string]$WslDistribution,
    [string]$WslUser,
    [string]$OutputDirectory,
    [string]$ReleaseVersion,
    [string]$CertificateThumbprint,
    [string]$TimestampServer = 'http://timestamp.digicert.com',
    [switch]$RequireSigned
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Invoke-Native {
    param([string]$FilePath, [string[]]$Arguments)
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$FilePath exited with code $LASTEXITCODE" }
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $repo 'dist' }
[IO.Directory]::CreateDirectory($OutputDirectory) | Out-Null
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$hash = (& git -C $repo rev-parse --short HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $hash -notmatch '^[0-9a-f]{7,40}$') { throw 'could not determine release id' }
$sourceCommit = (& git -C $repo rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $sourceCommit -notmatch '^[0-9a-f]{40}$') { throw 'could not determine source commit' }
$releaseId = if ($ReleaseVersion) { $ReleaseVersion } else { $hash }
if ($releaseId -notmatch '^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$') { throw "invalid release version $releaseId" }
if ($ReleaseVersion) {
    $tagCommit = (& git -C $repo rev-parse --verify "refs/tags/$ReleaseVersion^{commit}" 2>$null).Trim()
    if ($LASTEXITCODE -ne 0 -or $tagCommit -ne $sourceCommit) { throw "release tag $ReleaseVersion does not resolve to HEAD $sourceCommit" }
}
if ($RequireSigned -and -not $CertificateThumbprint) { throw '-RequireSigned requires -CertificateThumbprint' }

if (-not $PayloadArchive) {
    if (-not $WslDistribution -or -not $WslUser) {
        throw 'provide -PayloadArchive or both -WslDistribution and -WslUser'
    }
    $wslRepo = (& wsl.exe --distribution $WslDistribution --user $WslUser --exec /usr/bin/wslpath -a $repo 2>&1)
    if ($LASTEXITCODE -ne 0) { throw "could not translate repository path into WSL: $($wslRepo -join ' ')" }
    $wslRepo = (($wslRepo | ForEach-Object { $_.ToString() }) -join '').Trim()
    $wslRelease = if ($ReleaseVersion) { $ReleaseVersion } else { '' }
    Invoke-Native 'wsl.exe' @('--distribution', $WslDistribution, '--user', $WslUser, '--exec', '/bin/bash', '-lc', 'cd "$1"; if [ -n "$2" ]; then export CATS_RELEASE_VERSION="$2"; fi; exec make wsl-payload-dist', 'cats-build', $wslRepo, $wslRelease)
    $PayloadArchive = Join-Path $repo "dist\cats-wsl-payload_${releaseId}_ubuntu-24.04_linux_amd64.tar.gz"
}
$PayloadArchive = (Resolve-Path -LiteralPath $PayloadArchive).Path
if ((Split-Path -Leaf $PayloadArchive) -ne "cats-wsl-payload_${releaseId}_ubuntu-24.04_linux_amd64.tar.gz") {
    throw "payload archive name does not match Windows release $releaseId and Ubuntu 24.04 floor"
}

$rootName = "cats_${releaseId}_windows_amd64_wsl_ubuntu-24.04_amd64"
$stage = Join-Path $OutputDirectory ('.' + $rootName + '.stage')
$final = Join-Path $OutputDirectory $rootName
$zip = Join-Path $OutputDirectory ($rootName + '.zip')
Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $final -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $zip -Force -ErrorAction SilentlyContinue
[IO.Directory]::CreateDirectory((Join-Path $stage 'licenses')) | Out-Null

try {
    $launcher = Join-Path $stage 'Cats.exe'
    $launcherArgs = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $PSScriptRoot 'Build-WindowsLauncher.ps1'), '-OutputPath', $launcher)
    if ($CertificateThumbprint) { $launcherArgs += @('-CertificateThumbprint', $CertificateThumbprint, '-TimestampServer', $TimestampServer) }
    Invoke-Native 'powershell.exe' $launcherArgs

    Copy-Item -LiteralPath $PayloadArchive -Destination (Join-Path $stage 'wsl-payload.tar.gz')
    foreach ($name in @('Install-Cats.ps1', 'Uninstall-Cats.ps1', 'CatsInstaller.psm1')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $stage $name)
    }
    Copy-Item -LiteralPath (Join-Path $repo 'config.example.yaml') -Destination $stage
    Copy-Item -LiteralPath (Join-Path $repo 'NOTICE') -Destination $stage
    Copy-Item -LiteralPath (Join-Path $repo 'third_party\libghostty-vt\LICENSE') -Destination (Join-Path $stage 'licenses\libghostty-vt.txt')
    Copy-Item -LiteralPath (Join-Path $repo 'third_party\webview_go\LICENSE') -Destination (Join-Path $stage 'licenses\webview-go.txt')
    Copy-Item -LiteralPath (Join-Path $repo 'third_party\webview_go\libs\webview\LICENSE') -Destination (Join-Path $stage 'licenses\webview.txt')
    Copy-Item -LiteralPath (Join-Path $repo 'third_party\webview_go\libs\mswebview2\LICENSE') -Destination (Join-Path $stage 'licenses\mswebview2.txt')

    if ($CertificateThumbprint) {
        $certificate = Get-ChildItem Cert:\CurrentUser\My | Where-Object Thumbprint -eq $CertificateThumbprint | Select-Object -First 1
        if (-not $certificate) { throw "signing certificate $CertificateThumbprint was not found in CurrentUser\My" }
        foreach ($name in @('Install-Cats.ps1', 'Uninstall-Cats.ps1', 'CatsInstaller.psm1')) {
            $signature = Set-AuthenticodeSignature -FilePath (Join-Path $stage $name) -Certificate $certificate -TimestampServer $TimestampServer -HashAlgorithm SHA256
            if ($signature.Status -ne 'Valid') { throw "$name signing failed: $($signature.StatusMessage)" }
        }
    }

    $launcherSignatureObject = Get-AuthenticodeSignature -FilePath $launcher
    $launcherSignature = $launcherSignatureObject.Status.ToString()
    $installerSignatures = [ordered]@{}
    foreach ($name in @('Install-Cats.ps1', 'Uninstall-Cats.ps1', 'CatsInstaller.psm1')) {
        $installerSignatures[$name] = (Get-AuthenticodeSignature -FilePath (Join-Path $stage $name)).Status.ToString()
    }
    $installerSigned = @($installerSignatures.Values | Where-Object { $_ -ne 'Valid' }).Count -eq 0
    $releaseCandidateSigned = ($launcherSignature -eq 'Valid' -and $installerSigned)
    if ($RequireSigned -and -not $releaseCandidateSigned) { throw 'release candidate contains an invalid or unsigned executable/installer file' }
    $payloadDigest = (Get-FileHash -LiteralPath $PayloadArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    $launcherDigest = (Get-FileHash -LiteralPath $launcher -Algorithm SHA256).Hash.ToLowerInvariant()
    $epoch = (& git -C $repo show -s --format=%cI HEAD).Trim()
    $release = [ordered]@{
        schema = 2
        product = 'cats-windows-wsl'
        release_id = $releaseId
        compatibility_version = $hash
        source_commit = $sourceCommit
        architecture = 'amd64'
        windows_architecture = 'amd64'
        wsl_architecture = 'amd64'
        supported_distribution = [ordered]@{ id = 'ubuntu'; version = '24.04' }
        payload_archive = 'wsl-payload.tar.gz'
        built_at = $epoch
        minimum_windows_build = 26200
        minimum_wsl_version = '2.7.11.0'
        minimum_webview2 = '151.0.4129.72'
        authenticode = [ordered]@{
            launcher = $launcherSignature
            installer = $installerSignatures
            signer_subject = if ($launcherSignatureObject.SignerCertificate) { $launcherSignatureObject.SignerCertificate.Subject } else { $null }
            signer_thumbprint = if ($launcherSignatureObject.SignerCertificate) { $launcherSignatureObject.SignerCertificate.Thumbprint } else { $null }
            timestamp_server = if ($CertificateThumbprint) { $TimestampServer } else { $null }
            release_candidate_signed = $releaseCandidateSigned
        }
        components = [ordered]@{
            launcher = [ordered]@{ file = 'Cats.exe'; os = 'windows'; architecture = 'amd64'; sha256 = $launcherDigest; compatibility_version = $hash }
            payload = [ordered]@{ file = 'wsl-payload.tar.gz'; os = 'linux'; architecture = 'amd64'; distribution_floor = 'ubuntu-24.04'; sha256 = $payloadDigest; compatibility_version = $hash }
        }
    }
    [IO.File]::WriteAllText((Join-Path $stage 'release.json'), ($release | ConvertTo-Json -Depth 5), (New-Object Text.UTF8Encoding($false)))

    $checksumLines = New-Object System.Collections.Generic.List[string]
    Get-ChildItem -LiteralPath $stage -Recurse -File | Sort-Object FullName | ForEach-Object {
        $relative = $_.FullName.Substring($stage.Length + 1).Replace('\', '/')
        $digest = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $checksumLines.Add("$digest  $relative")
    }
    [IO.File]::WriteAllLines((Join-Path $stage 'SHA256SUMS'), $checksumLines, (New-Object Text.UTF8Encoding($false)))
    Move-Item -LiteralPath $stage -Destination $final
    Compress-Archive -LiteralPath $final -DestinationPath $zip -CompressionLevel Optimal
    Write-Host "Built $zip (launcher: $launcherSignature; installers signed: $installerSigned)"
}
catch {
    Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    throw
}
