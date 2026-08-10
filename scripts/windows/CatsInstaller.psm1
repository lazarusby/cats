Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$script:CatsWslExecutable = 'wsl.exe'

function Assert-CatsTextArgument {
    param([string]$Name, [string]$Value, [int]$Maximum = 256)
    if ([string]::IsNullOrWhiteSpace($Value) -or $Value -ne $Value.Trim()) { throw "$Name is required and must not have surrounding whitespace" }
    if ($Value.Length -gt $Maximum -or $Value.IndexOf([char]0) -ge 0 -or $Value -match '[\x00-\x1f\x7f]') { throw "$Name contains invalid characters or is too long" }
}

function Assert-CatsLinuxPath {
    param([string]$Name, [string]$Value)
    if ($Value -notmatch '^/' -or $Value -match '[\x00-\x1f\x7f]' -or $Value.Length -gt 4096) { throw "$Name is not a safe absolute Linux path" }
    if ($Value -eq '/' -or $Value -eq '/mnt' -or $Value.StartsWith('/mnt/')) { throw "$Name must be inside the WSL filesystem" }
}

function Assert-CatsWsl2Kernel {
    param([string]$KernelRelease)
    if ($KernelRelease -notmatch '(?i)microsoft.*wsl2') {
        throw 'the selected distribution is not running under WSL2'
    }
}

function Assert-CatsSupportedDistribution {
    param([string]$OsRelease)
    if ($OsRelease -notmatch '(?m)^ID="?ubuntu"?\s*$' -or $OsRelease -notmatch '(?m)^VERSION_ID="?24\.04"?\s*$') {
        throw 'the initial CATS payload supports only Ubuntu 24.04 LTS under WSL2'
    }
}

function Get-CatsCommandLinkSpecs {
    param([string]$PayloadRoot)
    Assert-CatsLinuxPath 'payload root' $PayloadRoot
    $suffix = '/.local/lib/cats'
    if (-not $PayloadRoot.EndsWith($suffix, [StringComparison]::Ordinal)) { throw 'refusing unexpected WSL payload root' }
    $linuxHome = $PayloadRoot.Substring(0, $PayloadRoot.Length - $suffix.Length)
    Assert-CatsLinuxPath 'Linux home' $linuxHome
    foreach ($name in @('catway', 'cathost', 'catctl')) {
        [pscustomobject]@{
            Path = "$linuxHome/.local/bin/$name"
            Target = "$PayloadRoot/current/$name"
        }
    }
}

function Invoke-CatsWsl {
    param(
        [string]$Distribution,
        [string]$User,
        [string]$Command,
        [string[]]$Arguments = @(),
        [string]$WorkingDirectory,
        [switch]$AllowFailure
    )
    Assert-CatsTextArgument 'distribution' $Distribution 128
    Assert-CatsTextArgument 'Linux user' $User 256
    $wslArguments = @('--distribution', $Distribution, '--user', $User)
    if ($WorkingDirectory) {
        Assert-CatsLinuxPath 'working directory' $WorkingDirectory
        $wslArguments += @('--cd', $WorkingDirectory)
    }
    $wslArguments += @('--exec', $Command)
    $wslArguments += $Arguments
    $output = & $script:CatsWslExecutable @wslArguments 2>&1
    $exitCode = $LASTEXITCODE
    $text = (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "WSL command $Command failed with exit code $exitCode`: $text"
    }
    [pscustomobject]@{ ExitCode = $exitCode; Output = $text; Arguments = $wslArguments }
}

function Test-CatsPackageChecksums {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$PackageRoot)
    $root = (Resolve-Path -LiteralPath $PackageRoot).Path
    $manifest = Join-Path $root 'SHA256SUMS'
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) { throw 'package SHA256SUMS is missing' }
    $seen = @{}
    foreach ($line in Get-Content -LiteralPath $manifest) {
        if (-not $line) { continue }
        if ($line -notmatch '^([0-9a-fA-F]{64}) [ *](.+)$') { throw "malformed checksum line: $line" }
        $expected = $Matches[1].ToLowerInvariant()
        $relative = $Matches[2].Replace('/', [IO.Path]::DirectorySeparatorChar)
        if ([IO.Path]::IsPathRooted($relative) -or $relative -match '(^|[\\/])\.\.([\\/]|$)' -or $relative.IndexOf(':') -ge 0) {
            throw "unsafe checksum path: $relative"
        }
        $path = [IO.Path]::GetFullPath((Join-Path $root $relative))
        $prefix = $root.TrimEnd('\') + '\'
        if (-not $path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) { throw "checksum path escapes package: $relative" }
        if ($seen.ContainsKey($path)) { throw "duplicate checksum path: $relative" }
        $seen[$path] = $true
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "package file is missing: $relative" }
        $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) { throw "checksum mismatch: $relative" }
    }
    if ($seen.Count -eq 0) { throw 'package checksum manifest is empty' }
    return $true
}

function Test-CatsPackageLayout {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$PackageRoot)
    $root = (Resolve-Path -LiteralPath $PackageRoot).Path
    $expectedFiles = @(
        'Cats.exe', 'CatsInstaller.psm1', 'config.example.yaml',
        'Install-Cats.ps1', 'NOTICE', 'release.json', 'SHA256SUMS',
        'Uninstall-Cats.ps1', 'wsl-payload.tar.gz',
        'licenses/libghostty-vt.txt', 'licenses/mswebview2.txt',
        'licenses/webview-go.txt', 'licenses/webview.txt'
    )
    $actualFiles = @(Get-ChildItem -LiteralPath $root -Recurse -File | ForEach-Object {
        if (($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "package file must not be a link or reparse point: $($_.FullName)"
        }
        $_.FullName.Substring($root.Length + 1).Replace('\', '/')
    } | Sort-Object)
    $actualDirectories = @(Get-ChildItem -LiteralPath $root -Recurse -Directory | ForEach-Object {
        if (($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "package directory must not be a link or reparse point: $($_.FullName)"
        }
        $_.FullName.Substring($root.Length + 1).Replace('\', '/')
    } | Sort-Object)
    if (($actualFiles -join "`n") -cne (($expectedFiles | Sort-Object) -join "`n")) {
        throw "package contains missing or unexpected files: $($actualFiles -join ', ')"
    }
    if (($actualDirectories -join "`n") -cne 'licenses') {
        throw "package contains missing or unexpected directories: $($actualDirectories -join ', ')"
    }
    return $true
}

function Assert-CatsPayloadArchiveListing {
    param([string[]]$Listing, [string]$ReleaseId)
    if ($ReleaseId -notmatch '^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$') { throw 'unsafe payload release id' }
    $root = "cats-wsl-payload_${ReleaseId}_ubuntu-24.04_linux_amd64"
    $expected = @{
        "$root/" = 'd'
        "$root/NOTICE" = '-'
        "$root/SHA256SUMS" = '-'
        "$root/catctl" = '-'
        "$root/cathost" = '-'
        "$root/cats-wsl-host" = '-'
        "$root/catway" = '-'
        "$root/config.example.yaml" = '-'
        "$root/licenses/" = 'd'
        "$root/licenses/libghostty-vt.txt" = '-'
        "$root/release.json" = '-'
    }
    $seen = @{}
    foreach ($line in $Listing) {
        if (-not $line.Trim()) { continue }
        $fields = @($line.Trim() -split '\s+')
        if ($fields.Count -lt 2 -or -not $fields[0]) { throw "malformed payload archive listing: $line" }
        $kind = $fields[0].Substring(0, 1)
        $path = $fields[$fields.Count - 1].Replace('\', '/')
        if (-not $expected.ContainsKey($path) -or $expected[$path] -cne $kind) {
            throw "unsafe or unexpected payload archive entry: $path ($kind)"
        }
        if ($seen.ContainsKey($path)) { throw "duplicate payload archive entry: $path" }
        $seen[$path] = $true
    }
    if ($seen.Count -ne $expected.Count) { throw 'payload archive is missing required regular files or directories' }
    foreach ($path in $expected.Keys) {
        if (-not $seen.ContainsKey($path)) { throw "payload archive is missing required entry: $path" }
    }
}

function Read-CatsRelease {
    param([string]$PackageRoot)
    $path = Join-Path $PackageRoot 'release.json'
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'package release.json is missing' }
    $release = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    if (($release.schema -ne 1 -and $release.schema -ne 2) -or $release.product -ne 'cats-windows-wsl') { throw 'unsupported CATS package metadata' }
    if ([string]$release.release_id -notmatch '^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$') { throw 'release_id is not a safe version identifier' }
    if ([string]$release.compatibility_version -notmatch '^[0-9a-f]{7,40}$') { throw 'compatibility_version must be a git hash' }
    if ($release.schema -eq 1 -and $release.compatibility_version -ne $release.release_id) { throw 'legacy package compatibility version does not match release id' }
    if ($release.architecture -ne 'amd64' -or $release.payload_archive -ne 'wsl-payload.tar.gz') { throw 'package is not the supported Windows/WSL amd64 layout' }
    if ($release.schema -eq 2) {
        if ([string]$release.source_commit -notmatch '^[0-9a-f]{40}$' -or -not ([string]$release.source_commit).StartsWith([string]$release.compatibility_version)) { throw 'source commit does not match compatibility version' }
        if ($release.windows_architecture -ne 'amd64' -or $release.wsl_architecture -ne 'amd64') { throw 'package architecture pair is unsupported' }
        if ($release.supported_distribution.id -ne 'ubuntu' -or $release.supported_distribution.version -ne '24.04') { throw 'package distribution floor is unsupported' }
        if ($release.components.launcher.file -ne 'Cats.exe' -or $release.components.payload.file -ne 'wsl-payload.tar.gz') { throw 'package component mapping is unsupported' }
        if ($release.components.launcher.compatibility_version -ne $release.compatibility_version -or $release.components.payload.compatibility_version -ne $release.compatibility_version) { throw 'package component compatibility mapping is inconsistent' }
    }
    return $release
}

function Test-CatsPrerequisites {
    param($Release)
    if (-not [Environment]::Is64BitOperatingSystem -or -not [Environment]::Is64BitProcess) { throw 'CATS requires 64-bit PowerShell on Windows amd64' }
    if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() -ne 'X64') { throw 'this package supports only Windows amd64' }
    $build = [int](Get-ItemPropertyValue 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' 'CurrentBuildNumber')
    if ($build -lt [int]$Release.minimum_windows_build) { throw "Windows build $build is below the qualified floor $($Release.minimum_windows_build)" }
    if (-not (Get-Command $script:CatsWslExecutable -ErrorAction SilentlyContinue)) { throw 'wsl.exe was not found' }
    $wslVersion = & $script:CatsWslExecutable --version 2>&1
    if ($LASTEXITCODE -ne 0) { throw "wsl.exe --version failed: $($wslVersion -join ' ')" }
    $wslText = (($wslVersion | ForEach-Object { $_.ToString() }) -join "`n").Replace([string][char]0, '')
    if ($wslText -notmatch '(?m)^\D*([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)') { throw 'could not parse the Store WSL version' }
    if ([version]$Matches[1] -lt [version]$Release.minimum_wsl_version) { throw "WSL $($Matches[1]) is below the qualified floor $($Release.minimum_wsl_version)" }
    $webViewId = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    $paths = @(
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$webViewId",
        "HKCU:\Software\Microsoft\EdgeUpdate\Clients\$webViewId"
    )
    $versions = @($paths | ForEach-Object {
        $item = Get-ItemProperty -Path $_ -Name pv -ErrorAction SilentlyContinue
        if ($item -and $item.PSObject.Properties['pv']) { $item.pv }
    } | Where-Object { $_ -and $_ -ne '0.0.0.0' })
    if ($versions.Count -eq 0) { throw 'Evergreen WebView2 Runtime is not installed' }
    $qualifiedWebView = @($versions | Where-Object { [version]$_ -ge [version]$Release.minimum_webview2 })
    if ($qualifiedWebView.Count -eq 0) { throw "WebView2 is below the qualified floor $($Release.minimum_webview2)" }
}

function Get-CatsDistributions {
    $output = & $script:CatsWslExecutable --list --quiet 2>&1
    if ($LASTEXITCODE -ne 0) { throw "could not list WSL distributions: $($output -join ' ')" }
    $items = @($output | ForEach-Object { $_.ToString().Replace([string][char]0, '').Trim([char]0xfeff, ' ', "`r", "`n", "`t") } | Where-Object { $_ } | Select-Object -Unique)
    if ($items.Count -eq 0) { throw 'WSL reported no installed distributions' }
    return $items
}

function Resolve-CatsTarget {
    param([string]$Distribution, [string]$User)
    $distributions = @(Get-CatsDistributions)
    if (-not $Distribution) {
        if ($distributions.Count -eq 1) { $Distribution = $distributions[0] }
        else {
            Write-Host 'Installed WSL distributions:'
            for ($i = 0; $i -lt $distributions.Count; $i++) { Write-Host "  [$($i + 1)] $($distributions[$i])" }
            $selection = [int](Read-Host 'Select a distribution number')
            if ($selection -lt 1 -or $selection -gt $distributions.Count) { throw 'invalid distribution selection' }
            $Distribution = $distributions[$selection - 1]
        }
    }
    if ($distributions -notcontains $Distribution) { throw "distribution $Distribution is not installed" }
    if (-not $User) {
        $result = Invoke-CatsWsl $Distribution 'root' '/usr/bin/id' @('-un')
        $User = $result.Output
        if ($User -eq 'root') {
            $default = & $script:CatsWslExecutable --distribution $Distribution --exec /usr/bin/id -un 2>&1
            if ($LASTEXITCODE -ne 0) { throw 'could not determine the distribution default user' }
            $User = (($default | ForEach-Object { $_.ToString() }) -join '').Trim()
        }
    }
    Assert-CatsTextArgument 'Linux user' $User 256
    $identity = Invoke-CatsWsl $Distribution $User '/usr/bin/id' @('-u')
    if ($identity.Output -eq '0') { throw 'installing the CATS payload as root is not supported' }
    $kernel = Invoke-CatsWsl $Distribution $User '/bin/cat' @('/proc/sys/kernel/osrelease')
    Assert-CatsWsl2Kernel $kernel.Output
    $architecture = Invoke-CatsWsl $Distribution $User '/bin/uname' @('-m')
    if ($architecture.Output -ne 'x86_64') { throw "WSL payload requires x86_64, got $($architecture.Output)" }
    $home = Invoke-CatsWsl $Distribution $User '/bin/sh' @('-lc', 'printf "%s" "$HOME"')
    Assert-CatsLinuxPath 'Linux home' $home.Output
    [pscustomobject]@{ Distribution = $Distribution; User = $User; Home = $home.Output }
}

function Save-CatsAppConfig {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Distribution,
        [Parameter(Mandatory = $true)][string]$User,
        [Parameter(Mandatory = $true)][string]$PayloadPath
    )
    $config = New-Object psobject
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        try { $config = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
        catch { throw "existing app.json is malformed; it was not changed: $($_.Exception.Message)" }
    }
    foreach ($property in @(
        @{ Name = 'mode'; Value = 'local' },
        @{ Name = 'wsl'; Value = [pscustomobject]@{ distribution = $Distribution; user = $User; payload_path = $PayloadPath } }
    )) {
        if ($config.PSObject.Properties[$property.Name]) { $config.($property.Name) = $property.Value }
        else { $config | Add-Member -NotePropertyName $property.Name -NotePropertyValue $property.Value }
    }
    $directory = Split-Path -Parent $Path
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    $temporary = Join-Path $directory ('.app-' + [Guid]::NewGuid().ToString('N') + '.json')
    [IO.File]::WriteAllText($temporary, ($config | ConvertTo-Json -Depth 12), (New-Object Text.UTF8Encoding($false)))
    if (Test-Path -LiteralPath $Path) {
        $replacementBackup = Join-Path $directory ('.app-backup-' + [Guid]::NewGuid().ToString('N') + '.json')
        try { [IO.File]::Replace($temporary, $Path, $replacementBackup) }
        finally { Remove-Item -LiteralPath $replacementBackup -Force -ErrorAction SilentlyContinue }
    }
    else { [IO.File]::Move($temporary, $Path) }
    return $config
}

function Restore-CatsAppConfigBytes {
    param([string]$Path, [byte[]]$Bytes)
    if ($null -eq $Bytes) {
        Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
        return
    }
    $directory = Split-Path -Parent $Path
    $temporary = Join-Path $directory ('.app-rollback-' + [Guid]::NewGuid().ToString('N') + '.json')
    $replacementBackup = Join-Path $directory ('.app-rollback-backup-' + [Guid]::NewGuid().ToString('N') + '.json')
    [IO.File]::WriteAllBytes($temporary, $Bytes)
    try { [IO.File]::Replace($temporary, $Path, $replacementBackup) }
    finally {
        Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $replacementBackup -Force -ErrorAction SilentlyContinue
    }
}

function Get-CatsShortcut {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($Path)
    [pscustomobject]@{ TargetPath = $shortcut.TargetPath; Arguments = $shortcut.Arguments; WorkingDirectory = $shortcut.WorkingDirectory; IconLocation = $shortcut.IconLocation }
}

function Set-CatsShortcut {
    param([string]$Path, [string]$TargetPath, [string]$Arguments = '', [string]$WorkingDirectory = '', [string]$IconLocation = '')
    [IO.Directory]::CreateDirectory((Split-Path -Parent $Path)) | Out-Null
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($Path)
    $shortcut.TargetPath = $TargetPath
    $shortcut.Arguments = $Arguments
    $shortcut.WorkingDirectory = $WorkingDirectory
    $shortcut.IconLocation = $IconLocation
    $shortcut.Save()
}

function Restore-CatsShortcut {
    param([string]$Path, $Previous)
    if ($null -eq $Previous) { Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue; return }
    Set-CatsShortcut $Path $Previous.TargetPath $Previous.Arguments $Previous.WorkingDirectory $Previous.IconLocation
}

function Remove-CatsWslTree {
    param([string]$Distribution, [string]$User, [string]$Path)
    Assert-CatsLinuxPath 'remove target' $Path
    Invoke-CatsWsl $Distribution $User '/bin/rm' @('-rf', '--', $Path) | Out-Null
}

function Invoke-CatsInstall {
    [CmdletBinding()]
    param(
        [string]$PackageRoot = $PSScriptRoot,
        [string]$Distribution,
        [string]$User,
        [switch]$DesktopShortcut,
        [switch]$NoCommandLinks,
        [switch]$NoLaunch,
        [switch]$SkipPrerequisiteCheck,
        [string]$PayloadRootOverride,
        [string]$LocalAppDataOverride,
        [string]$StartMenuOverride,
        [string]$DesktopOverride
    )
    $PackageRoot = (Resolve-Path -LiteralPath $PackageRoot).Path
    Test-CatsPackageChecksums $PackageRoot | Out-Null
    Test-CatsPackageLayout $PackageRoot | Out-Null
    $release = Read-CatsRelease $PackageRoot
    if (-not $SkipPrerequisiteCheck) { Test-CatsPrerequisites $release }
    $target = Resolve-CatsTarget $Distribution $User
    $osRelease = Invoke-CatsWsl $target.Distribution $target.User '/bin/cat' @('/etc/os-release')
    Assert-CatsSupportedDistribution $osRelease.Output
    $releaseId = [string]$release.release_id
    $payloadRoot = if ($PayloadRootOverride) { $PayloadRootOverride } else { "$($target.Home)/.local/lib/cats" }
    Assert-CatsLinuxPath 'payload root' $payloadRoot
    $payloadCurrent = "$payloadRoot/current"
    $payloadRelease = "$payloadRoot/$releaseId"
    $token = [Guid]::NewGuid().ToString('N')
    $payloadStage = "$payloadRoot/.stage-$token"
    $payloadBackup = "$payloadRoot/.backup-$token"

    $localAppData = if ($LocalAppDataOverride) { [IO.Path]::GetFullPath($LocalAppDataOverride) } else { [Environment]::GetFolderPath('LocalApplicationData') }
    if (-not $localAppData) { throw 'LocalAppData is unavailable' }
    $installRoot = Join-Path $localAppData 'Programs\Cats'
    $versionsRoot = Join-Path $installRoot 'versions'
    $windowsRelease = Join-Path $versionsRoot $releaseId
    $windowsStage = Join-Path $versionsRoot ('.stage-' + $token)
    $windowsBackup = Join-Path $versionsRoot ('.backup-' + $token)
    $configPath = Join-Path $localAppData 'Cats\app.json'
    $startMenuRoot = if ($StartMenuOverride) { [IO.Path]::GetFullPath($StartMenuOverride) } else { [Environment]::GetFolderPath('StartMenu') }
    $desktopRoot = if ($DesktopOverride) { [IO.Path]::GetFullPath($DesktopOverride) } else { [Environment]::GetFolderPath('Desktop') }
    $startShortcut = Join-Path $startMenuRoot 'Programs\Cats.lnk'
    $desktopLink = Join-Path $desktopRoot 'Cats.lnk'
    $logDir = Join-Path $localAppData 'Cats\logs'
    [IO.Directory]::CreateDirectory($logDir) | Out-Null
    $logPath = Join-Path $logDir 'installer.log'
    Add-Content -LiteralPath $logPath -Value "[$([DateTime]::UtcNow.ToString('o'))] staging release $releaseId for $($target.Distribution)/$($target.User)"

    $windowsHadRelease = $false
    $windowsBackedUp = $false
    $windowsReleaseReplaced = $false
    $payloadHadRelease = $false
    $payloadBackedUp = $false
    $payloadReleaseReplaced = $false
    $previousLink = $null
    $configBytes = $null
    $previousStart = $null
    $previousDesktop = $null
    $configChanged = $false
    $startShortcutChanged = $false
    $desktopShortcutChanged = $false
    $commandLinks = @()
    $commandLinksCreated = @()
    $committed = $false
    try {
        if (-not $NoCommandLinks) {
            $commandLinks = @(Get-CatsCommandLinkSpecs $payloadRoot)
            foreach ($link in $commandLinks) {
                $regularExists = (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/test' @('-e', $link.Path) -AllowFailure).ExitCode -eq 0
                $symlinkExists = (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/test' @('-L', $link.Path) -AllowFailure).ExitCode -eq 0
                if ($regularExists -or $symlinkExists) {
                    $existingTarget = Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/readlink' @($link.Path) -AllowFailure
                    if ($existingTarget.ExitCode -ne 0 -or $existingTarget.Output -ne $link.Target) {
                        throw "refusing to replace non-CATS command path $($link.Path)"
                    }
                }
            }
        }
        [IO.Directory]::CreateDirectory($versionsRoot) | Out-Null
        Copy-Item -LiteralPath $PackageRoot -Destination $windowsStage -Recurse
        $windowsHadRelease = Test-Path -LiteralPath $windowsRelease
        if ($windowsHadRelease) {
            Move-Item -LiteralPath $windowsRelease -Destination $windowsBackup
            $windowsBackedUp = $true
        }
        Move-Item -LiteralPath $windowsStage -Destination $windowsRelease
        $windowsReleaseReplaced = $true

        $archive = Join-Path $PackageRoot 'wsl-payload.tar.gz'
        $wslArchive = Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/wslpath' @('-a', $archive)
        Invoke-CatsWsl $target.Distribution $target.User '/bin/mkdir' @('-p', $payloadRoot, $payloadStage) | Out-Null
        Invoke-CatsWsl $target.Distribution $target.User '/bin/cp' @('--', $wslArchive.Output, "$payloadStage/payload.tar.gz") | Out-Null
        $copiedHash = (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/sha256sum' @("$payloadStage/payload.tar.gz")).Output.Split(' ')[0]
        $sourceHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($copiedHash.ToLowerInvariant() -ne $sourceHash) { throw 'WSL payload archive changed while crossing the Windows/WSL boundary' }
        $archiveListing = Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/env' @('LC_ALL=C', '/bin/tar', '-tvzf', "$payloadStage/payload.tar.gz")
        Assert-CatsPayloadArchiveListing ($archiveListing.Output -split "`n") $releaseId
        Invoke-CatsWsl $target.Distribution $target.User '/bin/tar' @('-xzf', "$payloadStage/payload.tar.gz", '-C', $payloadStage, '--strip-components=1', '--no-same-owner', '--no-same-permissions') | Out-Null
        Invoke-CatsWsl $target.Distribution $target.User '/bin/rm' @('-f', '--', "$payloadStage/payload.tar.gz") | Out-Null
        Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/sha256sum' @('--check', 'SHA256SUMS') $payloadStage | Out-Null
        Invoke-CatsWsl $target.Distribution $target.User '/bin/chmod' @('0755', "$payloadStage/catway", "$payloadStage/cathost", "$payloadStage/catctl", "$payloadStage/cats-wsl-host") | Out-Null
        $payloadMetadata = (Invoke-CatsWsl $target.Distribution $target.User '/bin/cat' @("$payloadStage/release.json")).Output | ConvertFrom-Json
        if ($payloadMetadata.release_id -ne $releaseId -or $payloadMetadata.compatibility_version -ne $release.compatibility_version -or $payloadMetadata.architecture -ne 'amd64') {
            throw 'Linux payload metadata does not match the Windows launcher package'
        }
        $health = (Invoke-CatsWsl $target.Distribution $target.User "$payloadStage/cats-wsl-host" @('--health-json')).Output | ConvertFrom-Json
        if ($health.helper_version -ne $release.compatibility_version -or $health.architecture -ne 'amd64' -or $health.payload_path -ne $payloadStage) {
            throw 'staged Linux payload failed its helper health identity check'
        }

        $previousLinkResult = Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/readlink' @($payloadCurrent) -AllowFailure
        if ($previousLinkResult.ExitCode -eq 0) { $previousLink = $previousLinkResult.Output }
        $payloadHadRelease = (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/test' @('-e', $payloadRelease) -AllowFailure).ExitCode -eq 0
        if (Test-Path -LiteralPath $configPath) { $configBytes = [IO.File]::ReadAllBytes($configPath) }
        $previousStart = Get-CatsShortcut $startShortcut
        $previousDesktop = Get-CatsShortcut $desktopLink

        if ($payloadHadRelease) {
            Invoke-CatsWsl $target.Distribution $target.User '/bin/mv' @($payloadRelease, $payloadBackup) | Out-Null
            $payloadBackedUp = $true
        }
        Invoke-CatsWsl $target.Distribution $target.User '/bin/mv' @($payloadStage, $payloadRelease) | Out-Null
        $payloadReleaseReplaced = $true
        $newLink = "$payloadRoot/.current-$token"
        Invoke-CatsWsl $target.Distribution $target.User '/bin/ln' @('-s', $releaseId, $newLink) | Out-Null
        Invoke-CatsWsl $target.Distribution $target.User '/bin/mv' @('-Tf', $newLink, $payloadCurrent) | Out-Null
        if (-not $NoCommandLinks) {
            foreach ($link in $commandLinks) {
                $linkDirectory = $link.Path.Substring(0, $link.Path.LastIndexOf('/'))
                Invoke-CatsWsl $target.Distribution $target.User '/bin/mkdir' @('-p', $linkDirectory) | Out-Null
                $exists = (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/test' @('-L', $link.Path) -AllowFailure).ExitCode -eq 0
                if (-not $exists) {
                    Invoke-CatsWsl $target.Distribution $target.User '/bin/ln' @('-s', $link.Target, $link.Path) | Out-Null
                    $commandLinksCreated += $link.Path
                }
            }
        }

        Save-CatsAppConfig $configPath $target.Distribution $target.User $payloadCurrent | Out-Null
        $configChanged = $true
        $executable = Join-Path $windowsRelease 'Cats.exe'
        Set-CatsShortcut $startShortcut $executable '' $windowsRelease "$executable,0"
        $startShortcutChanged = $true
        if ($DesktopShortcut) {
            Set-CatsShortcut $desktopLink $executable '' $windowsRelease "$executable,0"
            $desktopShortcutChanged = $true
        }

        $originalLocalAppData = $env:LOCALAPPDATA
        try {
            $env:LOCALAPPDATA = $localAppData
            $smoke = Start-Process -FilePath $executable -ArgumentList '--install-smoke' -Wait -PassThru
        }
        finally { $env:LOCALAPPDATA = $originalLocalAppData }
        if ($smoke.ExitCode -ne 0) { throw "installed launcher/backend smoke check exited $($smoke.ExitCode)" }

        $committed = $true
        if ($windowsBackedUp) { Remove-Item -LiteralPath $windowsBackup -Recurse -Force -ErrorAction SilentlyContinue }
        if ($payloadBackedUp) { Invoke-CatsWsl $target.Distribution $target.User '/bin/rm' @('-rf', '--', $payloadBackup) -AllowFailure | Out-Null }
        Add-Content -LiteralPath $logPath -Value "[$([DateTime]::UtcNow.ToString('o'))] committed release $releaseId"
        if (-not $NoLaunch) {
            try { Start-Process -FilePath $executable | Out-Null }
            catch { Add-Content -LiteralPath $logPath -Value "[$([DateTime]::UtcNow.ToString('o'))] launch after install failed: $_" }
        }
        [pscustomobject]@{ Release = $releaseId; Distribution = $target.Distribution; User = $target.User; WindowsPath = $windowsRelease; PayloadPath = $payloadCurrent; LogPath = $logPath }
    }
    catch {
        $failure = $_
        if ($committed) { throw }
        if ($configChanged) { Restore-CatsAppConfigBytes $configPath $configBytes }
        if ($startShortcutChanged) { Restore-CatsShortcut $startShortcut $previousStart }
        if ($desktopShortcutChanged) { Restore-CatsShortcut $desktopLink $previousDesktop }
        foreach ($linkPath in $commandLinksCreated) {
            Invoke-CatsWsl $target.Distribution $target.User '/bin/rm' @('-f', '--', $linkPath) -AllowFailure | Out-Null
        }
        if ($previousLink) {
            $rollbackLink = "$payloadRoot/.rollback-$token"
            Invoke-CatsWsl $target.Distribution $target.User '/bin/ln' @('-s', $previousLink, $rollbackLink) -AllowFailure | Out-Null
            Invoke-CatsWsl $target.Distribution $target.User '/bin/mv' @('-Tf', $rollbackLink, $payloadCurrent) -AllowFailure | Out-Null
        } else { Invoke-CatsWsl $target.Distribution $target.User '/bin/rm' @('-f', '--', $payloadCurrent) -AllowFailure | Out-Null }
        if ($payloadReleaseReplaced) { Remove-CatsWslTree $target.Distribution $target.User $payloadRelease }
        Remove-CatsWslTree $target.Distribution $target.User $payloadStage
        if ($payloadBackedUp) { Invoke-CatsWsl $target.Distribution $target.User '/bin/mv' @($payloadBackup, $payloadRelease) -AllowFailure | Out-Null }
        Remove-Item -LiteralPath $windowsStage -Recurse -Force -ErrorAction SilentlyContinue
        if ($windowsReleaseReplaced) { Remove-Item -LiteralPath $windowsRelease -Recurse -Force -ErrorAction SilentlyContinue }
        if ($windowsBackedUp -and (Test-Path -LiteralPath $windowsBackup)) { Move-Item -LiteralPath $windowsBackup -Destination $windowsRelease }
        Add-Content -LiteralPath $logPath -Value "[$([DateTime]::UtcNow.ToString('o'))] rolled back release $releaseId`: $failure"
        throw
    }
}

function Get-CatsUninstallPlan {
    [CmdletBinding()]
    param([string]$LocalAppData, [string]$StartMenu, [string]$Desktop, [string]$PayloadRoot, [switch]$RemoveAllData, [string]$ConfigRoot, [string]$StateRoot)
    Assert-CatsLinuxPath 'payload root' $PayloadRoot
    $links = @(Get-CatsCommandLinkSpecs $PayloadRoot)
    $paths = @($links | ForEach-Object {
        [pscustomobject]@{ Side = 'WSL'; Path = $_.Path; Kind = 'managed-link'; ExpectedTarget = $_.Target }
    })
    $paths += @(
        [pscustomobject]@{ Side = 'Windows'; Path = (Join-Path $LocalAppData 'Programs\Cats'); Kind = 'payload' },
        [pscustomobject]@{ Side = 'Windows'; Path = (Join-Path $StartMenu 'Programs\Cats.lnk'); Kind = 'shortcut' },
        [pscustomobject]@{ Side = 'Windows'; Path = (Join-Path $Desktop 'Cats.lnk'); Kind = 'shortcut' },
        [pscustomobject]@{ Side = 'WSL'; Path = $PayloadRoot; Kind = 'payload' }
    )
    if ($RemoveAllData) {
        $paths += [pscustomobject]@{ Side = 'Windows'; Path = (Join-Path $LocalAppData 'Cats'); Kind = 'data' }
        $paths += [pscustomobject]@{ Side = 'WSL'; Path = $ConfigRoot; Kind = 'data' }
        $paths += [pscustomobject]@{ Side = 'WSL'; Path = $StateRoot; Kind = 'data' }
    }
    return $paths
}

function Invoke-CatsUninstall {
    [CmdletBinding()]
    param(
        [string]$Distribution,
        [string]$User,
        [switch]$RemoveAllData,
        [switch]$Force,
        [string]$PayloadRootOverride,
        [string]$LocalAppDataOverride,
        [string]$StartMenuOverride,
        [string]$DesktopOverride
    )
    $localAppData = if ($LocalAppDataOverride) { [IO.Path]::GetFullPath($LocalAppDataOverride) } else { [Environment]::GetFolderPath('LocalApplicationData') }
    $configPath = Join-Path $localAppData 'Cats\app.json'
    if (Test-Path -LiteralPath $configPath) {
        $config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
        if (-not $Distribution) { $Distribution = [string]$config.wsl.distribution }
        if (-not $User) { $User = [string]$config.wsl.user }
    }
    if (-not $Distribution -or -not $User) { throw 'distribution and Linux user are required when app.json has no WSL target' }
    $target = Resolve-CatsTarget $Distribution $User
    $payloadRoot = if ($PayloadRootOverride) { $PayloadRootOverride } else { "$($target.Home)/.local/lib/cats" }
    $xdg = Invoke-CatsWsl $target.Distribution $target.User '/bin/sh' @('-lc', 'printf "%s\n%s\n" "${XDG_CONFIG_HOME:-$HOME/.config}/cats" "${XDG_STATE_HOME:-$HOME/.local/state}/cats"')
    $xdgPaths = @($xdg.Output -split "`n")
    $startMenuRoot = if ($StartMenuOverride) { [IO.Path]::GetFullPath($StartMenuOverride) } else { [Environment]::GetFolderPath('StartMenu') }
    $desktopRoot = if ($DesktopOverride) { [IO.Path]::GetFullPath($DesktopOverride) } else { [Environment]::GetFolderPath('Desktop') }
    $plan = @(Get-CatsUninstallPlan $localAppData $startMenuRoot $desktopRoot $payloadRoot -RemoveAllData:$RemoveAllData -ConfigRoot $xdgPaths[0] -StateRoot $xdgPaths[1])
    Write-Host 'CATS will remove these exact paths:'
    $plan | ForEach-Object { Write-Host "  [$($_.Side)/$($_.Kind)] $($_.Path)" }
    Write-Host 'Projects, worktrees, and external agent configuration are not removed.'
    if ($RemoveAllData) { Write-Warning 'CATS sessions, configuration, themes, and installed plugins in the listed data paths will be unrecoverable.' }
    if (-not $Force -and (Read-Host 'Type REMOVE to continue') -cne 'REMOVE') { throw 'uninstall cancelled' }

    foreach ($item in $plan) {
        if ($item.Kind -eq 'managed-link') {
            $linkTarget = Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/readlink' @($item.Path) -AllowFailure
            if ($linkTarget.ExitCode -eq 0 -and $linkTarget.Output -eq $item.ExpectedTarget) {
                Invoke-CatsWsl $target.Distribution $target.User '/bin/rm' @('-f', '--', $item.Path) | Out-Null
            }
            elseif ($linkTarget.ExitCode -eq 0 -or (Invoke-CatsWsl $target.Distribution $target.User '/usr/bin/test' @('-e', $item.Path) -AllowFailure).ExitCode -eq 0) {
                Write-Warning "Preserved non-CATS command path $($item.Path)"
            }
        }
        elseif ($item.Side -eq 'WSL') { Remove-CatsWslTree $target.Distribution $target.User $item.Path }
        elseif ($item.Kind -eq 'shortcut') { Remove-Item -LiteralPath $item.Path -Force -ErrorAction SilentlyContinue }
        elseif (Test-Path -LiteralPath $item.Path) { Remove-Item -LiteralPath $item.Path -Recurse -Force }
    }
    return $plan
}

Export-ModuleMember -Function Invoke-CatsInstall, Invoke-CatsUninstall, Test-CatsPackageChecksums, Test-CatsPackageLayout, Save-CatsAppConfig, Get-CatsUninstallPlan
