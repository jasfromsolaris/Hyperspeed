# Sync apps/control-plane into a sibling directory for a small private Git repo (Render, etc.).
# Usage: .\scripts\export-control-plane-mirror.ps1
# Optional: $env:CONTROL_PLANE_MIRROR_DST = "D:\repos\hyperspeed-control-plane-private"
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Src = Join-Path $RepoRoot "apps\control-plane"
$Dst = if ($env:CONTROL_PLANE_MIRROR_DST) {
  $env:CONTROL_PLANE_MIRROR_DST
} else {
  Join-Path (Split-Path $RepoRoot -Parent) "hyperspeed-control-plane-private"
}

if (-not (Test-Path $Src)) { throw "Missing $Src" }
New-Item -ItemType Directory -Force -Path $Dst | Out-Null
Get-ChildItem -Path $Dst -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
$exclude = @(".env", "data", "render.standalone.yaml", "README.mirror.md")
Get-ChildItem -Path $Src -Force | ForEach-Object {
  if ($exclude -contains $_.Name) { return }
  Copy-Item -Path $_.FullName -Destination (Join-Path $Dst $_.Name) -Recurse -Force
}
Copy-Item (Join-Path $Src "render.standalone.yaml") (Join-Path $Dst "render.yaml") -Force
Copy-Item (Join-Path $Src "README.mirror.md") (Join-Path $Dst "README.md") -Force
Write-Host "Exported to $Dst"
Write-Host "Next: cd there, git init, add remote, commit, push."
