# Cross-compile pathsize for all supported platforms into dist/.
# Usage: pwsh ./build.ps1
$ErrorActionPreference = 'Stop'

$binary = 'pathsize'
$dist = 'dist'

$version = (git describe --tags --always --dirty 2>$null)
if (-not $version) { $version = 'dev' }
$ldflags = "-s -w -X main.version=$version"

$platforms = @(
    'linux/amd64',
    'linux/arm64',
    'darwin/amd64',
    'darwin/arm64',
    'windows/amd64',
    'windows/arm64'
)

New-Item -ItemType Directory -Path $dist -Force | Out-Null

foreach ($p in $platforms) {
    $parts = $p.Split('/')
    $env:GOOS = $parts[0]
    $env:GOARCH = $parts[1]
    $ext = if ($parts[0] -eq 'windows') { '.exe' } else { '' }
    $out = Join-Path $dist "$($binary)_$($parts[0])_$($parts[1])$ext"
    Write-Host "building $out (version $version)"
    go build -ldflags $ldflags -o $out .
}

Remove-Item Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
Write-Host "done -> $dist"
