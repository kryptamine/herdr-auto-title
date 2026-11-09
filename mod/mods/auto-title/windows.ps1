# The popup's filesystem operations, with paths passed as arguments, never code.
param(
    [ValidateSet('copy', 'exists', 'temp', 'status', 'clean')][string]$Operation,
    [string]$Path
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)

switch ($Operation) {
    'copy' {
        $local = $env:LOCALAPPDATA
        if ($local -notmatch '^(?:[A-Za-z]:[\\/]|\\\\[^\\]+\\[^\\]+)') {
            $local = Join-Path $HOME 'AppData\Local'
        }
        [Console]::Write((Join-Path $local 'herdr-auto-title\bin\herdr-auto-title.exe'))
    }
    'exists' {
        if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { exit 1 }
    }
    'temp' {
        $folder = Join-Path ([IO.Path]::GetTempPath()) ('auto-title-' + [guid]::NewGuid().ToString('N'))
        [void][IO.Directory]::CreateDirectory($folder)
        [Console]::Write($folder)
    }
    'status' {
        $status = Join-Path $Path 'status'
        if (-not (Test-Path -LiteralPath $status)) { exit 1 }
        [Console]::Write([IO.File]::ReadAllText($status))
    }
    'clean' {
        $folder = [IO.Path]::GetFullPath($Path)
        $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/')
        if ([IO.Path]::GetDirectoryName($folder) -ne $temp -or
            [IO.Path]::GetFileName($folder) -notmatch '^auto-title-[a-f0-9]{32}$') {
            throw 'Refusing to remove a folder not created by Auto Title.'
        }
        if (Test-Path -LiteralPath $folder) { Remove-Item -LiteralPath $folder -Recurse -Force }
    }
}
