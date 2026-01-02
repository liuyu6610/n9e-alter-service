$ErrorActionPreference = 'Stop'

$port = 18080
$prefix = "http://127.0.0.1:$port/"
$outDir = Join-Path $PSScriptRoot 'out'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add($prefix)
$listener.Start()

Write-Host "listening: $prefix"
Write-Host "out: $outDir"

try {
  while ($true) {
    $ctx = $listener.GetContext()
    $req = $ctx.Request
    $res = $ctx.Response

    $ts = [DateTimeOffset]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')
    $path = $req.Url.AbsolutePath.Trim('/').Replace('/','_')
    if ([string]::IsNullOrWhiteSpace($path)) { $path = 'root' }
    $file = Join-Path $outDir "$ts-$path.json"

    $reader = [System.IO.StreamReader]::new($req.InputStream, [System.Text.Encoding]::UTF8)
    $body = $reader.ReadToEnd()
    $reader.Close()

    [System.IO.File]::WriteAllText($file, $body, [System.Text.Encoding]::UTF8)

    Write-Host "----"
    Write-Host "time=$ts method=$($req.HttpMethod) path=$($req.Url.AbsolutePath) len=$($body.Length)"
    Write-Host "saved=$file"
    if ($body.Length -le 8000) {
      Write-Host $body
    } else {
      Write-Host ($body.Substring(0,8000) + "...")
    }

    $res.StatusCode = 200
    $respBody = '{"ok":true}'
    $buf = [System.Text.Encoding]::UTF8.GetBytes($respBody)
    $res.ContentType = 'application/json; charset=utf-8'
    $res.ContentLength64 = $buf.Length
    $res.OutputStream.Write($buf, 0, $buf.Length)
    $res.OutputStream.Close()
  }
} finally {
  $listener.Stop()
  $listener.Close()
}
