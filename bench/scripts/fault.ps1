param(
    [ValidateSet("kafka-stop", "kafka-start", "mysql-stop", "mysql-start", "redis-stop", "redis-start", "kill-msg-core")]
    [string]$Action
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
$started = Get-Date

switch ($Action) {
    "kafka-stop"   { docker stop claran-kafka | Out-Null }
    "kafka-start"  { docker start claran-kafka | Out-Null }
    "mysql-stop"   { docker stop claran-mysql | Out-Null }
    "mysql-start"  { docker start claran-mysql | Out-Null }
    "redis-stop"   { docker stop claran-redis | Out-Null }
    "redis-start"  { docker start claran-redis | Out-Null }
    "kill-msg-core" {
        $procs = Get-Process -Name "msg-core-service" -ErrorAction SilentlyContinue
        if (-not $procs) { throw "msg-core-service not running" }
        $procs | Stop-Process -Force
    }
}

$elapsed = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)
Write-Output ("action={0} at={1} elapsed_s={2}" -f $Action, $started.ToString("o"), $elapsed)
