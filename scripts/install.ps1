# Puts herdr-auto-title.exe in the plugin root: this version's release binary,
# else a Go build, else the latest published release. Herdr runs it on install;
# docs/architecture/installation.md says why in that order.

Set-StrictMode -Version 2
# Not 'Stop': Windows PowerShell 5.1 then turns a native program's stderr into
# a terminating error. The cmdlets whose failure matters ask for Stop.
$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $Root -ErrorAction Stop

$Name = 'herdr-auto-title'
$Releases = 'https://github.com/kryptamine/herdr-auto-title/releases'
if ($env:HERDR_AUTO_TITLE_RELEASES) { $Releases = $env:HERDR_AUTO_TITLE_RELEASES }
$script:Installed = $null
$script:GoMissing = $false

function Say([string]$Message) { [Console]::Error.WriteLine("${Name}: $Message") }

$Version = $null
foreach ($Line in Get-Content -LiteralPath 'herdr-plugin.toml' -ErrorAction Stop) {
    if ($Line -match '^version\s*=\s*"([^"]*)"') { $Version = $Matches[1]; break }
}

function Get-AssetName {
    try {
        $Arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    } catch {
        # An older .NET has no RuntimeInformation; a 32-bit shell reports the
        # machine's architecture in PROCESSOR_ARCHITEW6432.
        $Arch = $env:PROCESSOR_ARCHITEW6432
        if (-not $Arch) { $Arch = $env:PROCESSOR_ARCHITECTURE }
    }
    switch -Regex ($Arch) {
        '^(X64|AMD64)$' { return "$Name-windows-amd64.exe" }
        '^ARM64$' { return "$Name-windows-arm64.exe" }
    }
    return $null
}

# Moves a verified download into the plugin root once it runs there. Its
# checksum already says which release it is; running it says it works here.
function Install-Checked([string]$File) {
    $Staged = Join-Path $Root ".$Name.$PID.exe"
    Copy-Item -LiteralPath $File -Destination $Staged -Force -ErrorAction Stop
    try {
        $Reported = (& $Staged --version 2>$null | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) {
            Say 'the downloaded binary does not run on this machine'
            return $false
        }
        Move-Item -LiteralPath $Staged -Destination (Join-Path $Root "$Name.exe") -Force -ErrorAction Stop
        $script:Installed = $Reported.Substring("$Name ".Length)
        return $true
    } finally {
        Remove-Item -LiteralPath $Staged -Force -ErrorAction SilentlyContinue
    }
}

function Install-Release([string]$Base) {
    $Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("$Name-" + [System.Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $Temp -ErrorAction Stop | Out-Null
    try {
        # Windows PowerShell 5.1 still offers only TLS 1.0 on some systems.
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        $File = Join-Path $Temp $Asset
        $Sums = Join-Path $Temp 'SHA256SUMS'
        Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri "$Base/$Asset" -OutFile $File -ErrorAction Stop
        Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri "$Base/SHA256SUMS" -OutFile $Sums -ErrorAction Stop

        $Expected = $null
        foreach ($Line in Get-Content -LiteralPath $Sums -ErrorAction Stop) {
            $Fields = $Line.Trim() -split '\s+', 2
            if ($Fields.Count -eq 2 -and ($Fields[1] -eq $Asset -or $Fields[1] -eq "*$Asset")) {
                $Expected = $Fields[0].ToLowerInvariant()
                break
            }
        }
        # Not Get-FileHash: started from PowerShell 7, Windows PowerShell inherits
        # its module path and cannot load the module that defines it.
        $Sha = [System.Security.Cryptography.SHA256]::Create()
        $Stream = [System.IO.File]::OpenRead($File)
        try { $Actual = ([System.BitConverter]::ToString($Sha.ComputeHash($Stream)) -replace '-', '').ToLowerInvariant() }
        finally { $Stream.Dispose(); $Sha.Dispose() }
        if ($Expected -ne $Actual) {
            Say "$Asset does not match its checksum in SHA256SUMS"
            return $false
        }
        return (Install-Checked $File)
    } catch {
        Say "download failed: $($_.Exception.Message)"
        return $false
    } finally {
        Remove-Item -LiteralPath $Temp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-Build {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        $script:GoMissing = $true
        Say 'Go is not installed, so it cannot be built from source'
        return $false
    }
    Say 'building from source'
    # Its stdout would otherwise become part of what this function returns.
    & go build -o "$Name.exe" ./cmd/herdr-auto-title | Out-Null
    if ($LASTEXITCODE -eq 0) { return $true }
    Say 'it could not be built from source, which needs Go 1.24 or newer'
    return $false
}

if ($env:HERDR_AUTO_TITLE_BUILD -eq 'source') {
    if (Invoke-Build) { exit 0 }
    exit 1
}

$Asset = Get-AssetName
if (-not $Asset) { Say "no release binary is published for $env:PROCESSOR_ARCHITECTURE" }

if ($Asset -and $Version) {
    Say "downloading $Asset $Version"
    if (Install-Release "$Releases/download/v$Version") {
        Say "installed release $($script:Installed)"
        exit 0
    }
    Say "release $Version could not be installed"
}

if (Invoke-Build) { exit 0 }

# A release in the minutes before its binaries are attached is still a draft,
# and latest/download then serves the release before it.
if ($Asset -and (Install-Release "$Releases/latest/download")) {
    Say "installed release $($script:Installed), the latest published one, in place of ${Version};"
    Say "if $Version was released minutes ago, install again shortly to get it"
    exit 0
}

Say "could not install ${Version}: no release binary could be downloaded, and"
if ($script:GoMissing) {
    Say 'building one from source needs Go 1.24 or newer, which is not installed'
} else {
    Say 'the source build above failed'
}
exit 1
