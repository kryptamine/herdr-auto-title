$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
trap { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
$binary = {{ps .Binary}}
$operation = {{ps .Operation}}
$parentPID = {{.ParentPID}}
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$taskName = 'herdr-auto-title-' + $sid
$scheduler = New-Object -ComObject Schedule.Service
$scheduler.Connect()
$folder = $scheduler.GetFolder('\')
$task = $null
try {
    $task = $folder.GetTask($taskName)
} catch {
    if ($_.Exception.HResult -ne -2147024894) { throw }
}

if ($operation -eq 'running') {
    $null -ne $task -and $task.State -eq 4
    exit 0
}

if ($operation -eq 'stop' -or $operation -eq 'uninstall') {
    if ($null -ne $task) {
        $task.Enabled = $false
        $task.Stop(0)
        $deadline = [DateTime]::UtcNow.AddSeconds(10)
        while ($task.GetInstances(0).Count -ne 0) {
            if ([DateTime]::UtcNow -ge $deadline) { throw 'The desktop task did not stop.' }
            Start-Sleep -Milliseconds 100
        }
    }
    if ($operation -eq 'stop') { exit 0 }
    if ($null -ne $task) { $folder.DeleteTask($taskName, 0) }

    if (Test-Path -LiteralPath $binary) {
        $parent = Get-Process -Id $parentPID -ErrorAction Stop
        if ($parent.Path -eq $binary) {
            # Retire this exact image so a new install cannot be deleted by cleanup.
            $retired = $binary + '.' + [Guid]::NewGuid().ToString('N') + '.old'
            Move-Item -LiteralPath $binary -Destination $retired
            $quoted = "'" + $retired.Replace("'", "''") + "'"
            $cleanup = @"
`$ErrorActionPreference = 'Stop'
Wait-Process -Id $parentPID -Timeout 30 -ErrorAction SilentlyContinue
# The exited image stays locked briefly; measured about 200 ms on ARM64.
`$deadline = [DateTime]::UtcNow.AddSeconds(30)
while (Test-Path -LiteralPath $quoted) {
    try { Remove-Item -LiteralPath $quoted -Force } catch {
        if ([DateTime]::UtcNow -ge `$deadline) { throw }
        Start-Sleep -Milliseconds 100
    }
}
`$dir = Split-Path -LiteralPath $quoted
if ([IO.Directory]::GetFileSystemEntries(`$dir).Length -eq 0) { [IO.Directory]::Delete(`$dir) }
"@
            $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($cleanup))
            Start-Process -FilePath "$PSHOME\powershell.exe" -WindowStyle Hidden -ArgumentList @(
                '-NoProfile', '-NonInteractive', '-WindowStyle', 'Hidden', '-EncodedCommand', $encoded
            ) | Out-Null
            exit 0
        }
        $deadline = [DateTime]::UtcNow.AddSeconds(10)
        while (Test-Path -LiteralPath $binary) {
            try { Remove-Item -LiteralPath $binary -Force } catch {
                if ([DateTime]::UtcNow -ge $deadline) { throw }
                Start-Sleep -Milliseconds 100
            }
        }
    }
    $dir = Split-Path -LiteralPath $binary
    if ((Test-Path -LiteralPath $dir) -and [IO.Directory]::GetFileSystemEntries($dir).Length -eq 0) {
        [IO.Directory]::Delete($dir)
    }
    exit 0
}

$definition = $scheduler.NewTask(0)
$definition.RegistrationInfo.Description = 'Herdr Auto Title for the Herdr desktop app'
$definition.Principal.UserId = $sid
$definition.Principal.LogonType = 3
$definition.Principal.RunLevel = 0
$trigger = $definition.Triggers.Create(9)
$trigger.UserId = $sid
$definition.Triggers.Create(7) | Out-Null
$settings = $definition.Settings
$settings.Enabled = $true
$settings.MultipleInstances = 2
$settings.DisallowStartIfOnBatteries = $false
$settings.StopIfGoingOnBatteries = $false
$settings.ExecutionTimeLimit = 'PT0S'
$settings.RestartInterval = 'PT1M'
$settings.RestartCount = 999

$action = $definition.Actions.Create(0)
$action.Path = $binary
$action.Arguments = 'desktop service'
$task = $folder.RegisterTaskDefinition($taskName, $definition, 6, $sid, $null, 3)
$taskName
