# Keep Windows resources consistent with the version in src/main.go.
$ErrorActionPreference = 'Stop'
$source = Get-Content -LiteralPath 'src/main.go' -Raw
$match = [regex]::Match($source, '(?m)^const\s+appVersion\s*=\s*"(?<version>[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?)"')
if (-not $match.Success) { throw 'Cannot read appVersion from src/main.go' }
$parts = @($match.Groups['version'].Value.Split('.'))
if ($parts.Count -eq 3) { $parts += '0' }
if ($parts.Count -ne 4) { throw "Unsupported version: $($match.Groups['version'].Value)" }
$version = $parts -join '.'
$path = 'winres/winres.json'
$data = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
$data.RT_MANIFEST.'#1'.'0409'.identity.version = $version
$fixed = $data.RT_VERSION.'#1'.'0000'.fixed
$fixed.file_version = $version
$fixed.product_version = $version
$info = $data.RT_VERSION.'#1'.'0000'.info.'0409'
$info.FileVersion = $version
$info.ProductVersion = $version
$json = $data | ConvertTo-Json -Depth 30
[System.IO.File]::WriteAllText(
    (Join-Path (Get-Location) $path),
    $json + [Environment]::NewLine,
    [System.Text.UTF8Encoding]::new($false)
)
Write-Host "Windows resource metadata synchronized to $version"

# Synchronize the Inno Setup fallback version. Command-line /DAppVersion
# remains available for local testing, but normal builds need no override.
$installerPath = 'installer/WhisperGUI.iss'
$installerSource = [System.IO.File]::ReadAllText((Join-Path (Get-Location) $installerPath))
$installerVersion = $match.Groups['version'].Value
$installerPattern = '(?m)^(\s*#define\s+AppVersion\s+)"[^"]+"'
$replacement = '${1}"' + $installerVersion + '"'
$updated = [regex]::Replace($installerSource, $installerPattern, $replacement)
if ($updated -eq $installerSource -and $installerSource -notmatch ('(?m)^\s*#define\s+AppVersion\s+"' + [regex]::Escape($installerVersion) + '"')) {
    throw "Cannot find the installer AppVersion definition in $installerPath"
}
[System.IO.File]::WriteAllText(
    (Join-Path (Get-Location) $installerPath),
    $updated,
    [System.Text.UTF8Encoding]::new($false)
)
Write-Host "Inno Setup version synchronized to $installerVersion"
