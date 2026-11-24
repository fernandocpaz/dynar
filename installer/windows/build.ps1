param(
    [Parameter(Mandatory = $true)][string]$AssetDir,
    [Parameter(Mandatory = $true)][string]$OutputDir,
    [Parameter(Mandatory = $true)][string]$ReleaseTag
)
$ErrorActionPreference = 'Stop'
if ($ReleaseTag -notmatch '^v(\d+\.\d+\.\d+)(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "Invalid version: $ReleaseTag"
}
$numeric = $Matches[1]
$compiler = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'
if (-not (Test-Path -LiteralPath $compiler)) { throw 'Inno Setup 6 is required on the build machine.' }
$assets = (Resolve-Path -LiteralPath $AssetDir).Path
foreach ($arch in @('amd64','arm64')) {
    if (-not (Test-Path -LiteralPath (Join-Path $assets "dynar_${ReleaseTag}_windows_$arch.exe"))) {
        throw "Missing Windows $arch payload"
    }
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$out = (Resolve-Path -LiteralPath $OutputDir).Path
$script = Join-Path $PSScriptRoot 'dynar.iss'
& $compiler "/DReleaseTag=$ReleaseTag" "/DNumericVersion=$numeric" "/DAssetDir=$assets" "/O$out" $script
if ($LASTEXITCODE -ne 0) { throw "Inno Setup failed: $LASTEXITCODE" }
