# ClaranAIM 压测工具链

> 基线：`main @ e76a742`
> 规范：`bench/GOAL.md`
> 状态：`bench/status.md`

## 1. 目录

```text
bench/
  bin/                    # 本地编译产物（不提交）
  logs/                   # 服务日志（不提交）
  mockllm/                # OpenAI 兼容 mock 服务
  seed/                   # 种子数据与 token 生成
  loadgen/                # Go 压测器（HTTP + WebSocket）
  scripts/                # 启停、故障注入、smoke、对账
  results/<run-id>/       # 原始结果
  report/                 # 阶段报告
```

## 2. 前置条件

- Docker Desktop，至少 6GB 可用内存；本次环境为 20C / 7.6GiB。
- Go 1.25+（实际使用 `go1.27.0`）。
- 基础设施：`claran-mysql`、`claran-redis`、`claran-kafka`、`claran-etcd`、`claran-otel-collector`、`claran-prometheus`、`claran-grafana`、`claran-jaeger`。
- MySQL 使用 `ClaranAIM_bench` 库；开发库不受影响。
- 不需要 k6 / vegeta；压测器是仓库内 Go 程序。

## 3. 关键配置

| 配置 | 值 | 说明 |
|---|---|---|
| MySQL | `127.0.0.1:3306`，库 `ClaranAIM_bench` | 账号从 `.env` 读取 |
| MySQL X Plugin | `33061` | 修复 `33060` 端口冲突 |
| Redis | `127.0.0.1:6379` | db 0 |
| Kafka | `127.0.0.1:9092` | `claran.message.events` 单分区 |
| API | `http://127.0.0.1:18080` | api-gateway |
| WS | `ws://127.0.0.1:8081/ws` | token 通过 query 传递 |
| metrics | `19002/19003/19004/19080/19081/19101` | Prometheus 抓 `host.docker.internal` |
| 限流 | `RATE_LIMIT_ENABLED=false` | 压测期间关闭，报告必须注明 |
| LLM | `http://127.0.0.1:18090/v1` | mock，固定 150ms |
| embedding | `http://127.0.0.1:18090/v1/embeddings` | 256 维确定性向量 |

## 4. 常用命令

```powershell
# 启动 mock LLM
.\bench\scripts\start-mock.ps1 -LatencyMs 150

# 启动最小服务集（user/group/msg-core/msg-history/ws/api）
.\bench\scripts\start-services.ps1

# 启动 Agent 或 RAG 依赖
.\bench\scripts\start-extra.ps1 -Agent
.\bench\scripts\start-extra.ps1 -RAG

# 全量种子（幂等）
.\bench\bin\seed.exe -dsn $env:MYSQL_DSN -run-id seed-20261124 `
  -users 1000 -groups 100 -messages 100000 -reset

# 生成压测 token（避免登录限流）
.\bench\bin\tokens.exe -manifest bench/results/seed/manifest.json `
  -out bench/results/seed/tokens.txt -secret $env:JWT_SECRET_KEY

# 冒烟
.\bench\scripts\smoke.ps1 -RunId AIM-P0-smoke-final

# 压测（示例：C3 100 并发）
.\bench\bin\loadgen.exe -scenario c3 -concurrency 100 -duration 5m -warmup 1m `
  -tokens bench/results/seed/tokens.txt -run-id AIM-P1-C3-100vu

# 自动按内存降档跑 C2/C3（本机推荐）
.\bench\scripts\run-phase1.ps1 -Phase c3 -Levels 10,20,50 `
  -Seconds 60 -WarmupSeconds 10 -Repeats 3 -Prefix AIM-P1-C3

# 故障注入：Kafka / MySQL / kill msg-core
.\bench\scripts\c10.ps1 -Scenario kafka
.\bench\scripts\c10.ps1 -Scenario mysql
.\bench\scripts\c10.ps1 -Scenario kill

# 故障注入
.\bench\scripts\fault.ps1 -Action kafka-stop
.\bench\scripts\fault.ps1 -Action kafka-start

# 停止服务
.\bench\scripts\stop-services.ps1
```

## 5. 结果结构

原始结果默认不提交；运行脚本后生成在本地 `bench/results/`，仓库只保留报告与脚本。

`bench/results/<run-id>/summary.json`：

| 字段 | 含义 |
|---|---|
| `sent/ok/failed` | 请求数、成功数、失败数 |
| `rps` | 测量窗口内请求速率 |
| `p50/p90/p95/p99/max` | HTTP 端到端分位（毫秒） |
| `statuses` | HTTP 状态码分布 |
| `errors` | 错误分类 |
| `ws_recv/ws_dup` | WebSocket 收到/重复消息数 |

## 6. 已知差异

- `websocket-gateway` 使用内存幂等表，不写 `processed_events`；WS 对账以
  `claran_business_events_total{domain="websocket",operation="delivery",status="success"}` 和客户端 `ws_recv` 为准。
- `msg-core-service` 与 `agent-manager-service` 使用 MySQL `processed_events`。
- 种子消息 ID 小于当前雪花 ID，避免新消息排到历史之后。
- 群消息不写 `message_user_states`，避免 10 万消息放大为百万行；历史查询使用 LEFT JOIN，不影响返回。
- Kafka `claran.message.events` 当前只有 1 个分区，这本身是 C3/C4 的预期瓶颈之一。

## 7. 验收

Phase 0 通过标准见 `bench/GOAL.md` 第 5 节；最新 smoke 证据在
`bench/results/AIM-P0-smoke-final/smoke.json`。

