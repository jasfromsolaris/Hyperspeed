# Point this repo at .githooks (pre-push blocks accidental push to public OSS Hyperspeed).
# Run once per clone from the repository root:  .\scripts\setup-git-hooks.ps1

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root
git config core.hooksPath .githooks
Write-Host "core.hooksPath set to .githooks — pre-push protection active for this clone."
