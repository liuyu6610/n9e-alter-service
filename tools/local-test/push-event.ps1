param(
  [string]$Base = '',
  [string]$Token = ''
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

if ([string]::IsNullOrWhiteSpace($Token)) {
  $t = ''
  if ($null -ne $cfg -and $null -ne $cfg.push -and $null -ne $cfg.push.token) { $t = [string]$cfg.push.token }
  if ([string]::IsNullOrWhiteSpace($t)) { $t = 'push-token-123' }
  $Token = $t
}

$now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()

$evt = @{
  id = 10001
  hash = 'hash-demo-001'
  rule_id = 20001
  rule_name = 'demo_rule_cpu_high'
  severity = 3
  group_id = 30001
  group_name = 'demo_group'
  target_ident = 'host-01'
  target_note = ''
  first_trigger_time = $now
  trigger_time = $now
  cluster = 'local'
  tags_map = @{
    cluster = 'local'
    app = 'demo'
    instance = 'host-01'
    env = 'dev'
  }
}

$body = ($evt | ConvertTo-Json -Depth 10)

Invoke-RestMethod `
  -Method Post `
  -Uri "$Base/api/v1/events/ingest" `
  -Headers @{ 'X-Token' = $Token } `
  -ContentType 'application/json' `
  -Body $body
