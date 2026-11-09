# Install a checked release from a terminal tab, then report to the popup.
param([string]$Version, [string]$Directory)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$status = Join-Path $Directory 'status'
$sums = Join-Path $Directory 'SHA256SUMS'
$binary = $null
$result = 1
$reason = 'The install was stopped.'

try {
    $arch = $env:PROCESSOR_ARCHITEW6432
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'ARM64' { $arch = 'arm64' }
        'AMD64' { $arch = 'amd64' }
        default { throw "No release binary for $arch." }
    }
    $asset = "herdr-auto-title-windows-$arch.exe"
    $binary = Join-Path $Directory $asset
    $releases = $env:HERDR_AUTO_TITLE_RELEASES
    if (-not $releases) { $releases = 'https://github.com/kryptamine/herdr-auto-title/releases' }
    $url = "$releases/download/v$Version"
    Write-Host "Downloading Auto Title $Version"
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "$url/$asset" -OutFile $binary
        Invoke-WebRequest -UseBasicParsing -Uri "$url/SHA256SUMS" -OutFile $sums
    } catch {
        throw "The download of $Version failed."
    }
    $pattern = '^([a-fA-F0-9]{64})\s+\*?' + [regex]::Escape($asset) + '$'
    $want = Select-String -LiteralPath $sums -Pattern $pattern | Select-Object -First 1
    $sha = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($binary)
    try {
        $have = [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '')
    } finally {
        $stream.Dispose()
        $sha.Dispose()
    }
    if (-not $want -or $want.Matches[0].Groups[1].Value -ne $have) {
        throw "$asset does not match SHA256SUMS."
    }
    & $binary desktop install
    if ($LASTEXITCODE -ne 0) { throw 'desktop install failed.' }
    $result = 0
} catch {
    $reason = $_.Exception.Message
} finally {
    # A whole terminal killed by Windows cannot run finally; the popup times out.
    foreach ($file in @($binary, $sums)) {
        if ($file -and (Test-Path -LiteralPath $file)) { Remove-Item -LiteralPath $file -Force }
    }
    $text = if ($result -eq 0) { "0`n" } else { "1`n$reason`n" }
    if (Test-Path -LiteralPath $Directory) { [IO.File]::WriteAllText($status, $text) }
}

if ($result -ne 0) {
    Write-Host $reason
    if (-not [Console]::IsInputRedirected) { [void](Read-Host 'Press Enter to close') }
}
if ($env:HC_TERMINAL_ID) { & hc kill $env:HC_TERMINAL_ID }
exit $result
