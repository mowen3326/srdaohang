# ============================================================
# daohang Windows launcher
# Usage: .\start.ps1
# Reads .env in the same directory (if present) and starts the app
# ============================================================
$ErrorActionPreference = 'Stop'

# $PSScriptRoot always points to the script's directory
$root = $PSScriptRoot
if (-not $root) {
    $root = Split-Path -Parent $MyInvocation.MyCommand.Path
}
Set-Location $root

# 1. Load .env (format: KEY=VALUE, lines starting with # are comments)
$envFile = Join-Path $root '.env'
if (Test-Path $envFile) {
    # Read as UTF-8 explicitly to handle Chinese comments correctly
    $lines = Get-Content -Path $envFile -Encoding UTF8
    foreach ($raw in $lines) {
        $line = $raw.Trim()
        if ($line -and -not $line.StartsWith('#')) {
            $idx = $line.IndexOf('=')
            if ($idx -gt 0) {
                $key = $line.Substring(0, $idx).Trim()
                $val = $line.Substring($idx + 1).Trim()
                [Environment]::SetEnvironmentVariable($key, $val, 'Process')
            }
        }
    }
} else {
    Write-Host "WARN: .env not found, using built-in defaults." -ForegroundColor Yellow
}

# 2. Locate the executable (prefer current dir daohang*.exe)
$exe = Get-ChildItem -Path $root -Filter 'daohang*.exe' | Select-Object -First 1
if (-not $exe) {
    throw "daohang*.exe not found in $root. Run build.ps1 first or copy the binary here."
}

Write-Host "Starting: $($exe.Name)" -ForegroundColor Cyan
& $exe.FullName
