$ErrorActionPreference = 'Stop'

$repo = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$configPath = Join-Path $PSScriptRoot 'config.local.json'

if (-not (Test-Path $configPath)) {
  throw "config not found: $configPath"
}

Write-Host "repo=$repo"
Write-Host "config=$configPath"
Write-Host "tip=Press Ctrl+C to stop"

Push-Location $repo
try {
  & go run . -config $configPath
} finally {
  Pop-Location
}
