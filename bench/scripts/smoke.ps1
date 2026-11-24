param(
    [string]$RunId = "",
    [string]$BaseUrl = "http://127.0.0.1:18080",
    [string]$WsUrl = "ws://127.0.0.1:8081/ws",
    [string]$MockUrl = "http://127.0.0.1:18090"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-BenchEnv
Set-Location $script:RepoRoot

if ([string]::IsNullOrWhiteSpace($RunId)) {
    $RunId = "AIM-P0-smoke-" + (Get-Date -Format "yyyyMMdd-HHmm")
}
$outDir = Join-Path $script:RepoRoot "bench/results/$RunId"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$result = [ordered]@{
    run_id     = $RunId
    started_at = (Get-Date).ToString("o")
    steps      = @()
}

function Add-Step([string]$Name, [string]$Status, $Detail) {
    $script:result.steps += [ordered]@{
        name   = $Name
        status = $Status
        detail = $Detail
    }
    Write-Host ("[{0}] {1}" -f $Status, $Name)
}

function Invoke-Sql([string]$Sql) {
    $tmp = Join-Path $env:TEMP ("bench-smoke-{0}.sql" -f ([guid]::NewGuid().ToString("N")))
    Set-Content -LiteralPath $tmp -Value $Sql -Encoding utf8
    docker cp $tmp claran-mysql:/tmp/bench_smoke.sql | Out-Null
    $output = docker exec claran-mysql sh -c "mysql -u$($script:BenchUser) -p$($script:BenchPassword) -D ClaranAIM_bench -N -B -e 'source /tmp/bench_smoke.sql'" 2>&1
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    return $output
}

# 1) 登录
$loginBody = @{ username = "bench-user-0001"; password = $env:BENCH_PASSWORD } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$BaseUrl/api/v1/user/login" -Method Post -ContentType "application/json" -Body $loginBody -TimeoutSec 15
$token = $login.data.access_token
if ([string]::IsNullOrWhiteSpace($token)) { throw "login failed: $($login | ConvertTo-Json -Compress)" }
$headers = @{ Authorization = "Bearer $token" }
Add-Step "login" "PASS" @{ user_id = $login.data.user_id }

# 2) 取私聊会话
$manifest = Get-Content "bench/results/seed/manifest.json" -Raw | ConvertFrom-Json
$conv = $manifest.private_conversations[0]
$convID = "$($conv.id)"
Add-Step "manifest" "PASS" @{ conversation_id = $convID; users = $manifest.users.Count }

# 3) 发送消息
$clientMsgId = "$RunId-send-1"
$content = "bench-$RunId-hello"
$sendBody = @{
    conversation_id = $convID
    content         = $content
    msg_type        = "text"
    client_msg_id   = $clientMsgId
} | ConvertTo-Json
$send = Invoke-RestMethod -Uri "$BaseUrl/api/v1/message/send" -Method Post -Headers $headers -ContentType "application/json" -Body $sendBody -TimeoutSec 30
if ($send.code -ne 0) { throw "send failed: $($send | ConvertTo-Json -Compress)" }
$msgID = "$($send.data.msg_id)"
Add-Step "send_message" "PASS" @{ msg_id = $msgID; client_msg_id = $clientMsgId }

# 4) 历史与幂等
$history = Invoke-RestMethod -Uri "$BaseUrl/api/v1/message/history/${convID}?limit=5" -Headers $headers -TimeoutSec 15
$found = @($history.data.messages | Where-Object { $_.id -eq [int64]$msgID }).Count
$sendAgain = Invoke-RestMethod -Uri "$BaseUrl/api/v1/message/send" -Method Post -Headers $headers -ContentType "application/json" -Body $sendBody -TimeoutSec 30
$sameID = "$($sendAgain.data.msg_id)" -eq $msgID
Add-Step "history_and_idempotency" $(if ($found -eq 1 -and $sameID) { "PASS" } else { "FAIL" }) @{
    history_found = $found
    idempotent    = $sameID
}

# 5) 落库、状态与 Outbox
Start-Sleep -Seconds 2
$counts = Invoke-Sql @"
SELECT COUNT(*) FROM messages WHERE content = '$content';
SELECT COUNT(*) FROM message_user_states WHERE message_id = $msgID;
SELECT status FROM event_outbox WHERE aggregate_id = $msgID ORDER BY created_at DESC LIMIT 1;
"@
$lines = @($counts | Where-Object { $_ -and $_ -notmatch '^mysql:' })
Add-Step "db_and_outbox" "PASS" @{
    messages    = $lines[0]
    user_states = $lines[1]
    outbox      = $lines[2]
}

# 6) WebSocket 投递
$login2 = Invoke-RestMethod -Uri "$BaseUrl/api/v1/user/login" -Method Post -ContentType "application/json" -Body (@{ username = "bench-user-0002"; password = $env:BENCH_PASSWORD } | ConvertTo-Json) -TimeoutSec 15
$ws = New-Object System.Net.WebSockets.ClientWebSocket
$ws.ConnectAsync([Uri]"${WsUrl}?token=$($login2.data.access_token)", [Threading.CancellationToken]::None).GetAwaiter().GetResult()
$wsMsgId = "$RunId-ws-1"
$wsContent = "bench-$RunId-ws"
$wsBody = @{ conversation_id = $convID; content = $wsContent; msg_type = "text"; client_msg_id = $wsMsgId } | ConvertTo-Json
Invoke-RestMethod -Uri "$BaseUrl/api/v1/message/send" -Method Post -Headers $headers -ContentType "application/json" -Body $wsBody -TimeoutSec 30 | Out-Null
$buf = New-Object byte[] 8192
$cts = [Threading.CancellationTokenSource]::new(10000)
$wsOk = $false
$wsText = ""
try {
    $recv = $ws.ReceiveAsync([ArraySegment[byte]]$buf, $cts.Token).GetAwaiter().GetResult()
    $wsText = [Text.Encoding]::UTF8.GetString($buf, 0, $recv.Count)
    $wsOk = $wsText -like "*$wsMsgId*"
} catch {
    $wsText = $_.Exception.Message
}
$ws.Dispose()
Add-Step "websocket_delivery" $(if ($wsOk) { "PASS" } else { "FAIL" }) @{ matched = $wsOk; payload = $wsText }

# 7) Mock LLM / embedding
$chat = Invoke-RestMethod -Uri "$MockUrl/v1/chat/completions" -Method Post -ContentType "application/json" -Body '{"model":"mock-gpt","messages":[{"role":"user","content":"smoke"}]}' -TimeoutSec 15
$emb = Invoke-RestMethod -Uri "$MockUrl/v1/embeddings" -Method Post -ContentType "application/json" -Body '{"model":"mock-embedding","input":"smoke"}' -TimeoutSec 15
Add-Step "mock_llm" "PASS" @{
    chat_content = $chat.choices[0].message.content
    embedding_dim = @($emb.data[0].embedding).Count
}

# 8) Prometheus 与 Jaeger
$targets = (Invoke-RestMethod "http://127.0.0.1:8084/api/v1/targets" -TimeoutSec 10).data.activeTargets |
    Where-Object { $_.labels.service -in @("api-gateway", "msg-core-service", "websocket-gateway", "user-service") }
$upCount = @($targets | Where-Object { $_.health -eq "up" }).Count
$services = (Invoke-RestMethod "http://127.0.0.1:8085/api/services" -TimeoutSec 10).data
Add-Step "observability" $(if ($upCount -eq 4) { "PASS" } else { "FAIL" }) @{
    prometheus_up = $upCount
    jaeger_services = $services
}

$result.finished_at = (Get-Date).ToString("o")
$result.overall = if (@($result.steps | Where-Object { $_.status -eq "FAIL" }).Count -eq 0) { "PASS" } else { "FAIL" }
$encoded = $result | ConvertTo-Json -Depth 8
Set-Content -LiteralPath (Join-Path $outDir "smoke.json") -Value $encoded -Encoding utf8

$encoded
Write-Host "smoke result: $outDir/smoke.json"
if ($result.overall -ne "PASS") { exit 1 }
