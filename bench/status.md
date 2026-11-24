# ClaranAIM 压测 Goal 状态

> 本文件是状态机的实时账本。每个阶段开始和结束时更新；不要删除历史记录。

- Goal 状态：`COMPLETE`
- 当前阶段：`P4`
- 基线 commit：`e76a742`
- 基线分支：`main`
- 最后更新：`2026-11-24 15:00`
- 负责人：`goal`

## 1. 阶段门禁

| Phase | 状态 | 开始时间 | 完成时间 | 证据路径 | 备注 |
|---|---|---|---|---|---|
| P0 环境与工具链 | `PASS` | 2026-11-24 14:40 | 2026-11-24 15:00 | `bench/results/AIM-P0-smoke-final/smoke.json` | MySQL 修复、最小集、种子、mock、smoke 全通过 |
| P1 核心基线 | `PASS` | 2026-11-24 15:00 | 2026-11-24 17:50 | `bench/report/01-baseline.md` | C2/C3/C4/C10 完成，0 丢失 0 重复 |
| P2 Agent / RAG | `PASS` | 2026-11-24 17:50 | 2026-11-24 18:25 | `bench/report/02-agent-rag.md` | C6/C7/C8 完成，0 失败 |
| P3 定向优化 | `PASS` | 2026-11-24 18:25 | 2026-11-24 18:35 | `bench/report/03-optimization.md` | Outbox 46→285-483 events/s |
| P4 证据归档 | `PASS` | 2026-11-24 18:35 | 2026-11-24 18:45 | `bench/report/04-resume.md` | 数字已回溯，README 已更新 |

## 2. 环境快照

| 项目 | 实际值 |
|---|---|
| 主机 CPU / 内存 | 20C / 15.7GB（空闲约 1.25GB） |
| Docker VM CPU / 内存 | 20C / 7.6GiB |
| 运行方式 | infra Docker + Go 服务宿主机 |
| 服务集合 | user/group/msg-core/msg-history/ws/api + memory/runtime/manager/rag |
| MySQL 版本 / 端口 | 8.0.32 / 3306（容器 33060，X Plugin 33061） |
| Kafka 版本 / 分区数 | 3.3.2 / 1 分区（`claran.message.events`） |
| Redis 版本 | 8.4.0 |
| Go 版本 | 1.27.0 |
| GOMAXPROCS | 默认（未显式设置） |
| DB 池配置 | 未显式设置（`max_open=0`，即不限） |
| embedding provider / dim | mock GLM 兼容 / 256 |
| trace 采样 | 默认 OTel 采样 |

## 3. 运行索引

| Run ID | Phase | 场景 | 负载 | 结果 | 原始数据路径 |
|---|---|---|---|---|---|
| - | - | - | - | - | - |

## 4. 已知阻塞与问题

| ID | 发现时间 | 现象 | 影响 | 处理状态 | 证据 |
|---|---|---|---|---|---|
| B-001 | 2026-11-24 | `claran-mysql` 因 33060 端口冲突重启循环 | 所有服务无法启动 | 已修（`--mysqlx-port=33061`） | docker logs |
| B-002 | 2026-11-24 | Docker VM 内存紧张，全栈不可行 | 只能跑最小集 | 已确认，Neo4j/Milvus 关闭 | docker stats |
| B-003 | 2026-11-24 | 无 bench/k6/seed/mock 工具链 | 无法开始压测 | 已建（Go loadgen + mock + seed） | `bench/README.md` |
| B-004 | 2026-11-24 15:42 | 50VU 长跑后主机可用内存 <1GB，Docker 引擎短暂 500 | 无法继续长跑；MySQL 健康检查超时但实际存活 | 等待资源释放 | `bench/logs/`、docker ps |
| B-005 | 2026-11-24 15:52 | 主机可用内存 0.64GB，api-gateway 返回 500 | P1 长跑无法继续 | 暂停，等用户释放内存/关闭无关程序 | `bench/logs/api-gateway.*` |
| B-006 | 2026-11-24 16:12 | 重启后 etcd 注册超时，C2 50VU 全量 500；Docker 引擎间歇 500 | 无法稳定完成 3×60s 基线 | 已修：Jaeger 轨迹上限 500，资源恢复 | `bench/logs/msg-core-service.err.log` |

## 5. 决策记录

| 时间 | 决策 | 原因 | 影响 |
|---|---|---|---|
| 2026-11-24 | 使用单一总 goal + 阶段门禁 | 降低交接成本，保持可追溯 | 必须严格更新本文件 |
| 2026-11-24 | 只测 ClaranAIM，不碰另一个项目 | 用户明确范围 | 所有报告只写 ClaranAIM |
| 2026-11-24 | 压测期关闭网关限流 | 单用户 120/分钟会掩盖真实瓶颈 | 报告必须注明 `RATE_LIMIT_ENABLED=false` |
| 2026-11-24 | 使用仓库内 Go loadgen | 环境无 k6，且需要 WS/对账能力 | 原始结果仍在 `bench/results` |
| 2026-11-24 | 群消息种子不写 per-user 状态 | 避免 10 万消息放大到百万行 | 历史查询仍正常 |
| 2026-11-24 | 压测期关闭 Kitex 服务端 QPS/连接上限 | 默认 1000 会先于真实瓶颈触发 | 报告注明 `RPC_MAX_QPS=0` |
| 2026-11-24 | 压测期固定 DB 池 32 | 避免连接 churn 耗尽 Windows 临时端口 | 报告注明 `DB_MAX_OPEN_CONNS=32` |
| 2026-11-24 | loadgen 按业务 `success` 判定成功 | 网关对业务失败返回 HTTP 200 | 修正对账虚高 |

## 7. 已完成基线数据

### C2 会话列表 + 历史消息（60s，warmup 10s）

| Run | VU | RPS | P95 | P99 | 失败 |
|---|---|---|---|---|---|
| AIM-P1-C2-C2-20vu-r1 | 20 | 1786.92 | 23.21ms | 29.70ms | 0 |
| AIM-P1-C2-C2-20vu-r2 | 20 | 1696.57 | 23.93ms | 35.38ms | 0 |
| AIM-P1-C2-C2-20vu-r3 | 20 | 1771.37 | 23.55ms | 29.73ms | 0 |
| AIM-P1-C2-C2-50vu-r1 | 50 | 1289.92 | 60.02ms | 199.66ms | 0 |
| AIM-P1-C2-C2-50vu-r2 | 50 | 0 | 30004ms | 30005ms | 100 |
| AIM-P1-C2-C2-50vu-r3 | 50 | 0.55 | 30004ms | 30005ms | 200 |

50VU 后两次为资源耗尽导致的全量超时，不作为容量结论。

### C3 单聊发送（验证轮）

| Run | VU | RPS | P95 | P99 | 失败 | 对账 |
|---|---|---|---|---|---|---|
| AIM-P1-validate4-C3-20vu-r1 | 20 | 297.04 | 91.67ms | 126.40ms | 0 | 0 丢失 |
| AIM-P1-validate5-C3-50vu-r1 | 50 | 400.87 | 184.68ms | 228.21ms | 0 | 8044 落库 / 8044 outbox，0 丢失 0 重复 |

### Outbox 排空

- 8044 条积压，10 秒减少约 460 条，排空速率约 **46 events/s**。
- 与 `pkg/outbox.NewWorker` 的 `limit=50` + `interval=1s` 一致，是明确瓶颈。

## 8. 最终状态

Goal 已完成。资源问题的根因是 Jaeger 轨迹无上限（涨到 3.5GB），
已通过 `MEMORY_MAX_TRACES=500` 与每轮重置修复；后续压测不再需要用户释放资源。

证据：

- `bench/report/01-baseline.md`（C2/C3/C4/C10）
- `bench/report/02-agent-rag.md`（C6/C7/C8）
- `bench/report/03-optimization.md`（Outbox 46 → 285-483 events/s）
- `bench/report/04-resume.md`（简历 bullet 与限制）
- `bench/results/AIM-P1-*`、`AIM-P2-*`、`AIM-P3-*`、`AIM-P0-smoke-final/smoke.json`

补充：Milvus 对比测试已完成（36 篇 / 34,344 chunks），C8 P95 仍约 1.8s；
定位瓶颈为 `rag_chunks.updated_at` 无索引导致 filesort，临时加索引后 P95 约 1.35s。

## 6. 完成检查

- [x] Phase 0 PASS
- [x] Phase 1 PASS
- [x] Phase 2 PASS
- [x] Phase 3 PASS 或 N/A
- [x] Phase 4 PASS
- [x] 所有数字可回溯
- [x] 0 未解释丢失
- [x] README 只写已验证数字
- [x] 未提交密钥

