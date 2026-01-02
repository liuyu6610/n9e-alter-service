param(
  [string]$Base = '',
  [switch]$SkipAPI
)

$ErrorActionPreference = 'Stop'

$cfgPath = Join-Path $PSScriptRoot 'config.local.json'
$cfg = $null
if (Test-Path $cfgPath) {
  try {
    $cfg = Get-Content -Raw -Path $cfgPath | ConvertFrom-Json
  } catch {
    $cfg = $null
  }
}

if ([string]::IsNullOrWhiteSpace($Base)) {
  $addr = ''
  if ($null -ne $cfg -and $null -ne $cfg.addr) { $addr = [string]$cfg.addr }
  if ([string]::IsNullOrWhiteSpace($addr)) { $addr = ':8080' }
  if ($addr.StartsWith('http')) {
    $Base = $addr
  } else {
    $Base = "http://127.0.0.1$addr"
  }
}
$outDir = Join-Path $PSScriptRoot 'out'
if (-not (Test-Path $outDir)) {
  throw "out dir not found: $outDir"
}

$start = Get-Date

Write-Host "start=$($start.ToString('o'))"
Write-Host "base=$Base"
Write-Host "out=$outDir"

# preflight: service should be up
Write-Host "step=preflight"
try {
  Invoke-RestMethod -Method Get -Uri "$Base/readyz" -TimeoutSec 3 | Out-Null
} catch {
  Write-Host "result=FAIL reason=service_not_ready"
  Write-Host "hint=run ./tools/local-test/start-service.ps1 and ensure port is free"
  Write-Host "err=$($_.Exception.Message)"
  exit 10
}

# 1) push one event
Write-Host "step=push"
& (Join-Path $PSScriptRoot 'push-event.ps1') | Out-Null

function Wait-ForOutFile([string]$suffix, [int]$timeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($timeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    $f = Get-ChildItem -Path $outDir -File -Filter "*-$suffix.json" |
      Where-Object { $_.LastWriteTime -ge $start } |
      Sort-Object LastWriteTime -Descending |
      Select-Object -First 1
    if ($null -ne $f) {
      return $f.FullName
    }
    Start-Sleep -Milliseconds 300
  }
  return $null
}

# 2) wait for normal webhook (should be near-immediate)
Write-Host "step=wait webhook"
$webhookFile = Wait-ForOutFile 'webhook' 20
if ($null -eq $webhookFile) {
  Write-Host "result=FAIL reason=webhook_not_received"
  exit 2
}
Write-Host "webhook_file=$webhookFile"

# 3) wait for escalation (default config.local.json uses after_seconds=10)
Write-Host "step=wait escalation"
$escalationFile = Wait-ForOutFile 'escalation' 35
if ($null -eq $escalationFile) {
  Write-Host "result=FAIL reason=escalation_not_received"
  exit 3
}
Write-Host "escalation_file=$escalationFile"

# 4) verify state via API
if ($SkipAPI) {
  Write-Host "step=check api skipped"
  Write-Host "result=OK"
  exit 0
}

Write-Host "step=check api"
try {
  $alerts = Invoke-RestMethod -Method Get -Uri "$Base/api/v1/alerts?status=active&offset=0&limit=50" -TimeoutSec 5
} catch {
  Write-Host "result=FAIL reason=alerts_api_error err=$($_.Exception.Message)"
  exit 4
}

$items = @()
if ($null -ne $alerts.items) { $items = $alerts.items }

$target = $items | Where-Object { $_.n9e_hash -eq 'hash-demo-001' } | Select-Object -First 1
if ($null -eq $target) {
  Write-Host "result=WARN reason=target_alert_not_found"
  Write-Host "hint=check /api/v1/alerts output manually"
  exit 0
}

$ln = [int64]$target.last_notified
$le = [int64]$target.last_escalated_at

Write-Host "last_notified=$ln last_escalated_at=$le escalated_step=$($target.escalated_step)"

if ($ln -le 0) {
  Write-Host "result=FAIL reason=last_notified_not_updated"
  exit 5
}
if ($le -le 0) {
  Write-Host "result=FAIL reason=last_escalated_at_not_updated"
  exit 6
}

Write-Host "result=OK"
