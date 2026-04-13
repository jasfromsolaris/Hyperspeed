# Backup Hyperspeed Docker data: Postgres dump (required) + optional object mirror.
# Intended for Docker Desktop/local workflows. Production backups are usually scheduled on the Linux host.
# Run from anywhere: .\scripts\backup-hyperspeed.ps1

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

$backupDir = if ($env:BACKUP_DIR) { $env:BACKUP_DIR } else { Join-Path $root "backups" }
$bucketName = if ($env:BUCKET_NAME) { $env:BUCKET_NAME } else { "hyperspeed-files" }
$retainCount = $env:RETAIN_COUNT
$stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$outFile = Join-Path $backupDir "hyperspeed-$stamp.sql.gz"

New-Item -ItemType Directory -Force -Path $backupDir | Out-Null

# gzip is available via Git Bash / WSL. If not present, use plain SQL output.
$gzip = Get-Command gzip -ErrorAction SilentlyContinue
if ($gzip) {
    $cmd = "docker compose exec -T postgres pg_dump -U hyperspeed -d hyperspeed | gzip -c > `"$outFile`""
} else {
    $outFile = Join-Path $backupDir "hyperspeed-$stamp.sql"
    $cmd = "docker compose exec -T postgres pg_dump -U hyperspeed -d hyperspeed > `"$outFile`""
}
Write-Host "Writing Postgres backup to: $outFile"
powershell -NoProfile -Command $cmd

$bytes = (Get-Item $outFile).Length
Write-Host "Postgres backup complete ($bytes bytes)"

if ($retainCount) {
    if ($retainCount -match '^\d+$') {
        $files = Get-ChildItem -Path $backupDir -Filter "hyperspeed-*.sql*" | Sort-Object Name -Descending
        if ($files.Count -gt [int]$retainCount) {
            $files | Select-Object -Skip ([int]$retainCount) | ForEach-Object {
                Remove-Item -Force $_.FullName
                Write-Host "Pruned old backup: $($_.FullName)"
            }
        }
    } else {
        Write-Host "RETAIN_COUNT is not numeric; skipping prune"
    }
}

$mc = Get-Command mc -ErrorAction SilentlyContinue
if ($mc -and $env:MC_SOURCE_ALIAS -and $env:MC_TARGET_ALIAS) {
    Write-Host "Mirroring object storage bucket: $($env:MC_SOURCE_ALIAS)/$bucketName -> $($env:MC_TARGET_ALIAS)/$bucketName"
    mc mirror --overwrite "$($env:MC_SOURCE_ALIAS)/$bucketName" "$($env:MC_TARGET_ALIAS)/$bucketName"
    Write-Host "Object mirror complete"
} else {
    Write-Host "Object mirror skipped. Set MC_SOURCE_ALIAS and MC_TARGET_ALIAS (and configure mc aliases) to enable."
}

Write-Host "Done. Restore Postgres with:"
if ($gzip) {
    Write-Host "  gzip -dc `"$outFile`" | docker compose exec -T postgres psql -U hyperspeed -d hyperspeed"
} else {
    Write-Host "  Get-Content -Raw `"$outFile`" | docker compose exec -T postgres psql -U hyperspeed -d hyperspeed"
}
