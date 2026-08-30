# ============================================================
# daohang cross-compile & package script
# Usage: run in project root:  .\build.ps1
# Output: dist\<goos>-<goarch>\  (executable + data dir)
# ============================================================
$ErrorActionPreference = 'Stop'

$appName = 'daohang'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

$targets = @(
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Exe = "$appName-windows-amd64.exe" },
    @{ GOOS = 'windows'; GOARCH = 'arm64'; Exe = "$appName-windows-arm64.exe" },
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Exe = "$appName-linux-amd64" },
    @{ GOOS = 'linux';   GOARCH = 'arm64'; Exe = "$appName-linux-arm64" }
)

$distDir = Join-Path $root 'dist'
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

# License files: MIT/BSD/OFL all require the notices to travel with the binary,
# so every platform folder gets a plain-text copy next to the executable.
$licMd = Join-Path $root 'THIRD_PARTY_LICENSES.md'
$projectLic = Join-Path $root 'LICENSE'
$licTxt = Join-Path $distDir 'THIRD_PARTY_LICENSES.txt'

if (-not (Test-Path $licMd)) { throw "missing THIRD_PARTY_LICENSES.md" }
if (-not (Test-Path $projectLic)) { throw "missing LICENSE" }

# Markdown code fences are noise in a plain-text file a user opens in Notepad.
$mdLines = ([IO.File]::ReadAllText($licMd)) -split "`r`n|`n"
$plainLines = $mdLines | Where-Object { $_ -notmatch '^\s*```' }
$plain = ($plainLines -join "`r`n").TrimEnd() + "`r`n"
[IO.File]::WriteAllText($licTxt, $plain, (New-Object System.Text.UTF8Encoding($false)))

Write-Host "Cross-compiling, output to dist\" -ForegroundColor Cyan

foreach ($t in $targets) {
    $platform = "$($t.GOOS)/$($t.GOARCH)"
    Write-Host "`n[$platform] building..." -ForegroundColor Yellow

    $outDir = Join-Path $distDir "$($t.GOOS)-$($t.GOARCH)"
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null

    $exePath = Join-Path $outDir $t.Exe

    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $env:CGO_ENABLED = '0'

    go build -ldflags="-s -w" -o $exePath .

    if ($LASTEXITCODE -ne 0) {
        throw "build failed: $platform"
    }

    # data dir (db + uploads) with placeholder files
    $dataOut = Join-Path $outDir 'data'
    New-Item -ItemType Directory -Force -Path (Join-Path $dataOut 'uploads') | Out-Null
    New-Item -ItemType File -Force -Path (Join-Path $dataOut '.gitkeep') | Out-Null
    New-Item -ItemType File -Force -Path (Join-Path $dataOut 'uploads\.gitkeep') | Out-Null

    # Ship the licenses next to the executable, not only in the repo.
    Copy-Item $projectLic (Join-Path $outDir 'LICENSE') -Force
    Copy-Item $licTxt (Join-Path $outDir 'THIRD_PARTY_LICENSES.txt') -Force

    Write-Host "[$platform] done -> $outDir" -ForegroundColor Green
}

$env:GOOS = $null
$env:GOARCH = $null
$env:CGO_ENABLED = $null

Write-Host "`nAll builds completed!" -ForegroundColor Cyan
Write-Host "Deploy tips:" -ForegroundColor Cyan
Write-Host "  1. Set a fixed SESSION_SECRET in production, otherwise sessions reset on restart"
Write-Host "  2. Use ADDR / DB_PATH env vars to change listen address / db path"
