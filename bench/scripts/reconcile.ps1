param(
    [Parameter(Mandatory = $true)]
    [string]$RunId,
    [int]$ExpectSent = 0
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-Location $script:RepoRoot

function Invoke-Sql([string]$Sql) {
    $tmp = Join-Path $env:TEMP ("bench-reconcile-{0}.sql" -f ([guid]::NewGuid().ToString("N")))
    Set-Content -LiteralPath $tmp -Value $Sql -Encoding utf8
    docker cp $tmp claran-mysql:/tmp/bench_reconcile.sql | Out-Null
    $output = docker exec claran-mysql sh -c "mysql -u$($script:BenchUser) -p$($script:BenchPassword) -D ClaranAIM_bench -N -B -e 'source /tmp/bench_reconcile.sql'" 2>&1
    Remove-Item -LiteralPath $tmp -ErrorAction SilentlyContinue
    return @($output | Where-Object { $_ -and $_ -notmatch '^mysql:' })
}

$like = "bench-$RunId-%"
$sql = @"
SELECT COUNT(*) FROM messages WHERE content LIKE '$like';
SELECT COUNT(*) FROM message_user_states s JOIN messages m ON m.id = s.message_id WHERE m.content LIKE '$like';
SELECT COUNT(*) FROM event_outbox o JOIN messages m ON m.id = o.aggregate_id WHERE m.content LIKE '$like';
SELECT COUNT(*) FROM event_outbox o JOIN messages m ON m.id = o.aggregate_id WHERE m.content LIKE '$like' AND o.status = 'published';
SELECT COUNT(*) FROM event_outbox o JOIN messages m ON m.id = o.aggregate_id WHERE m.content LIKE '$like' AND o.status IN ('pending','retrying');
SELECT IFNULL(MAX(TIMESTAMPDIFF(MICROSECOND, o.created_at, o.published_at))/1000, 0)
  FROM event_outbox o JOIN messages m ON m.id = o.aggregate_id
  WHERE m.content LIKE '$like' AND o.published_at IS NOT NULL;
"@
$rows = Invoke-Sql $sql
$result = [ordered]@{
    run_id            = $RunId
    expect_sent       = $ExpectSent
    messages          = [int64]$rows[0]
    user_states       = [int64]$rows[1]
    outbox_total      = [int64]$rows[2]
    outbox_published  = [int64]$rows[3]
    outbox_pending    = [int64]$rows[4]
    outbox_max_pub_ms = [double]$rows[5]
}

$kafka = docker exec claran-kafka kafka-run-class.sh kafka.tools.GetOffsetShell --broker-list localhost:9092 --topic claran.message.events 2>&1 |
    Select-String 'claran.message.events' | ForEach-Object { ($_.Line -split ':')[-1] }
$result.kafka_offset = if ($kafka) { [int64]$kafka } else { 0 }
$result.missing_messages = if ($ExpectSent -gt 0) { [Math]::Max(0, $ExpectSent - $result.messages) } else { 0 }
$result.duplicate_outbox = [Math]::Max(0, $result.outbox_total - $result.messages)

$dir = "bench/results/$RunId"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$json = $result | ConvertTo-Json -Depth 4
Set-Content -LiteralPath (Join-Path $dir "reconcile.json") -Value $json -Encoding utf8
$json
