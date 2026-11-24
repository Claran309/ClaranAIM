param(
    [switch]$Agent,
    [switch]$RAG,
    [switch]$Milvus,
    [string]$MockBase = "http://127.0.0.1:18090/v1"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "env.ps1")
Set-BenchEnv
Set-Location $script:RepoRoot
New-Item -ItemType Directory -Force -Path "bench/bin", "bench/logs" | Out-Null

# Agent / RAG 都走 mock LLM 与 mock embedding，避免真实费用。
$env:LLM_DEFAULT_BASE_URL = $MockBase
$env:LLM_DEFAULT_API_KEY = "mock-key"
$env:LLM_DEFAULT_MODEL = "mock-gpt"
# embedding 使用独立的低延迟 mock（默认 18091），避免 150ms 人为延迟主导 RAG 入库。
$embedBase = if ($env:MOCK_EMBEDDING_BASE) { $env:MOCK_EMBEDDING_BASE } else { "http://127.0.0.1:18091/v1" }
$env:RAG_EMBEDDING_PROVIDER = "glm"
$env:RAG_EMBEDDING_URL = "$embedBase/embeddings"
$env:RAG_EMBEDDING_API_KEY = "mock-key"
$env:RAG_EMBEDDING_MODEL = "mock-embedding"
$env:RAG_EMBEDDING_DIMENSION = "256"
$env:MEMORY_RAG_EMBEDDING_PROVIDER = "glm"
$env:MEMORY_RAG_EMBEDDING_URL = "$embedBase/embeddings"
$env:MEMORY_RAG_EMBEDDING_API_KEY = "mock-key"
$env:MEMORY_RAG_EMBEDDING_MODEL = "mock-embedding"
$env:MEMORY_RAG_EMBEDDING_DIMENSION = "256"
$env:MEMORY_RAG_MILVUS_ENABLED = "false"
if ($Milvus) {
    $env:MILVUS_ENABLED = "true"
    $env:MILVUS_ADDRESS = "127.0.0.1:19530"
    $env:MILVUS_COLLECTION = "claran_rag_chunks_milvus"
} else {
    $env:MILVUS_ENABLED = "false"
}
$env:NEO4J_ENABLED = "false"
$env:RAG_ROUTER_PROVIDER = "rule"

$targets = @()
if ($Agent) {
    $targets += @("memory-service", "agent-runtime-service", "agent-manager-service")
}
if ($RAG) {
    $targets += @("rag-service")
}

foreach ($name in $targets) {
    $exe = "bench/bin/$name.exe"
    if (-not (Test-Path $exe)) {
        Write-Host "building $name..."
        go build -o $exe "./cmd/$name"
        if ($LASTEXITCODE -ne 0) { throw "build failed: $name" }
    }
}

foreach ($name in $targets) {
    if (Get-Process -Name $name -ErrorAction SilentlyContinue) {
        Write-Host "$name already running"
        continue
    }
    Write-Host "starting $name"
    Start-Process -FilePath "bench/bin/$name.exe" -WorkingDirectory $script:RepoRoot -WindowStyle Hidden `
        -RedirectStandardOutput "bench/logs/$name.out.log" `
        -RedirectStandardError "bench/logs/$name.err.log" | Out-Null
    Start-Sleep -Seconds 3
}

Start-Sleep -Seconds 3
foreach ($name in $targets) {
    $p = Get-Process -Name $name -ErrorAction SilentlyContinue
    Write-Host ("{0}: {1}" -f $name, $(if ($p) { "running pid=$($p.Id)" } else { "NOT RUNNING" }))
}
