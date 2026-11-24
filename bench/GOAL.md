# ClaranAIM 压测与运维总 Goal（状态机）

> 基线：`main @ e76a742`
> 范围：仅 ClaranAIM
> 配套文件：`bench/status.md`（实时状态账本）、`bench/MASTER_GOAL.md`（启动提示词）
> 最后更新：2026-11-24

## 1. 总目标

在固定的 ClaranAIM commit 上，用本地可复现环境完成：

1. 消息写入、Outbox、Kafka、WebSocket 推送的核心容量基线；
2. MySQL、Kafka、Redis、服务进程的故障注入与恢复验证；
3. Agent / RAG 在 mock LLM、mock embedding 下的容量基线；
4. 基于基线证据的定向优化与同口径复测；
5. 可回溯到原始数据的容量报告、故障报告和简历数字。

最终产物必须能回答：

- 单机在指定配置下能稳定承载多少 msg/s、多少 WS 连接？
- 瓶颈在 MySQL、Outbox worker、Kafka、WebSocket 还是应用层？
- Kafka / MySQL / Redis 宕机或服务被 kill 后，消息会不会丢、多久恢复？
- Agent 并发上来后，是模型、工具、会话存储还是数据库先成为瓶颈？
- RAG 入库和检索在 mock embedding 下的吞吐、P95/P99 和失败率是多少？

## 2. 非目标与硬边界

- 只做 ClaranAIM；不要修改、运行或复用 `D:\CodeStudy\growing` 下的另一个项目。
- 不碰生产环境；不调用真实付费 LLM 做高并发；不触发真实送礼、私信、动态、关注等副作用。
- 不追求“17 个服务全部压一遍”；只覆盖本文件定义的核心链路。
- 不在 Phase 0-2 修改业务语义；Phase 3 只允许针对基线证据做定向优化。
- 不为了数字修改延迟口径、减少校验、放宽幂等或隐藏失败。
- 不提交 `.env`、API key、cookie、用户隐私数据。
- 不执行 `docker compose down -v`；不删除 Docker volume 或已有数据库。
- 不 push 远程，除非用户明确要求。

## 3. 状态机

### 3.1 状态定义

| 状态 | 含义 |
|---|---|
| `NOT_STARTED` | 尚未开始 |
| `P0_IN_PROGRESS` | 环境与工具链准备中 |
| `P0_PASS` | Phase 0 通过 |
| `P0_FAIL` | Phase 0 失败，可重试 |
| `P1_IN_PROGRESS` | 核心基线测试中 |
| `P1_PASS` | Phase 1 通过 |
| `P1_FAIL` | Phase 1 失败，可重试 |
| `P2_IN_PROGRESS` | Agent / RAG 测试中 |
| `P2_PASS` | Phase 2 通过 |
| `P2_FAIL` | Phase 2 失败，可重试 |
| `P3_IN_PROGRESS` | 定向优化与复测中 |
| `P3_PASS` | Phase 3 通过 |
| `P3_N/A` | 基线未发现需要优化的瓶颈，跳过 Phase 3 |
| `P3_FAIL` | Phase 3 未取得可验证收益，已回滚或记录 no-go |
| `P4_IN_PROGRESS` | 证据归档与简历数字固化中 |
| `P4_PASS` | Phase 4 通过 |
| `P4_FAIL` | Phase 4 失败 |
| `COMPLETE` | 总 goal 完成 |
| `BLOCKED` | 无法继续，必须停止并汇报 |

### 3.2 允许的状态转移

| From | To | 条件 |
|---|---|---|
| `NOT_STARTED` | `P0_IN_PROGRESS` | 读取本文件并初始化 `bench/status.md` |
| `P0_IN_PROGRESS` | `P0_PASS` | Phase 0 退出条件全部满足 |
| `P0_IN_PROGRESS` | `P0_FAIL` | smoke 失败或环境未就绪 |
| `P0_FAIL` | `P0_IN_PROGRESS` | 已修复问题并重新 smoke；最多重试 2 次 |
| `P0_FAIL` | `BLOCKED` | 连续 2 次失败或环境无法修复 |
| `P0_PASS` | `P1_IN_PROGRESS` | 自动进入 Phase 1 |
| `P1_IN_PROGRESS` | `P1_PASS` | Phase 1 退出条件全部满足 |
| `P1_IN_PROGRESS` | `P1_FAIL` | 基线数据不完整、不可复现或对账失败 |
| `P1_FAIL` | `P1_IN_PROGRESS` | 修复测试问题后重跑；最多重试 2 次 |
| `P1_FAIL` | `BLOCKED` | 连续 2 次失败或资源无法支撑核心基线 |
| `P1_PASS` | `P2_IN_PROGRESS` | 自动进入 Phase 2 |
| `P2_IN_PROGRESS` | `P2_PASS` | Phase 2 退出条件全部满足 |
| `P2_IN_PROGRESS` | `P2_FAIL` | Agent / RAG 数据不完整或不可复现 |
| `P2_FAIL` | `P2_IN_PROGRESS` | 修复 mock、数据或配置后重跑；最多重试 2 次 |
| `P2_FAIL` | `BLOCKED` | 连续 2 次失败或资源无法支撑 Agent / RAG |
| `P2_PASS` | `P3_IN_PROGRESS` | Phase 1 / 2 报告明确指出 top 瓶颈 |
| `P2_PASS` | `P3_N/A` | 基线报告没有明确 top 瓶颈，记录跳过原因 |
| `P3_IN_PROGRESS` | `P3_PASS` | 定向优化有可验证收益且可靠性无回归 |
| `P3_IN_PROGRESS` | `P3_FAIL` | 无显著收益或出现可靠性回归，已回滚 |
| `P3_PASS` / `P3_N/A` / `P3_FAIL` | `P4_IN_PROGRESS` | 进入证据归档 |
| `P4_IN_PROGRESS` | `P4_PASS` | 报告、截图、原始数据、简历数字全部可追溯 |
| `P4_IN_PROGRESS` | `P4_FAIL` | 存在无法追溯的数字或缺失证据 |
| `P4_PASS` | `COMPLETE` | 总 goal 完成 |
| 任意状态 | `BLOCKED` | 触发本文件“停止条件” |

### 3.3 门禁规则

1. **只有 `PASS` 才能进入下一阶段。** 唯一例外是 Phase 3 的 `P3_N/A`：基线没有明确瓶颈时，记录原因后可直接进入 Phase 4。
2. 每个阶段开始前必须读取 `bench/GOAL.md` 和 `bench/status.md`；阶段结束后必须更新 `bench/status.md`。
3. Phase 0 未通过，不得开始正式压测。
4. Phase 1 未通过，不得开始 Agent / RAG。
5. Phase 2 完成前不得修改业务语义或性能路径；允许在 Phase 0 增加 pprof、指标、bench 工具和 mock 代码。Phase 3 才允许修改业务逻辑，且只能针对基线报告中排名靠前的 1-2 个瓶颈。
6. 全流程固定 commit、配置、数据量；任何配置变化必须写入 `bench/status.md`，并说明是否影响可比性。
7. Phase 3 一次只改一项；每项改动独立 commit（如允许提交）、独立复测、独立记录。
8. 任何“0 丢失”结论必须有发送数、落库数、Outbox 发布数、消费数、WS 投递数的对账证据。
9. 禁止用另一个项目的压测数据、估算值或“理论值”填充 ClaranAIM 报告。

## 4. 通用执行规范

### 4.1 目录约定

```text
bench/
  GOAL.md                 # 本文件：状态机与阶段规范
  status.md               # 实时状态账本
  MASTER_GOAL.md          # 可直接复制的大 goal 提示词
  README.md               # 环境与脚本使用说明（Phase 0 产出）
  k6/                     # k6 脚本
  seed/                   # 种子数据与 reset 脚本
  mockllm/                # mock LLM / embedding 服务
  scripts/                # 启停、故障注入、采集、对账脚本
  results/<run-id>/       # 原始 JSON、日志、Prometheus/pprof 快照
  report/                 # 01-baseline.md、02-agent-rag.md、03-optimization.md、04-resume.md
```

### 4.2 Run ID

统一格式：`AIM-<phase>-<scenario>-<yyyyMMdd-HHmm>-<load>`

示例：`AIM-P1-C3-20261124-1530-300vu`

### 4.3 每轮必须记录的元数据

| 字段 | 示例 |
|---|---|
| commit / branch | `e76a742 / main` |
| 测试机规格 | `20C / 16GB host, Docker VM 7.6GiB` |
| 服务运行方式 | `infra in Docker + Go services on host` |
| 服务集合 | `user/group/msg-core/msg-history/websocket/api-gateway` |
| 依赖版本 | MySQL 8.0 / Kafka 3.x / Redis 7 / Go 1.25 |
| 数据量 | `1000 users / 100 groups / 1000 conversations / 100k messages` |
| 场景与档位 | `C3 300 VU` |
| 时长 | `warmup 1min + measure 5min` |
| GOMAXPROCS | 实际值 |
| Kafka 分区数 | 实际值 |
| DB 池 | `max_open / max_idle / in_use / wait_count` |
| embedding | `provider / dim` |
| 开关 | `graph on/off, rerank on/off, trace sampling` |
| 结果路径 | `bench/results/<run-id>/` |

### 4.4 延迟口径

报告必须分开写，不能混成一个“P99”：

| 指标 | 口径 |
|---|---|
| HTTP 端到端 | k6 `http_req_duration`，网关入口到响应 |
| msg-core 写入 | `claran_business_duration_ms{domain="msg_core",operation="send_message"}` |
| Outbox 发布 | `event_outbox.published_at - event_outbox.created_at` |
| Kafka 消费年龄 | `claran_business_gauge{domain="kafka_consumer",name="message_age_seconds"}` |
| WS 投递 | 客户端收到时间 - 消息 `created_at`（本机时钟） |
| Agent 运行 | Agent API 端到端耗时 |
| RAG 检索 | RAG search API 端到端耗时 |

### 4.5 数据对账

每个发送类场景必须记录并核对：

| 项目 | 来源 |
|---|---|
| 发送成功数 | k6 2xx 请求数，且 `client_msg_id` 唯一 |
| 消息落库数 | `messages` 按内容前缀 `bench-<run-id>-` 过滤 |
| Outbox 发布数 | `event_outbox` 关联本轮 message ID，`status='published'` |
| Kafka 消费数 | `processed_events` 按 consumer / event_id 统计 |
| WS 投递数 | WS 客户端按 message ID 去重计数 |
| 丢失 | 发送成功但最终未落库 / 未发布 / 未消费 / 未投递 |
| 重复 | 同一 message ID 或 event ID 被成功投递多次；必须可被幂等消化 |

关键 SQL 示例：

```sql
-- 本轮消息数
SELECT COUNT(*) FROM messages WHERE content LIKE 'bench-<run-id>-%';

-- Outbox 状态分布
SELECT status, COUNT(*) FROM event_outbox GROUP BY status;

-- pending/retrying 最老年龄（秒）
SELECT TIMESTAMPDIFF(SECOND, MIN(created_at), NOW())
FROM event_outbox
WHERE status IN ('pending', 'retrying');

-- Outbox 发布延迟（毫秒）
SELECT
  AVG(TIMESTAMPDIFF(MICROSECOND, created_at, published_at)) / 1000 AS avg_ms,
  MAX(TIMESTAMPDIFF(MICROSECOND, created_at, published_at)) / 1000 AS max_ms
FROM event_outbox
WHERE published_at IS NOT NULL;

-- 消费者处理状态
SELECT consumer, state, COUNT(*)
FROM processed_events
GROUP BY consumer, state;
```

### 4.6 安全与资源

- 基础设施优先只起：MySQL、Redis、Kafka、Etcd、OTel Collector、Prometheus、Grafana、Jaeger。
- 压测阶段不要起 Neo4j、Milvus、DTM；Phase 2 需要 RAG 时再单独启用 Milvus。
- Go 服务跑宿主机，Prometheus 通过 `host.docker.internal:190xx` 抓取。
- 先停掉无关容器释放内存；不要停止用户正在使用的其他项目容器，除非用户确认。
- 高并发只使用 mock LLM / mock embedding。

### 4.7 资源自适应协议（本机固定执行）

本机是 16GB Windows + 7.6GiB Docker VM，无法稳定支撑 100VU 以上长跑。
以下规则由 goal 自动执行，**不再向用户申请释放资源**：

| 主机可用内存 | 允许的最大并发 | 处理 |
|---|---|---|
| `>= 2.5GB` | 50 VU | 正常跑当前档位 |
| `1.5-2.5GB` | 20 VU | 自动把档位降到 20VU，记录 `resource_cap=20` |
| `1.0-1.5GB` | 10 VU | 自动降到 10VU，记录 `resource_cap=10` |
| `< 1.0GB` | 不启动新长跑 | 等待 30s 重试；仍不足则跳过扩展档位，标记 `resource_limited` |

补充规则：

1. 每轮开始前检查 `docker version` 与 `docker exec claran-mysql mysqladmin ping`。
2. 出现 Docker 500 或 etcd 注册超时：等待 60s，重启 Go 服务（不动 Docker），重试一次；
   仍失败则把并发减半后再试一次。
3. Phase 1 在本机的有效档位：C2 `10/20/50`、C3 `10/20/50`、C4 `5×50 / 10×100`。
   50VU 只作为 best-effort，若出现资源超时则记录为 `resource_limited`，不作为容量结论。
4. 所有自动降档、跳过、重试必须写入 `bench/status.md` 和对应 run 的 `resource.json`。
5. 报告中的容量结论只采用“0 失败且资源正常”的轮次；资源受限轮次单独列出。

## 5. Phase 0：环境与工具链就绪

### 5.1 目标

在不修改业务语义的前提下，把本地压测环境、种子数据、mock 服务、k6 骨架和故障脚本准备好，并跑通 smoke。不要开始正式容量测试，不要做性能优化。

### 5.2 任务

**P0-1 修复 MySQL 与 bench 库**

- 修复 `docker-compose.yml` 和 `deployment/docker/docker-compose.infra.yaml` 中 MySQL 的启动参数：
  - 增加 `--mysqlx-port=33061`，或
  - 增加 `--mysqlx=0`。
- 重建 `claran-mysql`，确认容器 healthy、3306 可连接。
- 创建独立数据库 `ClaranAIM_bench`；用 `MYSQL_DSN` 环境变量或 `.env.bench` 指向它。
- 不删除 volume，不清空现有开发库。

**P0-2 最小运行集**

- 基础设施：`mysql`、`redis`、`kafka`、`etcd`、`otel-collector`、`prometheus`、`grafana`、`jaeger`。
- Go 服务：`user-service`、`group-service`、`msg-core-service`、`msg-history-service`、`websocket-gateway`、`api-gateway`。
- 记录每个服务端口、metrics 地址、启动命令和日志路径。

**P0-3 mock LLM / embedding**

- 提供 OpenAI 兼容接口：
  - `POST /v1/chat/completions`
  - `POST /v1/embeddings`
- 固定 150ms 延迟、固定 token 统计、256 维 embedding、确定性返回。
- 不访问外网，不依赖真实 API key。
- 支持通过环境变量调整延迟、token 数和错误率。

**P0-4 种子数据**

- 1,000 用户、100 个群（每群 20-200 人）、1,000 个会话、100,000 条历史消息。
- 发送内容统一加前缀 `bench-<run-id>-`，便于对账。
- 种子脚本幂等；另写 `reset` 脚本。
- 只用 `ClaranAIM_bench` 库。

**P0-5 k6 与故障脚本**

- k6：登录、会话列表、历史消息、单聊发送、群聊发送、RAG 检索、Agent 运行。
- 发送请求必须带唯一 `client_msg_id`。
- 故障脚本：stop/start Kafka、MySQL、Redis；kill -9 msg-core；记录注入与恢复时间。

**P0-6 指标与 profile**

- 为 `msg-core-service`、`api-gateway`、`websocket-gateway` 增加 pprof（仅 127.0.0.1）。
- 增加 Go runtime 与 `sql.DBStats` 指标；若改动过大，至少为 msg-core 增加 DB 池指标。
- 确认 Prometheus 能抓到所有目标。

**P0-7 smoke**

- 登录 → 建私聊 → 发一条消息 → 检查 `messages`、`message_user_states`、`event_outbox`。
- 确认 Outbox 发布到 Kafka，WebSocket 客户端收到消息。
- 用 mock LLM 跑一次 Agent run；用 mock embedding 跑一次 RAG ingest + search。
- 查询四个可靠性指标，保存一条 Jaeger trace。

### 5.3 交付物

- `bench/README.md`
- `docs/bench/00-environment.md`
- `bench/mockllm/`、`bench/seed/`、`bench/k6/`、`bench/scripts/`
- `bench/results/<run-id>-smoke/`：日志、JSON、Prometheus 快照、trace ID、截图
- 未解决问题清单

### 5.4 退出条件

- `claran-mysql` healthy。
- 最小服务集可启动，端口和 metrics 正常。
- smoke 全链路通过。
- 种子脚本重复执行不产生重复数据。
- Prometheus targets UP，至少一条 Jaeger trace 可查。
- 不提交密钥；业务语义零改动。

## 6. Phase 1：核心基线 C2 / C3 / C4 / C10

### 6.1 目标

在当前 commit 上拿到可复现基线，回答“单机能撑多少、瓶颈在哪、故障后会不会丢”；不做业务优化。

### 6.2 场景

| ID | 场景 | 初始档位 | 扩展档位 | 时长 | 重点 |
|---|---|---|---|---|---|
| C2 | 会话列表 + 历史消息 | 10 VU | 20 VU（50 为 best-effort） | 60s | DB 查询、P95/P99、错误率 |
| C3 | 单聊发送 | 10 VU | 20 / 50 VU | 60s | send RPS、P99、Outbox 积压、Kafka lag |
| C4 | 群聊扇出 | 10 群（平均约 105 人）× 10 VU | 同群 × 20 VU | 60s | 扇出 P99、WS 连接、用户状态写入 |
| C10 | 故障注入 | 见 6.3 | - | - | 检测/恢复时间、丢失/重复 |

本机执行 60s 测量 + 10s 预热，跑 3 次取中位数；这是 16GB 单机下的自适应档位。

### 6.3 故障注入

| 故障 | 操作 | 观察 | 验收 |
|---|---|---|---|
| Kafka 宕机 | stop 60s 后 start | Outbox 积压、恢复后补发、重复 | 0 丢失；重复可幂等 |
| MySQL 宕机 | stop 30s 后 start | 写入失败、连接池恢复、事务回滚 | 无脏数据；恢复 < 1min |
| Redis 宕机 | stop 30s 后 start | 缓存穿透、限流是否失效 | 服务不崩；恢复后正常 |
| msg-core kill | `kill -9` | 请求失败、重启、Outbox 补偿 | 自动恢复；无消息丢失 |
| 网络延迟 | `tc netem delay 200ms`（可选） | P99、超时率、重试 | 超时可控；不雪崩 |

### 6.4 必须采集

- k6：RPS、P50/P95/P99、错误率、超时率。
- msg-core：发送 QPS、业务耗时、MySQL 慢查询。
- Outbox：pending/retrying/dead 数量、最老年龄、发布延迟、重试次数。
- Kafka：consumer lag、消息年龄、分区倾斜。
- WebSocket：在线连接数、投递成功/失败、重连次数。
- MySQL：QPS、连接池、锁等待、慢查询 Top 10。
- Redis：命中率、P99、连接数。
- 进程：CPU、RSS、goroutine（有 pprof/runtime 指标时）。

### 6.5 交付物

- `bench/report/01-baseline.md`
- `bench/results/<run-id>/` 原始 k6 JSON、Prometheus 快照、pprof、服务日志
- Grafana / Jaeger 截图
- 容量表、吞吐-延迟曲线、瓶颈排序、故障恢复表

### 6.6 退出条件

- C2/C3/C4 初始档位和至少一个扩展档位完成 3 次。
- C10 至少完成 Kafka、MySQL、msg-core kill 三项。
- 每一项有原始数据和命令。
- 0 未解释丢失；重复有幂等证据。
- 报告明确指出 top 3 瓶颈；不修改业务代码。

## 7. Phase 2：Agent / RAG / 长连接 C6 / C7 / C8（C5 / C9 可选）

### 7.1 目标

在 mock LLM / mock embedding 下测 Agent、RAG 和连接规模；不追求真实模型高并发。

### 7.2 场景

| ID | 场景 | 初始档位 | 扩展档位 | 时长 | 重点 |
|---|---|---|---|---|---|
| C6 | Agent 运行 | 10 并发 | 20 并发 | 60s | runs/s、P95、steps/run、token/run |
| C7 | RAG 入库 | 6 篇 × 256KB | - | 直到完成 | docs/min、chunk、失败率、CPU/内存 |
| C8 | RAG 检索 | 10 QPS | 20 QPS | 60s | P95/P99、Recall@5、错误率 |
| C5 | WS 长连接（可选） | 1k 连接 | 3k 连接 | 10min | 连接成功率、内存、goroutine |
| C9 | Soak（可选） | 50% 最大负载 | - | 30min | 内存、GC、DB 池、错误累积 |

### 7.3 配置要求

- C6：测试 Agent 的 BaseURL 指向 mock LLM；记录 steps/run、tool calls/run、token/run。
- C7/C8：mock embedding 固定 256 维；关闭图抽取；RAG router 使用 rule 模式。
- 真实 embedding / 真实模型只做低并发 smoke（≤10 请求），不作为容量数据。
- hash embedding 与 mock embedding 必须分开报告；rerank 开/关分开。

### 7.4 交付物

- `bench/report/02-agent-rag.md`
- 原始数据、Grafana/Jaeger 截图
- 50 条 RAG 评测集的 Recall@5 / MRR / nDCG 结果（离线）

### 7.5 退出条件

- C6/C7/C8 初始档位完成 3 次。
- 所有数据标注 embedding provider/dim、graph、rerank、mock 延迟。
- 真实模型只出现于 smoke 记录。
- 0 未解释错误。

## 8. Phase 3：定向优化与同口径复测

### 8.1 启动条件

只有 Phase 1 / 2 报告明确指出 top 1-2 个瓶颈时启动。没有明确瓶颈时，记录 `P3_N/A` 并进入 Phase 4。

### 8.2 候选优化

- Outbox worker：批量、并发、减少空转。
- `message_user_states`：批量 upsert / 合并写。
- DB 连接池：`SetMaxOpenConns`、`SetMaxIdleConns`、`ConnMaxLifetime` 配置化。
- Kafka 分区数与消费者并行度。
- WebSocket 扇出与连接管理。
- RAG / Milvus 批量入库、缓存。

### 8.3 规则

- 一次只改一项，独立 commit（如允许提交）。
- 每项改动前后跑相同场景、相同配置、相同数据量，3 次取中位数。
- 先跑单测；涉及 DB 的跑 MySQL 集成测试。
- 采集 pprof / Prometheus 前后对比。
- 提升在噪声内、出现可靠性回归或错误率上升时，回滚并记录 `P3_FAIL`。
- 不修改 API 语义、不降低校验、不修改延迟口径。

### 8.4 交付物

- `bench/report/03-optimization.md`
- before/after 表、改动 diff、复测原始数据、pprof 对比
- 若 no-go，记录原因和证据

### 8.5 退出条件

- 有可验证收益（建议 >10-15%）或明确 no-go。
- 0 丢失、重复可幂等、错误率不升。
- 所有结论可回溯到 `bench/results`。

## 9. Phase 4：证据归档与简历数字固化

### 9.1 目标

把 `bench/results` 里的真实数据整理成可复查的报告和简历 bullet，不新增测试结论。

### 9.2 任务

- 核对每个数字的来源命令、原始文件和 run ID。
- 生成 `bench/report/04-resume.md`，列 3-5 条 bullet：
  - 最大稳定 msg/s、P99、Outbox 积压；
  - Kafka 宕机 / MySQL 宕机 / kill -9 的恢复时间与 0 丢失；
  - Agent 并发与 P95；
  - RAG 检索 P95 与召回；
  - 优化前后对比（如有）。
- 更新 `README.md` 的实测小节，只写已验证数字。
- 附 Grafana / Jaeger 截图、commit、配置和 run ID。
- 删除无法追溯的数字。

### 9.3 退出条件

- 每个数字都能打开原始文件或 SQL 结果。
- 不写“预计”“大概”“理论上”。
- 不暴露密钥、用户隐私或聊天正文。

## 10. 完成定义

总 goal 只有同时满足以下条件才算 `COMPLETE`：

1. Phase 0 PASS；
2. Phase 1 PASS；
3. Phase 2 PASS；
4. Phase 3 PASS 或 `P3_N/A`；
5. Phase 4 PASS；
6. `bench/status.md` 记录最终状态、证据路径、未解决项；
7. 所有简历数字均来自 `bench/results` 和 `bench/report`。

## 11. 停止条件

出现以下任一情况，标记 `BLOCKED` 并停止，不要硬凑数据：

- MySQL 修复 2 次仍无法 healthy；
- Phase 0 smoke 连续 2 次失败；
- 种子数据无法幂等，或无法建立 bench 专用库；
- 核心基线连续 2 次因资源/环境失败；
- 无法保证 0 丢失或重复幂等；
- 发现测试会触碰生产、真实副作用或真实付费模型高并发；
- 仓库基线被改动且无法恢复固定 commit。

`BLOCKED` 时必须汇报：

- 当前阶段与状态；
- 阻塞原因；
- 已尝试的修复；
- 需要的用户决策或资源；
- 现有证据路径。

## 12. 附录：指标与最小运行集

### 12.1 PromQL

```promql
# 消息写入 P99（毫秒）
histogram_quantile(0.99,
  sum by (le) (
    rate(claran_business_duration_ms_bucket{domain="msg_core",operation="send_message"}[5m])
  )
)

# Outbox 积压
max(claran_business_gauge{domain="outbox",name="backlog"})

# Kafka 消费消息年龄（秒）
max(claran_business_gauge{domain="kafka_consumer",name="message_age_seconds"})

# WS 推送失败率
max(claran_business_gauge{domain="websocket",name="push_failure_rate"})

# WS 在线连接数
max(claran_business_gauge{domain="websocket",name="online_connections"})
```

### 12.2 最小运行集

| 类型 | 组件 |
|---|---|
| 基础设施 | MySQL、Redis、Kafka、Etcd、OTel Collector、Prometheus、Grafana、Jaeger |
| Go 服务 | user-service、group-service、msg-core-service、msg-history-service、websocket-gateway、api-gateway |
| 端口 | API 18080；WS 8081；user 9101；group 9002；msg-core 9003；msg-history 9004 |
| metrics | 19080 / 19081 / 19101 / 19002 / 19003 / 19004 |

### 12.3 报告模板

`bench/report/01-baseline.md` 至少包含：

1. 环境与 commit；
2. 数据量与配置；
3. 每个场景的 3 次结果与中位数；
4. 吞吐-延迟曲线与拐点；
5. Outbox / Kafka / DB / WS 分阶段指标；
6. 故障注入表；
7. 瓶颈排序与证据；
8. 限制与不可比项。

