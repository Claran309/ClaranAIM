# ClaranAIM 压测总 Goal（启动提示词）

> 将下面整段内容作为 goal 的初始提示词。详细阶段规范在 `bench/GOAL.md`，状态账本在 `bench/status.md`。

```text
【ClaranAIM 压测与运维总 goal】

目标：在 D:\CodeStudy\GoProjects\src\ClaranAIM 的 e76a742 基线上，按 bench/GOAL.md 完成 Phase 0-4，产出可复现的容量基线、故障恢复数据、Agent/RAG 基线、定向优化结论和可写进简历的真实数字。

执行方式：
1. 开始前读取 bench/GOAL.md 和 bench/status.md，确认当前状态与门禁。
2. 每个阶段开始前重新读取 bench/GOAL.md 对应章节，不要依赖聊天上下文记忆。
3. 每个阶段结束更新 bench/status.md：写状态、时间、证据路径、未解决项、配置变化。
4. 只有当前阶段 PASS 才能进入下一阶段；FAIL 最多重试 2 次，之后标记 BLOCKED 并停止。
5. 每完成一个阶段，用简短消息汇报：阶段、状态、关键数字、证据路径、下一步。

范围与边界：
- 只做 ClaranAIM；不修改或运行 D:\CodeStudy\growing 下的另一个项目。
- 不碰生产环境；不触发真实送礼、私信、动态、关注等副作用。
- 高并发只用 mock LLM / mock embedding；真实模型只做低并发 smoke。
- Phase 2 完成前不修改业务语义或性能路径；允许 Phase 0 增加观测、pprof、bench 工具代码；Phase 3 只改基线报告指出的 top 1-2 个瓶颈。
- 不提交 .env、API key、cookie；不执行 docker compose down -v；不删除 volume。
- 不 push 远程，除非用户明确要求。

硬门禁：
- Phase 0：MySQL healthy、最小服务集可启动、种子幂等、smoke 全链路通过；否则不得开始正式压测。
- Phase 1：C2/C3/C4 至少初始档位 + 一个扩展档位各 3 次，C10 至少 Kafka/MySQL/kill msg-core 三项完成；否则不得进入 Phase 2。
- Phase 2：C6/C7/C8 初始档位各 3 次，配置标注完整；否则不得进入 Phase 3/4。
- Phase 3：只在基线报告有明确 top 瓶颈时启动；一次只改一项，同场景复测；无收益或可靠性回归则回滚并记录 no-go。
- Phase 4：每个数字必须能打开原始文件或 SQL 结果；否则不得声称完成。

固定纪律：
- 固定 commit、配置、数据量；每组 3 次取中位数；预热 1 分钟。
- 明确区分 HTTP 端到端、msg-core 业务、Outbox、Kafka、WS 各阶段延迟。
- 发送类场景必须用唯一 client_msg_id，并用内容前缀 bench-<run-id>- 做对账。
- 0 丢失必须有发送数、落库数、Outbox 发布数、Kafka 消费数、WS 投递数的证据。
- 所有结论放 bench/report/，所有原始数据放 bench/results/<run-id>/。

启动动作：
1. 检查 git status 与当前 commit，记录到 bench/status.md。
2. 读取 bench/GOAL.md 的 Phase 0，开始修复 MySQL、建立 bench 库、准备最小运行集。
3. 不要先跑完整 C1-C10 矩阵；先完成 Phase 0 smoke，再按 GOAL.md 只跑 C2/C3/C4/C10。
4. 遇到阻塞按 GOAL.md 的停止条件处理，不要跳过门禁。

完成定义：Phase 0-2 PASS；Phase 3 PASS 或 P3_N/A；Phase 4 PASS；bench/status.md 记录最终状态；所有简历数字可追溯。否则标记 BLOCKED 或 IN_PROGRESS，不得声称 goal 完成。
```

