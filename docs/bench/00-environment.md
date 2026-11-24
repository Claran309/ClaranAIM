# ClaranAIM 压测环境记录

> 记录时间：2026-11-24
> 基线：`main @ e76a742`

## 1. 主机与容器

| 项目 | 实际值 |
|---|---|
| 主机 CPU | 20 逻辑核 |
| 主机内存 | 15.7 GB，压测前空闲约 1.25 GB |
| Docker VM | 20C / 7.6 GiB |
| 磁盘可用 | 约 33 GB |
| 运行方式 | 基础设施 Docker + Go 服务宿主机 |

## 2. 组件版本

| 组件 | 版本 / 配置 |
|---|---|
| MySQL | 8.0.32，端口 3306（容器内 33060），X Plugin 33061 |
| Kafka | 3.3.2，KRaft 单节点，`claran.message.events` 1 分区 |
| Redis | 8.4.0 |
| Etcd | 单节点 |
| Go | 1.27.0 |
| OTel Collector | 4318 HTTP，Prometheus exporter 8889 |
| Prometheus | 8084 |
| Grafana | 8086 |
| Jaeger | 8085 |

## 3. 服务与端口

| 服务 | 地址 | metrics |
|---|---|---|
| api-gateway | `127.0.0.1:18080` | 19080 |
| websocket-gateway | `127.0.0.1:8081` | 19081 |
| user-service | `127.0.0.1:9101` | 19101 |
| group-service | `127.0.0.1:9002` | 19002 |
| msg-core-service | `127.0.0.1:9003` | 19003 |
| msg-history-service | `127.0.0.1:9004` | 19004 |
| memory-service | `127.0.0.1:9008` | 19008 |
| agent-runtime-service | `127.0.0.1:9007` | 19007 |
| agent-manager-service | `127.0.0.1:9006` | 19006 |
| rag-service | `127.0.0.1:9112` | 19112 |
| mockllm | `127.0.0.1:18090` | `/stats` |

## 4. 修复与变更

| 问题 | 处理 | 影响 |
|---|---|---|
| MySQL 因 `--port=33060` 与 X Plugin 默认端口冲突，重启循环 | 增加 `--mysqlx-port=33061` | 容器恢复 healthy |
| `.env` 指向开发库 | 压测使用 `ClaranAIM_bench` | 开发数据不受影响 |
| 无 pprof / DB 池 / runtime 指标 | `pkg/observability` 增加 pprof、`claran_db_pool`、`claran_go_runtime` | 可观测性补齐 |
| 网关登录限流 120/分钟 | 压测期 `RATE_LIMIT_ENABLED=false`，并使用预生成 token | C3 不被限流掩盖 |
| 无 k6 | 使用仓库内 Go `loadgen` | 可复现、无额外依赖 |
| Jaeger 内存无上限，压测中涨到 3.5GB | `MEMORY_MAX_TRACES=500`，每轮前重启清空 | 消除压测环境“假瓶颈” |
| Kitex/DB 连接 churn 耗尽 Windows 临时端口 | DB 池固定 32、压测器 keep-alive、每轮检查 TIME_WAIT | 避免 WSAENOBUFS 伪错误 |

## 5. 种子数据

| 项目 | 数量 |
|---|---|
| 用户 | 1,000 |
| 群 | 100（20-200 人） |
| 群成员 | 11,377 |
| 会话 | 1,100（100 群 + 1,000 私聊） |
| 会话参与者 | 13,377 |
| 历史消息 | 100,000 |
| 用户消息状态 | 180,000 |

种子命令耗时约 9.8 秒；重复执行不增加行数。

## 6. Smoke 证据

`bench/results/AIM-P0-smoke-final/smoke.json`：

- 登录、发送、历史、幂等：PASS
- 落库 1 条、用户状态 2 条、Outbox `published`：PASS
- WebSocket 收到 `new_message`：PASS
- mock chat / embedding 256 维：PASS
- Prometheus 4 个服务 UP；Jaeger 可见 9 个服务：PASS

## 7. 限制

- Docker VM 内存 7.6 GiB，Neo4j / Milvus 未启用；RAG 使用本地向量索引。
- 群消息不写 `message_user_states`，这是种子口径，不是生产行为。
- WS 消费端使用内存幂等表，不写 `processed_events`。
- 当前为单实例、单分区，容量数字只代表该配置。
- 本机有效档位为 10/20VU；50VU 仅 C3 稳定完成，C2 50VU 未纳入结论。

