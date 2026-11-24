param(
    [switch]$SkipGoTest,
    [switch]$SkipFrontendCheck
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

Write-Host "[1/3] 检查生成占位文件..."
if (Test-Path "handler.go") {
    throw "根目录存在 Kitex 占位 handler.go，请删除"
}
if (Test-Path "main.go") {
    throw "根目录存在 Kitex 占位 main.go，请删除"
}

if (-not $SkipFrontendCheck) {
    Write-Host "[2/3] 检查前端 JavaScript 语法..."
    node --check "dist/js/api.js"
    node --check "dist/js/app.js"
} else {
    Write-Host "[2/3] 跳过前端 JavaScript 检查"
}

if (-not $SkipGoTest) {
    Write-Host "[3/3] 运行 Go 全量测试..."
    go test ./...
} else {
    Write-Host "[3/3] 跳过 Go 测试"
}

Write-Host "E2E smoke checks passed."
