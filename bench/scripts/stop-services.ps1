param(
    [switch]$KeepInfra
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")

$names = @("api-gateway", "websocket-gateway", "msg-history-service", "msg-core-service", "group-service", "user-service")
foreach ($name in $names) {
    $procs = Get-Process -Name $name -ErrorAction SilentlyContinue
    if ($procs) {
        Write-Host "stopping $name (pid=$($procs.Id -join ','))"
        $procs | Stop-Process -Force
    }
}

