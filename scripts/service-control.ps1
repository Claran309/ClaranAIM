param(
    [ValidateSet("start", "stop", "restart", "status")]
    [string]$Action = "start",
    [switch]$NoDocker
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$script:ProcessById = $null
$script:DockerPortOwners = $null
$script:RunStartedAt = Get-Date

$services = @(
    @{ Step = 2;  Name = "user-service"; Port = 9101; Path = "cmd/user-service/main.go" },
    @{ Step = 3;  Name = "group-service"; Port = 9002; Path = "cmd/group-service/main.go" },
    @{ Step = 4;  Name = "msg-core-service"; Port = 9003; Path = "cmd/msg-core-service/main.go" },
    @{ Step = 5;  Name = "msg-history-service"; Port = 9004; Path = "cmd/msg-history-service/main.go" },
    @{ Step = 6;  Name = "file-service"; Port = 9005; Path = "cmd/file-service/main.go" },
    @{ Step = 7;  Name = "memory-service"; Port = 9008; Path = "cmd/memory-service/main.go" },
    @{ Step = 8;  Name = "settings-service"; Port = 9009; Path = "cmd/settings-service/main.go" },
    @{ Step = 9;  Name = "web-search-service"; Port = 9114; Path = "cmd/web-search-service/main.go" },
    @{ Step = 10; Name = "rag-service"; Port = 9112; Path = "cmd/rag-service/main.go" },
    @{ Step = 11; Name = "knowledge-service"; Port = 9113; Path = "cmd/knowledge-service/main.go" },
    @{ Step = 12; Name = "conversation-intelligence-service"; Port = 9015; Path = "cmd/conversation-intelligence-service/main.go" },
    @{ Step = 13; Name = "mcp-gateway-service"; Port = 9016; Path = "cmd/mcp-gateway-service/main.go" },
    @{ Step = 14; Name = "admin-service"; Port = 9017; Path = "cmd/admin-service/main.go" },
    @{ Step = 15; Name = "agent-runtime-service"; Port = 9007; Path = "cmd/agent-runtime-service/main.go" },
    @{ Step = 16; Name = "agent-manager-service"; Port = 9006; Path = "cmd/agent-manager-service/main.go" },
    @{ Step = 17; Name = "api-gateway"; Port = 18080; Path = "cmd/api-gateway/main.go" },
    @{ Step = 18; Name = "websocket-gateway"; Port = 8081; Path = "cmd/websocket-gateway/main.go" }
)

function Normalize-Text([string]$Text) {
    if ([string]::IsNullOrWhiteSpace($Text)) { return "" }
    return $Text.ToLowerInvariant().Replace("\", "/")
}

function Initialize-ProcessCache {
    if ($script:ProcessById) { return }
    $script:ProcessById = @{}
    foreach ($proc in @(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue)) {
        $script:ProcessById[[int]$proc.ProcessId] = $proc
    }
}

function Reset-ProcessCache {
    $script:ProcessById = $null
}

function Get-CachedProcess([int]$ProcessId) {
    Initialize-ProcessCache
    if ($script:ProcessById.ContainsKey($ProcessId)) {
        return $script:ProcessById[$ProcessId]
    }
    return $null
}

function Get-DockerPortOwners {
    if ($script:DockerPortOwners) { return $script:DockerPortOwners }
    $script:DockerPortOwners = @{}
    try {
        foreach ($line in @(docker ps --format "{{.Names}} {{.Ports}}" 2>$null)) {
            foreach ($match in [regex]::Matches($line, "(?:0\.0\.0\.0|\[::\]):(?<port>\d+)->")) {
                $port = [int]$match.Groups["port"].Value
                if (-not $script:DockerPortOwners.ContainsKey($port)) {
                    $script:DockerPortOwners[$port] = New-Object System.Collections.Generic.List[string]
                }
                $script:DockerPortOwners[$port].Add($line)
            }
        }
    } catch {
    }
    return $script:DockerPortOwners
}

function Get-ProcessTreeCommand([int]$ProcessId) {
    $parts = New-Object System.Collections.Generic.List[string]
    $current = Get-CachedProcess $ProcessId
    $guard = 0
    while ($current -and $guard -lt 8) {
        if ($current.CommandLine) {
            $parts.Add($current.CommandLine)
        } elseif ($current.Name) {
            $parts.Add($current.Name)
        }
        if (-not $current.ParentProcessId) { break }
        $current = Get-CachedProcess ([int]$current.ParentProcessId)
        $guard++
    }
    return ($parts -join " ")
}

function Get-PortOwnerHint([int]$ProcessId, [int]$Port) {
    $proc = Get-CachedProcess $ProcessId
    if (-not $proc) { return "" }
    $command = if ($proc.CommandLine) { $proc.CommandLine } else { $proc.Name }
    $hint = $command
    if ($proc.Name -in @("wslrelay.exe", "com.docker.backend.exe")) {
        $owners = Get-DockerPortOwners
        if ($owners.ContainsKey($Port)) {
            $hint = "$hint ; docker=$($owners[$Port] -join ' | ')"
        }
    }
    return $hint
}

function Get-PortListeners([int]$Port) {
    @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
}

function Get-ServiceGoRunProcesses($Service) {
    Reset-ProcessCache
    Initialize-ProcessCache
    $expectedPath = Normalize-Text $Service.Path
    @($script:ProcessById.Values | Where-Object {
        $_.Name -eq "go.exe" -and
        $_.CommandLine -and
        (Normalize-Text $_.CommandLine).Contains($expectedPath)
    })
}

function Get-TodayErrLogPath {
    $date = Get-Date -Format "yyyy-MM-dd"
    return Join-Path $repoRoot "logs\ERR\$date\ERR.log"
}

function Get-NewFatalLogLines {
    $errPath = Get-TodayErrLogPath
    if (-not (Test-Path $errPath)) { return @() }
    $lines = @(Get-Content $errPath -Tail 300 -ErrorAction SilentlyContinue)
    $newLines = New-Object System.Collections.Generic.List[string]
    foreach ($line in $lines) {
        if ($line -notmatch "FATAL|ERROR|ERR") { continue }
        $matched = [regex]::Match($line, "\]\s+(?<ts>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3})")
        if ($matched.Success) {
            try {
                $ts = [datetime]::ParseExact($matched.Groups["ts"].Value, "yyyy-MM-dd HH:mm:ss.fff", $null)
                if ($ts -ge $script:RunStartedAt.AddSeconds(-1)) {
                    $newLines.Add($line)
                }
            } catch {
            }
        }
    }
    return @($newLines)
}

function Report-NewFatalLogs {
    $lines = @(Get-NewFatalLogLines)
    if ($lines.Count -eq 0) { return $false }
    Write-Host ""
    Write-Host "[ERR] New fatal/error logs were written during this startup:"
    foreach ($line in $lines) {
        Write-Host "  $line"
    }
    return $true
}

function Report-ServiceRuntimeIssues {
    Reset-ProcessCache
    $issues = New-Object System.Collections.Generic.List[string]
    foreach ($svc in $services) {
        $runtimes = @(Get-ServiceRuntime $svc)
        $expected = @($runtimes | Where-Object { $_.IsExpected })
        if ($expected.Count -gt 0) { continue }
        if ($runtimes.Count -gt 0) {
            $issues.Add("$($svc.Name) port $($svc.Port) is occupied, but not by this repo service")
        } else {
            $issues.Add("$($svc.Name) port $($svc.Port) is not listening")
        }
    }
    if ($issues.Count -eq 0) { return $false }
    Write-Host ""
    Write-Host "[ERR] Startup verification found service issues:"
    foreach ($issue in $issues) {
        Write-Host "  $issue"
    }
    return $true
}

function Wait-ServiceListening($Service, [int]$TimeoutSeconds = 25) {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        Reset-ProcessCache
        $running = @(Get-ServiceRuntime $Service | Where-Object { $_.IsExpected })
        if ($running.Count -gt 0) {
            return $running
        }
        Start-Sleep -Milliseconds 500
    }
    return @()
}

function Get-ServiceRuntime($Service) {
    $listeners = Get-PortListeners $Service.Port
    $expectedPath = Normalize-Text $Service.Path
    $items = @()
    foreach ($listener in $listeners) {
        $pidValue = [int]$listener.OwningProcess
        $command = Get-ProcessTreeCommand $pidValue
        $hay = Normalize-Text $command
        $isExpected = $hay.Contains($expectedPath)
        $displayCommand = Get-PortOwnerHint $pidValue $Service.Port
        if ([string]::IsNullOrWhiteSpace($displayCommand)) {
            $displayCommand = $command
        } elseif ($command -and -not (Normalize-Text $displayCommand).Contains($expectedPath)) {
            $displayCommand = "$displayCommand ; tree=$command"
        }
        $items += [pscustomobject]@{
            PID = $pidValue
            Command = $displayCommand
            IsExpected = $isExpected
        }
    }
    return $items
}

function Write-StatusTable {
    $rows = foreach ($svc in $services) {
        $runtimes = Get-ServiceRuntime $svc
        if ($runtimes.Count -eq 0) {
            [pscustomobject]@{ Service = $svc.Name; Port = $svc.Port; State = "STOPPED"; PID = ""; Command = "" }
            continue
        }
        foreach ($runtime in $runtimes) {
            $state = if ($runtime.IsExpected) { "RUNNING" } else { "BLOCKED" }
            [pscustomobject]@{ Service = $svc.Name; Port = $svc.Port; State = $state; PID = $runtime.PID; Command = $runtime.Command }
        }
    }
    $rows | Format-Table -AutoSize
}

function Get-ReversedServices {
    $copy = @($services)
    [array]::Reverse($copy)
    return $copy
}

function Stop-ServiceProcess($Service) {
    Reset-ProcessCache
    $runtimes = Get-ServiceRuntime $Service
    $expected = @($runtimes | Where-Object { $_.IsExpected })
    foreach ($runtime in $expected) {
        Write-Host "[STOP] $($Service.Name) pid=$($runtime.PID)"
        Stop-Process -Id $runtime.PID -Force -ErrorAction SilentlyContinue
    }
    $goParents = @(Get-ServiceGoRunProcesses $Service)
    if ($expected.Count -eq 0 -and $goParents.Count -eq 0) {
        Write-Host "[STOP] $($Service.Name) is not running as this repo service."
        return
    }
    foreach ($parent in $goParents) {
        Write-Host "[STOP] $($Service.Name) go-run pid=$($parent.ProcessId)"
        Stop-Process -Id $parent.ProcessId -Force -ErrorAction SilentlyContinue
    }
    Reset-ProcessCache
}

function Start-ServiceProcess($Service) {
    Write-Host "[$($Service.Step)/18] Starting $($Service.Name) (port $($Service.Port))..."
    Reset-ProcessCache
    $runtimes = Get-ServiceRuntime $Service
    if ($runtimes.Count -gt 0) {
        $expected = @($runtimes | Where-Object { $_.IsExpected })
        if ($expected.Count -gt 0) {
            $pids = ($expected | ForEach-Object { $_.PID }) -join ","
            Write-Host "[SKIP] $($Service.Name) is already running from this repo on 127.0.0.1:$($Service.Port) pid=$pids"
            return $true
        }
        Write-Host "[BLOCKED] $($Service.Name) cannot start because 127.0.0.1:$($Service.Port) is occupied by another process:"
        foreach ($runtime in $runtimes) {
            Write-Host "  pid=$($runtime.PID) $($runtime.Command)"
        }
        return $false
    }

    $goParents = @(Get-ServiceGoRunProcesses $Service)
    if ($goParents.Count -gt 0) {
        $goPids = ($goParents | ForEach-Object { $_.ProcessId }) -join ","
        $after = @(Wait-ServiceListening $Service 8)
        if ($after.Count -gt 0) {
            $pids = ($after | ForEach-Object { $_.PID }) -join ","
            Write-Host "[SKIP] $($Service.Name) already has go-run pid=$goPids and is listening on 127.0.0.1:$($Service.Port) pid=$pids"
            return $true
        }
        Write-Host "[WARN] $($Service.Name) has go-run pid=$goPids but is not listening on 127.0.0.1:$($Service.Port). Check its terminal logs."
        return $true
    }

    $cmd = "cd /d `"$repoRoot`" && go run $($Service.Path)"
    Start-Process -FilePath "cmd.exe" -ArgumentList "/k", $cmd -WindowStyle Normal | Out-Null
    Reset-ProcessCache
    $after = @(Wait-ServiceListening $Service 25)
    if ($after.Count -eq 0) {
        Write-Host "[WARN] $($Service.Name) window was opened, but the service did not listen on 127.0.0.1:$($Service.Port) within 25s. Check its terminal logs."
    } else {
        $pids = ($after | ForEach-Object { $_.PID }) -join ","
        Write-Host "[OK] $($Service.Name) is listening on 127.0.0.1:$($Service.Port) pid=$pids"
    }
    return $true
}

function Ensure-DockerContainers {
    if ($NoDocker) { return }
    Write-Host "[1/18] Checking Docker containers..."
    $dockerOK = $true
    try {
        $names = docker ps --format "{{.Names}}" 2>$null
        foreach ($name in @("MySQL", "Redis", "etcd")) {
            if ($names -notcontains $name) { $dockerOK = $false }
        }
    } catch {
        $dockerOK = $false
    }
    if (-not $dockerOK) {
        Write-Host "[WARN] Docker containers MySQL/Redis/etcd are not all running. Run docker-compose up -d if services need them."
    }
}

switch ($Action) {
    "status" {
        Write-Host "ClaranAIM service status"
        Write-StatusTable
        exit 0
    }
    "stop" {
        Write-Host "Stopping ClaranAIM services started from this repo..."
        foreach ($svc in (Get-ReversedServices)) {
            Stop-ServiceProcess $svc
        }
        exit 0
    }
    "restart" {
        Write-Host "Restarting ClaranAIM services..."
        foreach ($svc in (Get-ReversedServices)) {
            Stop-ServiceProcess $svc
        }
        Start-Sleep -Seconds 2
        if ($NoDocker) {
            & $PSCommandPath -Action start -NoDocker
        } else {
            & $PSCommandPath -Action start
        }
        exit $LASTEXITCODE
    }
    "start" {
        Write-Host "========================================"
        Write-Host "  ClaranAIM Services Starting"
        Write-Host "========================================"
        Write-Host ""
        Ensure-DockerContainers
        Write-Host ""
        $blocked = New-Object System.Collections.Generic.List[string]
        foreach ($svc in $services) {
            if (-not (Start-ServiceProcess $svc)) {
                $blocked.Add($svc.Name)
            }
        }
        Start-Sleep -Seconds 3
        $hasFatalLogs = Report-NewFatalLogs
        $hasRuntimeIssues = Report-ServiceRuntimeIssues
        Write-Host ""
        Write-Host "========================================"
        if ($blocked.Count -eq 0 -and -not $hasFatalLogs -and -not $hasRuntimeIssues) {
            Write-Host "  All services are running or were started."
        } else {
            if ($blocked.Count -gt 0) {
                Write-Host "  Some services were blocked: $($blocked -join ', ')"
            }
            if ($hasFatalLogs) {
                Write-Host "  Startup wrote fatal/error logs. Inspect logs\ERR before using the frontend."
            }
            if ($hasRuntimeIssues) {
                Write-Host "  Some services failed final runtime verification."
            }
        }
        Write-Host "========================================"
        Write-Host ""
        Write-Host "  API Gateway:          http://127.0.0.1:18080"
        Write-Host "  WebSocket Gateway:    ws://127.0.0.1:8081"
        Write-Host "  MinIO Console:        http://127.0.0.1:9001"
        Write-Host "  Frontend:             Open dist/index.html in browser"
        Write-Host ""
        Write-Host "Use scripts\status.bat to inspect, scripts\stop.bat to stop residual go-run processes, scripts\restart.bat to restart."
        exit $(if ($blocked.Count -eq 0 -and -not $hasFatalLogs -and -not $hasRuntimeIssues) { 0 } else { 1 })
    }
}
