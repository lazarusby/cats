[CmdletBinding()]
param(
    [string]$OutputPath,
    [string]$CertificateThumbprint,
    [string]$TimestampServer = 'http://timestamp.digicert.com'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Invoke-Native {
    param([string]$FilePath, [string[]]$Arguments)
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath exited with code $LASTEXITCODE"
    }
}

function Find-Tool {
    param([string]$Name, [string[]]$Candidates)
    $command = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($command) { return $command.Source }
    foreach ($candidate in $Candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }
    throw "$Name was not found"
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if (-not $OutputPath) { $OutputPath = Join-Path $repo 'bin\Cats.exe' }
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
$outputDir = Split-Path -Parent $OutputPath
[IO.Directory]::CreateDirectory($outputDir) | Out-Null

$go = Find-Tool 'go.exe' @('C:\Program Files\Go\bin\go.exe')
$gcc = Find-Tool 'gcc.exe' @('C:\msys64\ucrt64\bin\gcc.exe')
$windres = Find-Tool 'windres.exe' @('C:\msys64\ucrt64\bin\windres.exe')
$toolDir = Split-Path -Parent $gcc
$env:PATH = "$toolDir;$(Split-Path -Parent $go);$env:PATH"
$env:CGO_ENABLED = '1'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CC = $gcc
$env:CXX = Join-Path $toolDir 'g++.exe'

Push-Location $repo
$resource = Join-Path $repo 'cmd\catapp\cats_windows_amd64.syso'
$work = Join-Path $outputDir '.cats-windows-resource'
try {
    [IO.Directory]::CreateDirectory($work) | Out-Null
    $icon = Join-Path $work 'Cats.ico'
    Invoke-Native $go @('run', '.\scripts\windows\gen-icon.go', '-output', $icon)

    $hash = (& git rev-parse --short HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $hash -notmatch '^[0-9a-f]{7,40}$') { throw 'could not determine git build hash' }
    $subject = (& git log -1 --pretty=%s).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'could not determine git subject' }
    $subjectB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($subject))
    $productVersion = (& git describe --tags --always --dirty).Trim()
    if ($LASTEXITCODE -ne 0) { $productVersion = $hash }
    $productVersion = $productVersion.Replace('"', "'").Replace("`r", '').Replace("`n", '')
    $commitCount = [int]((& git rev-list --count HEAD).Trim())
    $fileVersion = "0,0,$([Math]::Min(65535, [Math]::Floor($commitCount / 65536))),$($commitCount % 65536)"

    $manifest = (Resolve-Path '.\scripts\windows\cats.exe.manifest').Path.Replace('\', '\\')
    $iconRC = $icon.Replace('\', '\\')
    $template = Get-Content -LiteralPath '.\scripts\windows\cats.rc.in' -Raw
    $rc = $template.Replace('@ICON_PATH@', $iconRC).
        Replace('@MANIFEST_PATH@', $manifest).
        Replace('@FILE_VERSION@', $fileVersion).
        Replace('@PRODUCT_VERSION@', $productVersion)
    $rcPath = Join-Path $work 'cats.rc'
    [IO.File]::WriteAllText($rcPath, $rc, (New-Object Text.UTF8Encoding($false)))
    Invoke-Native $windres @('--input', $rcPath, '--output', $resource, '--output-format=coff')

    $stampPackage = 'github.com/rohanthewiz/cats/internal/buildinfo'
    $ldflags = "-H windowsgui -X main.defaultMode=local -X $stampPackage.hash=$hash -X $stampPackage.subjectB64=$subjectB64"
    Invoke-Native $go @('build', '-trimpath', '-ldflags', $ldflags, '-o', $OutputPath, '.\cmd\catapp')

    if ($CertificateThumbprint) {
        $certificate = Get-ChildItem Cert:\CurrentUser\My | Where-Object Thumbprint -eq $CertificateThumbprint | Select-Object -First 1
        if (-not $certificate) { throw "signing certificate $CertificateThumbprint was not found in CurrentUser\My" }
        $signature = Set-AuthenticodeSignature -FilePath $OutputPath -Certificate $certificate -TimestampServer $TimestampServer -HashAlgorithm SHA256
        if ($signature.Status -ne 'Valid') { throw "Cats.exe signing failed: $($signature.StatusMessage)" }
    }
    $status = (Get-AuthenticodeSignature -FilePath $OutputPath).Status
    Write-Host "Built $OutputPath (Authenticode: $status)"
}
finally {
    Remove-Item -LiteralPath $resource -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    Pop-Location
}
