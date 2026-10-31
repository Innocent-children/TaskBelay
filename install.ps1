[CmdletBinding()]
param(
    [switch]$Source,
    [string]$Version = 'latest'
)

$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Use install.sh on macOS, Linux or WSL.' }
if ($Source -and $Version -ne 'latest') { throw '-Source and -Version cannot be combined.' }
if ($Version -ne 'latest' -and $Version -notmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid release tag.' }
$architecture = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $architecture = $env:PROCESSOR_ARCHITEW6432 }
switch ($architecture) {
    'AMD64' { $asset = 'taskbelay-windows-amd64.exe'; $goArch = 'amd64' }
    'ARM64' { $asset = 'taskbelay-windows-arm64.exe'; $goArch = 'arm64' }
    default { throw 'Supported architectures: amd64, arm64.' }
}
$localData = $env:LOCALAPPDATA
if (-not $localData) { $localData = Join-Path $env:USERPROFILE 'AppData\Local' }
$installDir = Join-Path $localData 'Programs\TaskBelay'
$target = Join-Path $installDir 'taskbelay.exe'
$marker = Join-Path $installDir '.taskbelay-install'
$markerText = "TaskBelay user installation v1`n"
[IO.Directory]::CreateDirectory($installDir) | Out-Null
$installLock = Join-Path $installDir '.taskbelay-install.lock'
New-Item -ItemType Directory -Path $installLock -ErrorAction Stop | Out-Null
$stage = $null
try {
    foreach ($path in @($target, $marker)) {
        if (Test-Path -LiteralPath $path) {
            $item = Get-Item -Force -LiteralPath $path
            if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                throw "Refusing to replace a directory or link: $path"
            }
        }
    }
    if ((Test-Path -LiteralPath $target) -and -not (Test-Path -LiteralPath $marker)) {
        throw "Executable is not managed by TaskBelay: $target"
    }
    if ((Test-Path -LiteralPath $marker) -and [IO.File]::ReadAllText($marker) -ne $markerText) {
        throw 'Installation marker does not belong to TaskBelay.'
    }
    $stage = Join-Path $installDir ('.taskbelay-stage-' + [Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($stage) | Out-Null
    $staged = Join-Path $stage 'taskbelay.exe'
    if ($Source) {
        Get-Command go -ErrorAction Stop | Out-Null
        if (-not $PSScriptRoot -or -not (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'go.mod'))) {
            throw 'Run -Source from a TaskBelay checkout with Go 1.26 or newer.'
        }
        Write-Host 'Building TaskBelay from source...'
        Push-Location $PSScriptRoot
        $previousGOOS, $previousGOARCH, $previousCGO, $previousGOWORK = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED, $env:GOWORK
        try {
            $env:GOOS = 'windows'; $env:GOARCH = $goArch; $env:CGO_ENABLED = '0'; $env:GOWORK = 'off'
            & go build -trimpath -o $staged ./cmd/taskbelay
            if ($LASTEXITCODE -ne 0) { throw 'Build failed; previous version retained.' }
        } finally {
            $env:GOOS = $previousGOOS; $env:GOARCH = $previousGOARCH; $env:CGO_ENABLED = $previousCGO; $env:GOWORK = $previousGOWORK
            Pop-Location
        }
    } else {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        $releases = 'https://github.com/Innocent-children/TaskBelay/releases'
        if ($Version -eq 'latest') { $base = "$releases/latest/download" } else { $base = "$releases/download/$Version" }
        $metadataPath = Join-Path $stage 'release-manifest.json'
        Invoke-WebRequest -UseBasicParsing -Uri "$base/release-manifest.json" -OutFile $metadataPath -TimeoutSec 300
        $metadata = [IO.File]::ReadAllText($metadataPath) | ConvertFrom-Json
        $keys = @($metadata.PSObject.Properties.Name)
        if ($keys.Count -ne 3 -or @($keys | Where-Object { $_ -cnotin @('version', 'revision', 'assets') }).Count -ne 0 -or $metadata.version -isnot [string] -or $metadata.revision -isnot [string] -or $metadata.assets -isnot [Array]) { throw 'Invalid release metadata.' }
        $assets = @{}
        foreach ($name in $metadata.assets) {
            if ($name -isnot [string] -or $assets.ContainsKey($name) -or $name -cnotmatch '^(taskbelay-(darwin|linux)-(amd64|arm64)|taskbelay-windows-(amd64|arm64)\.exe|install\.sh|install\.ps1|SHA256SUMS)$') { throw 'Invalid or duplicate release asset.' }
            $assets[$name] = $true
        }
        $releaseVersion = $metadata.version
        if ($releaseVersion -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or $metadata.revision -cnotmatch '^([0-9a-f]{40}|[0-9a-f]{64})$' -or -not $assets[$asset] -or -not $assets['SHA256SUMS']) { throw 'Invalid release identity or missing platform asset; previous version retained.' }
        if ($Version -ne 'latest' -and $Version -cne $releaseVersion) { throw 'Release tag does not match metadata; previous version retained.' }
        $Version = $releaseVersion
        $base = "$releases/download/$releaseVersion"
        Write-Host "Downloading $asset ($Version)..."
        $manifestPath = Join-Path $stage 'SHA256SUMS'
        Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile $manifestPath -TimeoutSec 300
        $manifest = [IO.File]::ReadAllText($manifestPath)
        $entries = @($manifest -split "`n" | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($asset) + '\s*$') })
        if ($entries.Count -ne 1) { throw 'Missing or invalid SHA-256 entry; previous version retained.' }
        $expected = ($entries[0] -split '\s+')[0]
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $staged -TimeoutSec 300
        if ((Get-FileHash -LiteralPath $staged -Algorithm SHA256).Hash -ne $expected) {
            throw 'SHA-256 mismatch; previous version retained.'
        }
    }
    $installedVersion = & $staged --version
    if ($LASTEXITCODE -ne 0 -or $installedVersion -notlike 'TaskBelay *') { throw 'Invalid TaskBelay executable; previous version retained.' }
    if (-not $Source -and -not $installedVersion.StartsWith("TaskBelay $releaseVersion (")) { throw 'Executable version does not match release metadata; previous version retained.' }
    $retired = $null
    if (Test-Path -LiteralPath $target) {
        $retired = Join-Path $installDir ('.taskbelay-retired-' + [Guid]::NewGuid().ToString('N') + '.exe')
        [IO.File]::Move($target, $retired)
    }
    try { [IO.File]::Move($staged, $target) }
    catch {
        if ($retired) { [IO.File]::Move($retired, $target) }
        throw
    }
    [IO.File]::WriteAllText($marker, $markerText, [Text.Encoding]::ASCII)
    if ($retired) {
        try { Remove-Item -LiteralPath $retired -Force }
        catch { Write-Warning "Old executable is still in use. Close older Agent sessions, then remove: $retired" }
    }
    [string]$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $installDir) {
        $userPath = ($userPath.TrimEnd(';') + ';' + $installDir).TrimStart(';')
        [Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
    }
    if (($env:PATH -split ';') -notcontains $installDir) { $env:PATH += ';' + $installDir }
    Write-Host "`nInstalled $installedVersion"
    Write-Host ("Run now: & '" + $target.Replace("'", "''") + "'")
    Write-Host 'Or reopen your terminal and run: taskbelay. Choose Agent integration in the menu.'
} finally {
    if ($stage) {
        $staged = Join-Path $stage 'taskbelay.exe'
        if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged -Force }
        $manifestPath = Join-Path $stage 'SHA256SUMS'
        if (Test-Path -LiteralPath $manifestPath) { Remove-Item -LiteralPath $manifestPath -Force }
        $metadataPath = Join-Path $stage 'release-manifest.json'
        if (Test-Path -LiteralPath $metadataPath) { Remove-Item -LiteralPath $metadataPath -Force }
        [IO.Directory]::Delete($stage)
    }
    [IO.Directory]::Delete($installLock)
}
