param(
    [Parameter(Mandatory = $true)][string]$SetupPath,
    [Parameter(Mandatory = $true)][string]$ReleaseTag
)
$ErrorActionPreference = 'Stop'
$setup = (Resolve-Path -LiteralPath $SetupPath).Path
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\DynaR'
$exe = Join-Path $installDir 'dynar.exe'
$environmentKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
$original = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$originalKind = if ($null -ne $original) { $environmentKey.GetValueKind('Path') } else { $null }
$sentinel = 'C:\DynaR-Installer-Smoke-Sentinel'
$baseline = if ([string]::IsNullOrEmpty($original)) { $sentinel } else { "$original;$sentinel" }
$registryKind = [Microsoft.Win32.RegistryValueKind]::ExpandString
function Read-UserPath {
    $environmentKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
}
function Run-Setup([string]$File) {
    $p = Start-Process -FilePath $File -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART') -Wait -PassThru
    if ($p.ExitCode -ne 0) { throw "Setup failed with exit code $($p.ExitCode)" }
}
function Assert-Installed {
    if (-not (Test-Path -LiteralPath $exe)) { throw 'Installed executable missing' }
    $reported = & $exe version
    if ($LASTEXITCODE -ne 0 -or ($reported -join ' ') -notmatch ('^DynaR\s+' + [regex]::Escape($ReleaseTag) + '(?:\s|$)')) {
        throw "Unexpected DynaR version: $reported"
    }
    if ((Read-UserPath) -cne "$installDir;$baseline") { throw 'User PATH was not updated exactly once or existing entries changed' }
}
function Uninstall-DynaR {
    $uninstaller = Get-ChildItem -LiteralPath $installDir -Filter 'unins*.exe' | Select-Object -First 1
    if (-not $uninstaller) { throw 'Uninstaller missing' }
    Run-Setup $uninstaller.FullName
    if (Test-Path -LiteralPath $exe) { throw 'Uninstall did not remove DynaR' }
}
if (Test-Path -LiteralPath $installDir) { throw 'Installer smoke requires a clean runner' }
try {
    $environmentKey.SetValue('Path', $baseline, $registryKind)
    Run-Setup $setup
    Assert-Installed
    Run-Setup $setup
    Assert-Installed
    Uninstall-DynaR
    if ((Read-UserPath) -cne $baseline) { throw 'Uninstall modified unrelated PATH entries' }
    $baseline = "$installDir;$baseline"
    $environmentKey.SetValue('Path', $baseline, $registryKind)
    Run-Setup $setup
    Uninstall-DynaR
    if ((Read-UserPath) -cne $baseline) { throw 'Uninstall removed a pre-existing user PATH entry' }
    Write-Host "Windows offline install, version, reinstall, PATH ownership, and uninstall verified: $ReleaseTag"
} finally {
    if ($null -eq $original) { $environmentKey.DeleteValue('Path', $false) }
    else { $environmentKey.SetValue('Path', $original, $originalKind) }
    $environmentKey.Dispose()
}
