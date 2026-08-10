[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PackageRoot,
    [Parameter(Mandatory = $true)][string]$Distribution,
    [Parameter(Mandatory = $true)][string]$User
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0
Import-Module (Join-Path $PSScriptRoot 'CatsInstaller.psm1') -Force
$PackageRoot = (Resolve-Path -LiteralPath $PackageRoot).Path

function New-IsolatedRoots {
    $token = [Guid]::NewGuid().ToString('N')
    $windows = Join-Path ([IO.Path]::GetTempPath()) ("cats phase6 $([char]0x00fc)nicode-$token")
    $wslBase = '/tmp/cats-phase6-integration-' + $token
    if (-not ([IO.Path]::GetFullPath($windows)).StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()), [StringComparison]::OrdinalIgnoreCase)) { throw 'unsafe Windows test root' }
    if ($wslBase -notmatch '^/tmp/cats-phase6-integration-[0-9a-f]{32}$') { throw 'unsafe WSL test root' }
    [pscustomobject]@{
        Windows = $windows
        WSLBase = $wslBase
        Payload = "$wslBase/.local/lib/cats"
        Local = (Join-Path $windows 'LocalAppData')
        Start = (Join-Path $windows 'StartMenu')
        Desktop = (Join-Path $windows 'Desktop')
    }
}

function Remove-IsolatedRoots {
    param($Roots)
    & wsl.exe --distribution $Distribution --user $User --exec /bin/rm -rf -- $Roots.WSLBase
    if ($LASTEXITCODE -ne 0) { throw 'could not remove isolated WSL test root' }
    if (Test-Path -LiteralPath $Roots.Windows) { Remove-Item -LiteralPath $Roots.Windows -Recurse -Force }
}

function Get-ShortcutFromIndependentProcess {
    param([string]$Path)
    $job = Start-Job -ScriptBlock {
        param($ShortcutPath)
        $shell = New-Object -ComObject WScript.Shell
        $shortcut = $shell.CreateShortcut($ShortcutPath)
        [pscustomobject]@{
            TargetPath = $shortcut.TargetPath
            Arguments = $shortcut.Arguments
            WorkingDirectory = $shortcut.WorkingDirectory
            IconLocation = $shortcut.IconLocation
        }
    } -ArgumentList $Path
    try { return Receive-Job -Job $job -Wait -ErrorAction Stop }
    finally { Remove-Job -Job $job -Force -ErrorAction SilentlyContinue }
}

$success = New-IsolatedRoots
try {
    $installed = Invoke-CatsInstall -PackageRoot $PackageRoot -Distribution $Distribution -User $User `
        -DesktopShortcut -NoLaunch -SkipPrerequisiteCheck -PayloadRootOverride $success.Payload `
        -LocalAppDataOverride $success.Local -StartMenuOverride $success.Start -DesktopOverride $success.Desktop
    if (-not (Test-Path -LiteralPath (Join-Path $installed.WindowsPath 'Cats.exe'))) { throw 'isolated launcher was not installed' }
    $expectedLauncher = Join-Path $installed.WindowsPath 'Cats.exe'
    $release = Get-Content -LiteralPath (Join-Path $PackageRoot 'release.json') -Raw | ConvertFrom-Json
    $expectedShortcutTarget = if ([bool]$release.authenticode.release_candidate_signed) { $expectedLauncher } else { Join-Path $env:SystemRoot 'explorer.exe' }
    $expectedShortcutArguments = if ([bool]$release.authenticode.release_candidate_signed) { '' } else { '"' + $expectedLauncher + '"' }
    foreach ($shortcutPath in @((Join-Path $success.Start 'Programs\Cats.lnk'), (Join-Path $success.Desktop 'Cats.lnk'))) {
        if (-not (Test-Path -LiteralPath $shortcutPath -PathType Leaf)) { throw "managed shortcut was not created: $shortcutPath" }
        $shortcut = Get-ShortcutFromIndependentProcess $shortcutPath
        if ($shortcut.TargetPath -ne $expectedShortcutTarget -or $shortcut.Arguments -ne $expectedShortcutArguments -or
            $shortcut.WorkingDirectory -ne $installed.WindowsPath -or $shortcut.IconLocation -ne "$expectedLauncher,0") {
            throw "managed shortcut did not retain its launch properties: $shortcutPath"
        }
    }
    foreach ($name in @('catway', 'cathost', 'catctl')) {
        $link = "$($success.WSLBase)/.local/bin/$name"
        $expected = "$($success.Payload)/current/$name"
        $target = (& wsl.exe --distribution $Distribution --user $User --exec /usr/bin/readlink $link)
        if ($LASTEXITCODE -ne 0 -or $target -ne $expected) { throw "managed command link $name was not installed safely" }
    }
    Invoke-CatsUninstall -Distribution $Distribution -User $User -Force -PayloadRootOverride $success.Payload `
        -LocalAppDataOverride $success.Local -StartMenuOverride $success.Start -DesktopOverride $success.Desktop | Out-Null
    if (Test-Path -LiteralPath (Join-Path $success.Local 'Programs\Cats')) { throw 'Windows payload survived uninstall' }
    & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/test '!' -e $success.Payload
    if ($LASTEXITCODE -ne 0) { throw 'WSL payload survived uninstall' }
    foreach ($name in @('catway', 'cathost', 'catctl')) {
        & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/test '!' -L "$($success.WSLBase)/.local/bin/$name"
        if ($LASTEXITCODE -ne 0) { throw "managed command link $name survived uninstall" }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $success.Local 'Cats\app.json'))) { throw 'default uninstall removed launcher config' }
}
finally { Remove-IsolatedRoots $success }

$rollback = New-IsolatedRoots
$badPackage = Join-Path $rollback.Windows 'bad-package'
try {
    [IO.Directory]::CreateDirectory($rollback.Windows) | Out-Null
    $configPath = Join-Path $rollback.Local 'Cats\app.json'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $configPath)) | Out-Null
    [IO.File]::WriteAllBytes($configPath, [Text.Encoding]::UTF8.GetBytes('{"mode":"remote","remote":{"url":"https://cats.example","label":"preserve-me"}}'))
    $baseline = Invoke-CatsInstall -PackageRoot $PackageRoot -Distribution $Distribution -User $User `
        -NoLaunch -SkipPrerequisiteCheck -PayloadRootOverride $rollback.Payload `
        -LocalAppDataOverride $rollback.Local -StartMenuOverride $rollback.Start -DesktopOverride $rollback.Desktop
    $baselineExecutable = Join-Path $baseline.WindowsPath 'Cats.exe'
    $baselineDigest = (Get-FileHash -LiteralPath $baselineExecutable -Algorithm SHA256).Hash
    $originalConfig = [IO.File]::ReadAllBytes($configPath)

    Copy-Item -LiteralPath $PackageRoot -Destination $badPackage -Recurse
    Copy-Item -LiteralPath (Join-Path $env:SystemRoot 'System32\where.exe') -Destination (Join-Path $badPackage 'Cats.exe') -Force
    $manifestPath = Join-Path $badPackage 'SHA256SUMS'
    $badDigest = (Get-FileHash -LiteralPath (Join-Path $badPackage 'Cats.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
    $lines = Get-Content -LiteralPath $manifestPath | ForEach-Object {
        if ($_ -match '  Cats\.exe$') { "$badDigest  Cats.exe" } else { $_ }
    }
    [IO.File]::WriteAllLines($manifestPath, $lines, (New-Object Text.UTF8Encoding($false)))

    $failed = $false
    try {
        Invoke-CatsInstall -PackageRoot $badPackage -Distribution $Distribution -User $User `
            -NoLaunch -SkipPrerequisiteCheck -PayloadRootOverride $rollback.Payload `
            -LocalAppDataOverride $rollback.Local -StartMenuOverride $rollback.Start -DesktopOverride $rollback.Desktop | Out-Null
    }
    catch { $failed = $true }
    if (-not $failed) { throw 'forced smoke failure unexpectedly committed' }
    $restoredConfig = [IO.File]::ReadAllBytes($configPath)
    if ([Convert]::ToBase64String($originalConfig) -ne [Convert]::ToBase64String($restoredConfig)) { throw 'rollback did not restore app.json byte-for-byte' }
    if ((Get-FileHash -LiteralPath $baselineExecutable -Algorithm SHA256).Hash -ne $baselineDigest) { throw 'rollback did not restore the previous Windows release' }
    $current = (& wsl.exe --distribution $Distribution --user $User --exec /usr/bin/readlink "$($rollback.Payload)/current")
    if ($LASTEXITCODE -ne 0 -or $current -ne $baseline.Release) { throw 'rollback did not restore the previous WSL current pointer' }
    foreach ($name in @('catway', 'cathost', 'catctl')) {
        $link = "$($rollback.WSLBase)/.local/bin/$name"
        $target = (& wsl.exe --distribution $Distribution --user $User --exec /usr/bin/readlink $link)
        if ($LASTEXITCODE -ne 0 -or $target -ne "$($rollback.Payload)/current/$name") { throw "rollback did not preserve managed command link $name" }
    }
    $originalLocalAppData = $env:LOCALAPPDATA
    try {
        $env:LOCALAPPDATA = $rollback.Local
        $smoke = Start-Process -FilePath $baselineExecutable -ArgumentList '--install-smoke' -Wait -PassThru
    }
    finally { $env:LOCALAPPDATA = $originalLocalAppData }
    if ($smoke.ExitCode -ne 0) { throw 'restored installation failed its smoke check' }
}
finally { Remove-IsolatedRoots $rollback }

$collision = New-IsolatedRoots
try {
    $commandDirectory = "$($collision.WSLBase)/.local/bin"
    $commandPath = "$commandDirectory/catway"
    & wsl.exe --distribution $Distribution --user $User --exec /bin/mkdir -p $commandDirectory
    if ($LASTEXITCODE -ne 0) { throw 'could not create command-link collision directory' }
    & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/touch $commandPath
    if ($LASTEXITCODE -ne 0) { throw 'could not create command-link collision' }
    $failed = $false
    try {
        Invoke-CatsInstall -PackageRoot $PackageRoot -Distribution $Distribution -User $User `
            -NoLaunch -SkipPrerequisiteCheck -PayloadRootOverride $collision.Payload `
            -LocalAppDataOverride $collision.Local -StartMenuOverride $collision.Start -DesktopOverride $collision.Desktop | Out-Null
    }
    catch { $failed = $true }
    if (-not $failed) { throw 'non-CATS command-link collision was overwritten' }
    & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/test -f $commandPath
    if ($LASTEXITCODE -ne 0) { throw 'non-CATS command path was not preserved' }
    if (Test-Path -LiteralPath (Join-Path $collision.Local 'Programs\Cats')) { throw 'link collision staged Windows payload files' }
    & wsl.exe --distribution $Distribution --user $User --exec /usr/bin/test '!' -e $collision.Payload
    if ($LASTEXITCODE -ne 0) { throw 'link collision staged WSL payload files' }
}
finally { Remove-IsolatedRoots $collision }

Write-Host 'PASS: isolated install/smoke/uninstall, managed-link ownership, and existing-install rollback integration'
