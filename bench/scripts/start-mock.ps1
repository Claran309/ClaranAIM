param(
    [string]$Addr = "127.0.0.1:18090",
    [int]$LatencyMs = 150
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-Location $script:RepoRoot
New-Item -ItemType Directory -Force -Path "bench/logs" | Out-Null

$port = ($Addr -split ":")[-1]
$listening = Get-NetTCPConnection -LocalPort ([int]$port) -State Listen -ErrorAction SilentlyContinue
if ($listening) {
    Write-Host "mockllm already listening on $Addr"
    return
}

$exe = Join-Path $script:RepoRoot "bench/bin/mockllm.exe"
if (-not (Test-Path $exe)) {
    go build -o $exe ./bench/mockllm
    if ($LASTEXITCODE -ne 0) { throw "build mockllm failed" }
}

$env:MOCK_LLM_LATENCY_MS = "$LatencyMs"
Start-Process -FilePath $exe -WorkingDirectory $script:RepoRoot -WindowStyle Hidden `
    -ArgumentList @("-addr=$Addr") `
    -RedirectStandardOutput "bench/logs/mockllm.out.log" `
    -RedirectStandardError "bench/logs/mockllm.err.log" | Out-Null

if (-not (Wait-Http "http://$Addr/health" 20)) {
    throw "mockllm did not become ready"
}
Write-Host "mockllm ready at $Addr"
