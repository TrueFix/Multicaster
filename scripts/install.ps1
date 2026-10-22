param(
    [string]$Version = '__VERSION__',
    [string]$Repository = '__REPOSITORY__',
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\Multicaster')
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$') { throw 'Download install.ps1 from a published release.' }
if ($Repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw 'Repository must be owner/name.' }
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$architecture = $env:PROCESSOR_ARCHITEW6432
if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
switch ($architecture.ToUpperInvariant()) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw 'Supported Windows architectures: AMD64 and ARM64.' }
}
$asset = "multicaster-windows-$arch.exe"
$base = "https://github.com/$Repository/releases/download/v$Version"
$temp = Join-Path ([IO.Path]::GetTempPath()) ('multicaster-' + [guid]::NewGuid().ToString('N'))
$stage = $null
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $binary = Join-Path $temp $asset
    $checksums = Join-Path $temp 'SHA256SUMS'
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $binary
    Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile $checksums
    $expected = $null
    foreach ($line in Get-Content -LiteralPath $checksums) {
        if ($line -match '^([0-9a-f]{64})  (.+)$' -and $Matches[2] -eq $asset) { $expected = $Matches[1] }
    }
    if (-not $expected -or (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'Checksum mismatch. Nothing was installed.' }
    $reported = & $binary --version
    if ($LASTEXITCODE -ne 0 -or "$reported".Trim() -ne $Version) { throw 'Binary version does not match the release.' }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $target = Join-Path $InstallDir 'multicaster.exe'
    $stage = Join-Path $InstallDir ('.multicaster-' + [guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath $binary -Destination $stage
    try {
        if (Test-Path -LiteralPath $target) { [IO.File]::Replace($stage, $target, $null) }
        else { Move-Item -LiteralPath $stage -Destination $target }
    } catch { throw "Could not replace the executable. Stop Multicaster after your stream finishes, then retry. $($_.Exception.Message)" }
    $stage = $null
    Write-Host "Installed Multicaster $Version to $target"
    Write-Host "Run: & '$target'"
    Write-Host 'Your configuration is preserved in %APPDATA%\multicaster.'
} finally {
    if ($stage -and (Test-Path -LiteralPath $stage)) { Remove-Item -LiteralPath $stage -Force }
    Remove-Item -LiteralPath $temp -Recurse -Force
}
