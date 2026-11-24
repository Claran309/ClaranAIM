# bench 环境变量：所有脚本共用，确保服务连接 bench 库而非开发库。

$ErrorActionPreference = "Stop"
$script:RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$script:BenchDB = "ClaranAIM_bench"

# 从仓库 .env 读取真实账号密码，避免脚本里写死密钥。
function Read-DotEnv([string]$Path) {
    $map = @{}
    if (-not (Test-Path -LiteralPath $Path)) { return $map }
    foreach ($line in Get-Content -LiteralPath $Path) {
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) { continue }
        $idx = $trimmed.IndexOf("=")
        if ($idx -le 0) { continue }
        $key = $trimmed.Substring(0, $idx).Trim()
        $value = $trimmed.Substring($idx + 1).Trim()
        $map[$key] = $value
    }
    return $map
}

$script:DotEnv = Read-DotEnv (Join-Path $script:RepoRoot ".env")
$script:BenchUser = if ($script:DotEnv["MYSQL_USER"]) { $script:DotEnv["MYSQL_USER"] } else { "claran" }
$script:BenchPassword = if ($script:DotEnv["MYSQL_PASSWORD"]) { $script:DotEnv["MYSQL_PASSWORD"] } else { $env:MYSQL_PASSWORD }
$script:BenchDSN = "${script:BenchUser}:${script:BenchPassword}@tcp(127.0.0.1:3306)/${script:BenchDB}?charset=utf8mb4&parseTime=True&loc=Local"

function Set-BenchEnv {
    $env:MYSQL_DSN = $script:BenchDSN
    $env:MYSQL_HOST = "127.0.0.1"
    $env:MYSQL_PORT = "3306"
    $env:MYSQL_USER = $script:BenchUser
    $env:MYSQL_PASSWORD = $script:BenchPassword
    $env:MYSQL_DATABASE = $script:BenchDB
    $env:REDIS_HOST = "127.0.0.1"
    $env:REDIS_PORT = "6379"
    $env:REDIS_DB = "0"
    $env:ETCD_ENDPOINTS = "http://127.0.0.1:2379"
    $env:KAFKA_ENABLED = "true"
    # 压测时关闭网关限流，否则单用户 120/分钟会把 C3 变成限流测试；
    # 正式报告必须注明该配置。
    $env:RATE_LIMIT_ENABLED = "false"
    # Kitex 服务端默认 1000 QPS / 1000 连接，会在压测中先于真实瓶颈触发；
    # bench 环境关闭服务端 limit，报告需注明这是压测配置。
    $env:RPC_MAX_QPS = "0"
    $env:RPC_MAX_CONNECTIONS = "0"
    # 固定 DB 池，避免高并发下连接churn耗尽 Windows 临时端口；
    # 该配置只由 bench 环境注入，不影响默认部署。
    $env:DB_MAX_OPEN_CONNS = "32"
    $env:DB_MAX_IDLE_CONNS = "32"
    $env:DB_CONN_MAX_LIFETIME_MINUTES = "30"
    # Phase 3 优化项：把 Outbox 单轮批量与轮询间隔做成可配置。
    $env:OUTBOX_BATCH_SIZE = "500"
    $env:OUTBOX_INTERVAL_MS = "200"
    $env:OUTBOX_CONCURRENCY = "8"
    $env:OBSERVABILITY_ENABLED = "true"
    $env:OBSERVABILITY_OTLP_ENDPOINT = "http://127.0.0.1:8082"
    $env:BENCH_PASSWORD = if ($env:BENCH_PASSWORD) { $env:BENCH_PASSWORD } else { "bench-local-pass" }
}

function Wait-Http([string]$Url, [int]$Seconds = 60) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri $Url -TimeoutSec 3 -UseBasicParsing -ErrorAction Stop
            if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) { return $true }
        } catch {
            Start-Sleep -Milliseconds 700
        }
    }
    return $false
}

