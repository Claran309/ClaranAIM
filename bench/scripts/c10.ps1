param(
    [ValidateSet("kafka", "mysql", "kill", "all")]
    [string]$Scenario = "all"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-BenchEnv
Set-Location $script:RepoRoot
New-Item -ItemType Directory -Force -Path "bench/logs" | Out-Null

$tokens = "bench/results/seed/tokens.txt"
if (-not (Test-Path $tokens)) { throw "missing tokens" }

function Get-FreeGB {
    $os = Get-CimInstance Win32_OperatingSystem
    return [math]::Round($os.FreePhysicalMemory / 1MB, 2)
}

function Reset-Jaeger {
    docker compose -f deployment/docker/docker-compose.infra.yaml --env-file .env up -d --force-recreate jaeger 2>&1 | Out-Null
    Start-Sleep -Seconds 4
}

function Invoke-Sql([string]$Sql) {
    $tmp = Join-Path $env:TEMP ("c10-{0}.sql" -f ([guid]::NewGuid().ToString("N")))
    Set-Content -LiteralPath $tmp -Value $Sql -Encoding utf8
    docker cp $tmp claran-mysql:/tmp/c10.sql | Out-Null
    $out = docker exec claran-mysql sh -c "mysql -u$($script:BenchUser) -p$($script:BenchPassword) -D ClaranAIM_bench -N -B -e 'source /tmp/c10.sql'" 2>&1
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    return @($out | Where-Object { $_ -and $_ -notmatch '^mysql:' })
}

function Get-Pending {
    $r = Invoke-Sql "SELECT COUNT(*) FROM event_outbox WHERE status IN ('pending','retrying');"
    return [int64]($r[-1])
}

function Start-Load([string]$runId, [int]$seconds, [int]$vus = 5) {
    $args = @(
        "-scenario=c3",
        "-concurrency=$vus",
        "-duration=${seconds}s",
        "-warmup=0s",
        "-run-id=$runId",
        "-out=bench/results",
        "-tokens=$tokens"
    )
    return Start-Process -FilePath "bench/bin/loadgen.exe" -WorkingDirectory $script:RepoRoot `
        -WindowStyle Hidden -PassThru -ArgumentList $args `
        -RedirectStandardOutput "bench/logs/$runId.out.log" `
        -RedirectStandardError "bench/logs/$runId.err.log"
}

function Wait-Login([int]$seconds = 180) {
    $m = Get-Content "bench/results/seed/manifest.json" -Raw | ConvertFrom-Json
    $body = @{ username = $m.users[0].username; password = $env:BENCH_PASSWORD } | ConvertTo-Json
    for ($i = 0; $i -lt $seconds; $i++) {
        try {
            $r = Invoke-WebRequest "http://127.0.0.1:18080/api/v1/user/login" -Method Post `
                -ContentType "application/json" -Body $body -TimeoutSec 3 -UseBasicParsing -SkipHttpErrorCheck
            if ($r.StatusCode -eq 200) { return $true }
        } catch {}
        Start-Sleep -Seconds 1
    }
    return $false
}

function Wait-Send([int]$seconds = 180) {
    $m = Get-Content "bench/results/seed/manifest.json" -Raw | ConvertFrom-Json
    $body = @{ username = $m.users[0].username; password = $env:BENCH_PASSWORD } | ConvertTo-Json
    $conv = $m.private_conversations[0].id
    $token = ""
    for ($i = 0; $i -lt $seconds; $i++) {
        if ([string]::IsNullOrEmpty($token)) {
            try {
                $login = Invoke-RestMethod "http://127.0.0.1:18080/api/v1/user/login" -Method Post `
                    -ContentType "application/json" -Body $body -TimeoutSec 5
                $token = $login.data.access_token
            } catch {
                Start-Sleep -Seconds 1
                continue
            }
        }
        $seq = [guid]::NewGuid().ToString("N").Substring(0, 10)
        $msg = @{
            conversation_id = "$conv"
            content = "c10-probe-$seq"
            msg_type = "text"
            client_msg_id = "c10-probe-$seq"
        } | ConvertTo-Json
        try {
            $r = Invoke-RestMethod "http://127.0.0.1:18080/api/v1/message/send" -Method Post `
                -Headers @{ Authorization = "Bearer $token" } -ContentType "application/json" `
                -Body $msg -TimeoutSec 5
            if ($r.code -eq 0 -and $r.data.success) { return $true }
        } catch {
            $token = ""
        }
        Start-Sleep -Seconds 1
    }
    return $false
}

function Wait-PendingZero([int]$seconds = 600) {
    $start = Get-Date
    for ($i = 0; $i -lt $seconds; $i += 5) {
        $pending = Get-Pending
        if ($pending -eq 0) {
            return @{ drained = $true; seconds = [math]::Round(((Get-Date) - $start).TotalSeconds, 1) }
        }
        Start-Sleep -Seconds 5
    }
    return @{ drained = $false; seconds = $seconds }
}

function Save-Result([string]$name, $result) {
    $dir = "bench/results/$name"
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $json = $result | ConvertTo-Json -Depth 6
    Set-Content -LiteralPath (Join-Path $dir "fault.json") -Value $json -Encoding utf8
    $json
}

$all = @()

if ($Scenario -eq "kafka" -or $Scenario -eq "all") {
    Reset-Jaeger
    $runId = "AIM-P1-C10-kafka-" + (Get-Date -Format "HHmmss")
    $start = Get-Date
    $proc = Start-Load $runId 120 5
    Start-Sleep -Seconds 15
    docker stop claran-kafka | Out-Null
    $faultAt = Get-Date
    Start-Sleep -Seconds 60
    docker start claran-kafka | Out-Null
    $restartAt = Get-Date
    $proc.WaitForExit()
    $drain = Wait-PendingZero 600
    $s = Get-Content "bench/results/$runId/summary.json" -Raw | ConvertFrom-Json
    & "$PSScriptRoot/reconcile.ps1" -RunId $runId -ExpectSent $s.ok | Out-Null
    $rc = Get-Content "bench/results/$runId/reconcile.json" -Raw | ConvertFrom-Json
    $r = [ordered]@{
        scenario = "kafka_down_60s"
        run_id = $runId
        fault_injected_at = $faultAt.ToString("o")
        kafka_restarted_at = $restartAt.ToString("o")
        downtime_s = 60
        sent = $s.ok
        failed_requests = $s.failed
        messages = $rc.messages
        outbox_published = $rc.outbox_published
        outbox_pending_after_drain = (Get-Pending)
        drained = $drain.drained
        drain_seconds = $drain.seconds
        missing = $rc.missing_messages
        duplicate_outbox = $rc.duplicate_outbox
    }
    $all += Save-Result $runId $r
}

if ($Scenario -eq "mysql" -or $Scenario -eq "all") {
    Reset-Jaeger
    $runId = "AIM-P1-C10-mysql-" + (Get-Date -Format "HHmmss")
    $proc = Start-Load $runId 100 5
    Start-Sleep -Seconds 15
    docker stop claran-mysql | Out-Null
    $faultAt = Get-Date
    Start-Sleep -Seconds 30
    docker start claran-mysql | Out-Null
    $restartAt = Get-Date
    $healthy = $false
    for ($i = 0; $i -lt 60; $i++) {
        $h = docker inspect claran-mysql --format '{{.State.Health.Status}}' 2>$null
        if ($h -eq "healthy") { $healthy = $true; break }
        Start-Sleep -Seconds 2
    }
    $recovered = Wait-Send 120
    $recoveryAt = Get-Date
    $proc.WaitForExit()
    $s = Get-Content "bench/results/$runId/summary.json" -Raw | ConvertFrom-Json
    & "$PSScriptRoot/reconcile.ps1" -RunId $runId -ExpectSent $s.ok | Out-Null
    $rc = Get-Content "bench/results/$runId/reconcile.json" -Raw | ConvertFrom-Json
    $r = [ordered]@{
        scenario = "mysql_down_30s"
        run_id = $runId
        fault_injected_at = $faultAt.ToString("o")
        mysql_restarted_at = $restartAt.ToString("o")
        health_recovered = $healthy
        api_recovered = $recovered
        recovery_seconds = [math]::Round(($recoveryAt - $restartAt).TotalSeconds, 1)
        sent = $s.ok
        failed_requests = $s.failed
        messages = $rc.messages
        missing = $rc.missing_messages
        duplicate_outbox = $rc.duplicate_outbox
    }
    $all += Save-Result $runId $r
}

if ($Scenario -eq "kill" -or $Scenario -eq "all") {
    Reset-Jaeger
    $runId = "AIM-P1-C10-kill-" + (Get-Date -Format "HHmmss")
    $proc = Start-Load $runId 80 5
    Start-Sleep -Seconds 15
    $killAt = Get-Date
    Get-Process -Name msg-core-service -ErrorAction SilentlyContinue | Stop-Process -Force
    $restartAt = Get-Date
    Start-Process -FilePath "bench/bin/msg-core-service.exe" -WorkingDirectory $script:RepoRoot -WindowStyle Hidden `
        -RedirectStandardOutput "bench/logs/msg-core-service.out.log" `
        -RedirectStandardError "bench/logs/msg-core-service.err.log" | Out-Null
    $recovered = Wait-Send 120
    $recoveryAt = Get-Date
    $proc.WaitForExit()
    $s = Get-Content "bench/results/$runId/summary.json" -Raw | ConvertFrom-Json
    & "$PSScriptRoot/reconcile.ps1" -RunId $runId -ExpectSent $s.ok | Out-Null
    $rc = Get-Content "bench/results/$runId/reconcile.json" -Raw | ConvertFrom-Json
    $r = [ordered]@{
        scenario = "kill9_msg_core"
        run_id = $runId
        killed_at = $killAt.ToString("o")
        restarted_at = $restartAt.ToString("o")
        api_recovered = $recovered
        recovery_seconds = [math]::Round(($recoveryAt - $restartAt).TotalSeconds, 1)
        sent = $s.ok
        failed_requests = $s.failed
        messages = $rc.messages
        missing = $rc.missing_messages
        duplicate_outbox = $rc.duplicate_outbox
    }
    $all += Save-Result $runId $r
}

$all
