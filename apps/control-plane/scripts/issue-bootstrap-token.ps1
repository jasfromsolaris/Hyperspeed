# Issue a one-time PROVISIONING_BOOTSTRAP_TOKEN for a customer (Hyperspeed operator).
# Loads apps/control-plane/.env when present (same folder as this script's parent).
#
# Requires either:
#   - CONTROL_PLANE_PUBLIC_URL + CONTROL_PLANE_BEARER_TOKEN  (POST /v1/installs/bootstrap-token), or
#   - WORKER_ADMIN_URL + WORKER_ADMIN_TOKEN                (POST /v1/admin/bootstrap-token on Worker)
#
# Usage:
#   .\scripts\issue-bootstrap-token.ps1
#   .\scripts\issue-bootstrap-token.ps1 -TtlSec 1800
#   .\scripts\issue-bootstrap-token.ps1 -InstallId "optional-stable-id"

param(
    [int]$TtlSec = 900,
    [string]$InstallId = ""
)

$ErrorActionPreference = "Stop"
# This script lives in apps/control-plane/scripts
$cpDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$envFile = Join-Path $cpDir ".env"

function Import-DotEnv {
    param([string]$Path)
    if (-not (Test-Path $Path)) { return }
    Get-Content $Path | ForEach-Object {
        $line = $_.Trim()
        if ($line -match '^\s*#' -or $line -eq "") { return }
        $eq = $line.IndexOf("=")
        if ($eq -lt 1) { return }
        $key = $line.Substring(0, $eq).Trim()
        $val = $line.Substring($eq + 1).Trim()
        if ($key -and -not [string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($key, "Process"))) { return }
        [Environment]::SetEnvironmentVariable($key, $val, "Process")
    }
}

Set-Location $cpDir
Import-DotEnv $envFile

$cpUrl = [Environment]::GetEnvironmentVariable("CONTROL_PLANE_PUBLIC_URL", "Process")
$bearer = [Environment]::GetEnvironmentVariable("CONTROL_PLANE_BEARER_TOKEN", "Process")
$workerUrl = [Environment]::GetEnvironmentVariable("WORKER_ADMIN_URL", "Process")
if (-not $workerUrl) { $workerUrl = "https://provision-gw.hyperspeedapp.com" }
$workerAdmin = [Environment]::GetEnvironmentVariable("WORKER_ADMIN_TOKEN", "Process")

$bodyObj = @{ ttl_sec = $TtlSec }
if ($InstallId) { $bodyObj.install_id = $InstallId }
$json = $bodyObj | ConvertTo-Json -Compress

if ($cpUrl -and $bearer) {
    $uri = $cpUrl.TrimEnd("/") + "/v1/installs/bootstrap-token"
    Write-Host "Calling control plane: $uri" -ForegroundColor DarkGray
    $resp = Invoke-RestMethod -Uri $uri -Method Post -Headers @{
        Authorization = "Bearer $bearer"
        "Content-Type"  = "application/json"
    } -Body $json
} elseif ($workerAdmin) {
    $uri = $workerUrl.TrimEnd("/") + "/v1/admin/bootstrap-token"
    Write-Host "Calling Worker admin: $uri" -ForegroundColor DarkGray
    $resp = Invoke-RestMethod -Uri $uri -Method Post -Headers @{
        Authorization = "Bearer $workerAdmin"
        "Content-Type"  = "application/json"
    } -Body $json
} else {
    Write-Host @"
Set either:
  CONTROL_PLANE_PUBLIC_URL + CONTROL_PLANE_BEARER_TOKEN in apps/control-plane/.env
or:
  WORKER_ADMIN_TOKEN (and optionally WORKER_ADMIN_URL) for direct Worker issuance.

Optional: CONTROL_PLANE_PUBLIC_URL=https://your-deployed-cp.example.com
"@ -ForegroundColor Yellow
    exit 1
}

$tok = $resp.provisioning_bootstrap_token
if (-not $tok) { $tok = $resp.ProvisioningBootstrapToken }
Write-Host ""
Write-Host "Give the customer this one-time bootstrap token (paste in Workspace settings or PROVISIONING_BOOTSTRAP_TOKEN):" -ForegroundColor Green
Write-Host $tok
Write-Host ""
if ($resp.expires_in_sec) { Write-Host ("Expires in (sec): " + $resp.expires_in_sec) }
if ($resp.provisioning_install_id) { Write-Host ("Install ID: " + $resp.provisioning_install_id) }
if ($resp.compose_env) {
    Write-Host ""
    Write-Host "Compose-style env (reference):" -ForegroundColor DarkGray
    $resp.compose_env | ConvertTo-Json
}
