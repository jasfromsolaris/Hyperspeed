# Live-reload API (Air) + DEBUG logs. Requires: Go, Air, Docker (Postgres + Redis).
# Usage: from repo root, .\scripts\dev-api.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

if (-not (Get-Command air -ErrorAction SilentlyContinue)) {
    Write-Host "Air not found. Install with:"
    Write-Host "  go install github.com/air-verse/air@latest"
    Write-Host "Ensure `$env:PATH includes your Go bin directory."
    exit 1
}

Write-Host "Starting postgres + redis..."
docker compose up -d postgres redis

$env:DEBUG = "true"
$env:DATABASE_URL = if ($env:DATABASE_URL) { $env:DATABASE_URL } else { "postgres://hyperspeed:hyperspeed@localhost:5432/hyperspeed?sslmode=disable" }
$env:REDIS_URL = if ($env:REDIS_URL) { $env:REDIS_URL } else { "redis://localhost:6379/0" }
# Optional: IDE Git clones (defaults unset = git panel shows integration unavailable until set).
if (-not $env:HS_GIT_WORKDIR_BASE) {
    $gitDir = Join-Path $env:TEMP "hyperspeed-git"
    New-Item -ItemType Directory -Force -Path $gitDir | Out-Null
    $env:HS_GIT_WORKDIR_BASE = $gitDir
}

Set-Location (Join-Path $root "apps\api")
Write-Host "Air watching Go files (DEBUG=true). Ctrl+C to stop."
air
