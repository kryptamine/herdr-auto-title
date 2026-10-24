# Puts herdr-auto-title.exe in the plugin root: the release binary when this
# checkout is that release's commit, else a Go build;
# docs/architecture/installation.md says why.

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
$Stable = 'herdr plugin install kryptamine/herdr-auto-title --ref stable'
$script:Released = $false
$script:Hinted = $false
$script:GoMissing = $false

function Say([string]$Message) { [Console]::Error.WriteLine("${Name}: $Message") }

$Version = $null
foreach ($Line in Get-Content -LiteralPath 'herdr-plugin.toml' -ErrorAction Stop) {
    if ($Line -match '^version\s*=\s*"([^"]*)"') { $Version = $Matches[1]; break }
}

$Head = $null
if (Get-Command git -ErrorAction SilentlyContinue) {
    $Head = (& git rev-parse HEAD 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { $Head = $null }
}

$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("$Name-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $Temp -ErrorAction Stop | Out-Null
# Windows PowerShell 5.1 still offers only TLS 1.0 on some systems.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

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

function Get-File([string]$Url, [string]$File) {
    Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri $Url -OutFile $File -ErrorAction Stop
}

# Moves a verified download into the plugin root once it reports the
# manifest's version there.
function Install-Checked([string]$File) {
    $Staged = Join-Path $Root ".$Name.$PID.exe"
    try {
        try {
            Copy-Item -LiteralPath $File -Destination $Staged -Force -ErrorAction Stop
        } catch {
            Say "the downloaded binary could not be copied into the plugin root: $($_.Exception.Message)"
            return $false
        }
        try {
            $Reported = (& $Staged --version 2>$null | Out-String).Trim()
            $Exit = $LASTEXITCODE
        } catch {
            $Exit = 1
        }
        if ($Exit -ne 0) {
            Say 'the downloaded binary does not run on this machine'
            return $false
        }
        if ($Reported -ne "$Name $Version") {
            Say "the downloaded binary reports '$Reported', not $Version"
            return $false
        }
        try {
            Move-Item -LiteralPath $Staged -Destination (Join-Path $Root "$Name.exe") -Force -ErrorAction Stop
        } catch {
            Say "the downloaded binary could not be put in place: $($_.Exception.Message)"
            return $false
        }
        return $true
    } finally {
        Remove-Item -LiteralPath $Staged -Force -ErrorAction SilentlyContinue
    }
}

function Install-Release([string]$Base) {
    $File = Join-Path $Temp $Asset
    $Sums = Join-Path $Temp 'SHA256SUMS'
    try {
        Get-File "$Base/$Asset" $File
        Get-File "$Base/SHA256SUMS" $Sums
    } catch {
        Say "$Asset could not be downloaded: $($_.Exception.Message)"
        return $false
    }

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
}

# The commit the release was cut from, or $null. Herdr's checkout keeps .git
# but no tags, so that commit is all it can be matched by.
function Get-ReleaseCommit([string]$Base) {
    $File = Join-Path $Temp 'COMMIT'
    try { Get-File "$Base/COMMIT" $File } catch { return $null }
    return ([string](Get-Content -LiteralPath $File -Raw)).Trim()
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

function Install-Plugin {
    $Asset = Get-AssetName
    if (-not $Asset) { Say "no release binary is published for $env:PROCESSOR_ARCHITECTURE" }

    if ($Asset -and $Version) {
        $Base = "$Releases/download/v$Version"
        $Commit = Get-ReleaseCommit $Base
        if (-not $Commit) {
            $script:Hinted = $true
            Say "release $Version could not be fetched, so it is built from source;"
            Say "for the latest release instead: $Stable"
        } elseif ($Commit -ne $Head) {
            $script:Hinted = $true
            Say "this checkout is not release $Version, so it is built from source;"
            Say "for the latest release instead: $Stable"
        } else {
            $script:Released = $true
            Say "downloading $Asset $Version"
            if (Install-Release $Base) {
                Say "installed release $Version"
                return $true
            }
            Say "release $Version could not be installed"
        }
    }

    if (Invoke-Build) { return $true }

    if ($script:GoMissing -and $script:Released) {
        Say "could not install ${Version}: its release binary could not be installed,"
        Say 'and building it from source needs Go 1.24 or newer'
    } elseif ($script:GoMissing -and $script:Hinted) {
        Say 'could not install: building from source needs Go 1.24 or newer;'
        Say "to install the latest release without Go: $Stable"
    } elseif ($script:GoMissing) {
        Say 'could not install: building from source needs Go 1.24 or newer'
    } else {
        Say 'could not install: the source build above failed'
    }
    return $false
}

try {
    $Ok = Install-Plugin
} finally {
    Remove-Item -LiteralPath $Temp -Recurse -Force -ErrorAction SilentlyContinue
}
if ($Ok) { exit 0 }
exit 1
