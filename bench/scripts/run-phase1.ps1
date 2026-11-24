param(
    [string]$Phase = "c2",
    [int[]]$Levels = @(20),
    [int]$Seconds = 300,
    [int]$WarmupSeconds = 60,
    [int]$Repeats = 3,
    [string]$Prefix = "AIM-P1",
    [int]$GroupLimit = 0,
    [int]$PrivateLimit = 0,
    [int]$MinFreePorts = 6000,
    [switch]$Adaptive = $true,
    [switch]$WS
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-BenchEnv
Set-Location $script:RepoRoot

function Get-TimeWaitCount {
    return @(Get-NetTCPConnection -State TimeWait -ErrorAction SilentlyContinue).Count
}

function Wait-PortBudget([int]$MinFree) {
    for ($i = 0; $i -lt 120; $i++) {
        $tw = Get-TimeWaitCount
        $free = 16384 - $tw
        if ($free -ge $MinFree) { return }
        Write-Host ("TIME_WAIT={0}, free={1}, waiting..." -f $tw, $free)
        Start-Sleep -Seconds 10
    }
    throw "port budget not recovered"
}

function Get-FreeGB {
    $os = Get-CimInstance Win32_OperatingSystem
    return [math]::Round($os.FreePhysicalMemory / 1MB, 2)
}

function Get-AdaptiveCap([double]$FreeGB) {
    # 本机实测：20VU 在 1.2GB 可用内存下仍可稳定；50VU 需要更多余量。
    if ($FreeGB -ge 2.5) { return 50 }
    if ($FreeGB -ge 1.5) { return 20 }
    if ($FreeGB -ge 1.0) { return 10 }
    return 0
}

$loadgen = Join-Path $script:RepoRoot "bench/bin/loadgen.exe"
if (-not (Test-Path $loadgen)) {
    go build -o $loadgen ./bench/loadgen
    if ($LASTEXITCODE -ne 0) { throw "build loadgen failed" }
}

$tokens = "bench/results/seed/tokens.txt"
if (-not (Test-Path $tokens)) { throw "missing tokens: $tokens" }

$summary = @()
foreach ($level in $Levels) {
    for ($rep = 1; $rep -le $Repeats; $rep++) {
        Wait-PortBudget $MinFreePorts
        $freeGB = Get-FreeGB
        $effectiveLevel = $level
        $resourceNote = "normal"
        if ($Adaptive) {
            $cap = Get-AdaptiveCap $freeGB
            if ($cap -eq 0) {
                Write-Host ("free={0}GB below 1.5GB, skipping {1}VU" -f $freeGB, $level)
                $resourceNote = "resource_limited_skip"
                $summary += [pscustomobject]@{
                    run_id = "-"; phase = $Phase; level = $level; repeat = $rep;
                    sent = 0; ok = 0; failed = 0; rps = 0; p50 = 0; p95 = 0; p99 = 0; ws_recv = 0;
                    path = ""; note = $resourceNote; free_gb = $freeGB
                }
                continue
            }
            if ($level -gt $cap) {
                $effectiveLevel = $cap
                $resourceNote = "resource_cap_$cap"
                Write-Host ("free={0}GB, cap {1}VU -> {2}VU" -f $freeGB, $level, $cap)
            }
        }
        $runId = "{0}-{1}-{2}vu-r{3}" -f $Prefix, $Phase.ToUpper(), $effectiveLevel, $rep
        if ($effectiveLevel -ne $level) { $runId = "$runId-capped" }
        Write-Host ("=== run {0} ===" -f $runId)
        $args = @(
            "-scenario=$Phase",
            "-concurrency=$effectiveLevel",
            "-duration=${Seconds}s",
            "-warmup=${WarmupSeconds}s",
            "-run-id=$runId",
            "-out=bench/results",
            "-tokens=$tokens"
        )
        if ($GroupLimit -gt 0) { $args += "-group-limit=$GroupLimit" }
        if ($PrivateLimit -gt 0) { $args += "-private-limit=$PrivateLimit" }
        if ($WS) { $args += "-ws-recv" }
        & $loadgen @args 2>&1 | Select-Object -Last 3 | ForEach-Object { Write-Host $_ }
        $path = "bench/results/$runId/summary.json"
        if (-not (Test-Path $path)) { throw "missing summary: $path" }
        $s = Get-Content $path -Raw | ConvertFrom-Json
        $summary += [pscustomobject]@{
            run_id   = $s.run_id
            phase    = $s.scenario
            level    = $effectiveLevel
            repeat   = $rep
            sent     = $s.sent
            ok       = $s.ok
            failed   = $s.failed
            rps      = $s.rps
            p50      = $s.p50_ms
            p95      = $s.p95_ms
            p99      = $s.p99_ms
            ws_recv  = $s.ws_recv
            path     = $path
            note     = $resourceNote
            free_gb  = $freeGB
        }
        Start-Sleep -Seconds 5
    }
}

$csv = "bench/results/$Prefix-$($Phase.ToUpper())-runs.csv"
$summary | Export-Csv -LiteralPath $csv -NoTypeInformation -Encoding utf8
$summary | Format-Table -AutoSize
Write-Host "runs csv: $csv"
