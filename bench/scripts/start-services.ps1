param(
    [string]$BinDir = "bench/bin",
    [string]$LogDir = "bench/logs"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-BenchEnv
Set-Location $script:RepoRoot

New-Item -ItemType Directory -Force -Path $BinDir, $LogDir | Out-Null

$services = @(
    @{ Name = "user-service";        Cmd = "./cmd/user-service";        Addr = "127.0.0.1:9101" },
    @{ Name = "group-service";       Cmd = "./cmd/group-service";       Addr = "127.0.0.1:9002" },
    @{ Name = "msg-core-service";    Cmd = "./cmd/msg-core-service";    Addr = "127.0.0.1:9003" },
    @{ Name = "msg-history-service"; Cmd = "./cmd/msg-history-service"; Addr = "127.0.0.1:9004" },
    @{ Name = "websocket-gateway";   Cmd = "./cmd/websocket-gateway";   Addr = "127.0.0.1:8081" },
    @{ Name = "api-gateway";         Cmd = "./cmd/api-gateway";         Addr = "127.0.0.1:18080" }
)

foreach ($svc in $services) {
    $exe = Join-Path $BinDir ($svc.Name + ".exe")
    if (-not (Test-Path $exe)) {
        Write-Host "building $($svc.Name)..."
        go build -o $exe $svc.Cmd
        if ($LASTEXITCODE -ne 0) { throw "build failed: $($svc.Name)" }
    }
}

foreach ($svc in $services) {
    $exe = Join-Path $BinDir ($svc.Name + ".exe")
    $out = Join-Path $LogDir ($svc.Name + ".out.log")
    $err = Join-Path $LogDir ($svc.Name + ".err.log")
    $existing = Get-Process -Name $svc.Name -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Host "$($svc.Name) already running (pid=$($existing.Id -join ','))"
        continue
    }
    Write-Host "starting $($svc.Name) on $($svc.Addr)"
    Start-Process -FilePath $exe -WorkingDirectory $script:RepoRoot -WindowStyle Hidden `
        -RedirectStandardOutput $out -RedirectStandardError $err | Out-Null
}

Write-Host "waiting for api-gateway..."
if (-not (Wait-Http "http://127.0.0.1:18080/api/v1/user/login" 60)) {
    Write-Warning "api-gateway not responding yet; check bench/logs"
} else {
    Write-Host "api-gateway is responding"
}

