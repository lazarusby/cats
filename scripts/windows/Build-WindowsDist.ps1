[CmdletBinding()]
param(
    [string]$PayloadArchive,
    [string]$WslDistribution,
    [string]$WslUser,
    [string]$OutputDirectory,
    [string]$CertificateThumbprint,
    [string]$TimestampServer = 'http://timestamp.digicert.com'
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
$hash = (& git -C $repo rev-parse --short HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $hash -notmatch '^[0-9a-f]{7,40}$') { throw 'could not determine release id' }

if (-not $PayloadArchive) {
    if (-not $WslDistribution -or -not $WslUser) {
        throw 'provide -PayloadArchive or both -WslDistribution and -WslUser'
    }
    $wslRepo = (& wsl.exe --distribution $WslDistribution --user $WslUser --exec /usr/bin/wslpath -a $repo 2>&1)
    if ($LASTEXITCODE -ne 0) { throw "could not translate repository path into WSL: $($wslRepo -join ' ')" }
    $wslRepo = (($wslRepo | ForEach-Object { $_.ToString() }) -join '').Trim()
    Invoke-Native 'wsl.exe' @('--distribution', $WslDistribution, '--user', $WslUser, '--exec', '/bin/bash', '-lc', 'cd "$1"; exec make wsl-payload-dist', 'cats-build', $wslRepo)
    $PayloadArchive = Join-Path $repo "dist\cats-wsl-payload_${hash}_linux_amd64.tar.gz"
}
$PayloadArchive = (Resolve-Path -LiteralPath $PayloadArchive).Path
if ((Split-Path -Leaf $PayloadArchive) -ne "cats-wsl-payload_${hash}_linux_amd64.tar.gz") {
    throw "payload archive name does not match Windows release $hash"
}

$rootName = "cats_${hash}_windows_wsl_amd64"
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

    $launcherSignature = (Get-AuthenticodeSignature -FilePath $launcher).Status.ToString()
    $installerSignature = (Get-AuthenticodeSignature -FilePath (Join-Path $stage 'Install-Cats.ps1')).Status.ToString()
    $epoch = (& git -C $repo show -s --format=%cI HEAD).Trim()
    $release = [ordered]@{
        schema = 1
        product = 'cats-windows-wsl'
        release_id = $hash
        compatibility_version = $hash
        architecture = 'amd64'
        payload_archive = 'wsl-payload.tar.gz'
        built_at = $epoch
        minimum_windows_build = 26200
        minimum_wsl_version = '2.7.11.0'
        minimum_webview2 = '151.0.4129.72'
        authenticode = [ordered]@{
            launcher = $launcherSignature
            installer = $installerSignature
            release_candidate_signed = ($launcherSignature -eq 'Valid' -and $installerSignature -eq 'Valid')
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
    Write-Host "Built $zip (launcher: $launcherSignature; installer: $installerSignature)"
}
catch {
    Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
    throw
}
