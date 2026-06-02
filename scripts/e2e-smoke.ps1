param(
    [switch]$SkipGoTest,
    [switch]$SkipFrontendCheck
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

Write-Host "[1/4] 检查关键文档状态..."
$plan = Get-Content "docs/plan.md" -Raw
if ($plan -match "实体归一化、Leiden 社区划分、社区摘要持久化.*未完成") {
    throw "docs/plan.md 仍把已落地的图谱 MVP 写成完全未完成"
}
if (($plan -notmatch "客户端.*缓存") -or ($plan -notmatch "云端漫游")) {
    throw "docs/plan.md 缺少客户端缓存/漫游策略条目"
}

Write-Host "[2/4] 检查生成占位文件..."
if (Test-Path "handler.go") {
    throw "根目录存在 Kitex 占位 handler.go，请删除"
}
if (Test-Path "main.go") {
    throw "根目录存在 Kitex 占位 main.go，请删除"
}

if (-not $SkipFrontendCheck) {
    Write-Host "[3/4] 检查前端 JavaScript 语法..."
    node --check "dist/js/api.js"
    node --check "dist/js/app.js"
} else {
    Write-Host "[3/4] 跳过前端 JavaScript 检查"
}

if (-not $SkipGoTest) {
    Write-Host "[4/4] 运行 Go 全量测试..."
    go test ./...
} else {
    Write-Host "[4/4] 跳过 Go 测试"
}

Write-Host "E2E smoke checks passed."
