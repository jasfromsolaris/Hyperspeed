# Point this repo at .githooks for shared local hooks (optional).
# Run once per clone from the repository root:  .\scripts\setup-git-hooks.ps1

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root
git config core.hooksPath .githooks
Write-Host "core.hooksPath set to .githooks"
