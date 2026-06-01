# ClaranAIM 项目路线图

## 1. 项目定位

ClaranAIM 是一个 AIM 系统：Agent + Instant Messaging。IM 负责用户、好友、群聊、消息、文件、历史、推送和会话事实；Agent 负责在真实 IM 上下文中理解消息、调用工具、沉淀知识、生成总结并辅助执行任务。

项目目标不是在聊天页旁边放一个 AI 问答框，而是让 Agent 成为 IM 的原生成员：可以作为真实系统用户进入私聊和群聊，可以被 @，可以基于会话上下文工作，也必须受权限、审计、限流和配置约束。

本文档只记录当前进度、剩余缺口和后续路线。详细架构、接口和历史变更见 `README.md`、`docs/AI assistent/TechArch.md`、`docs/AI assistent/APIdoc.md` 和 `docs/update/`。

## 2. 当前完成度总览

- [x] IM 基础服务：用户、好友、好友分组、群组、群成员、十位用户 UID、十位群号、群号加入、消息发送、编辑、撤回、已读、本地删除、历史查询、消息搜索、文件上传、手动翻译均已有后端接口或基本闭环。
- [~] IM 高级同步：WebSocket、Outbox、消息事实表和用户消息状态已具备基础；离线推送、多端完整同步、客户端本地数据库缓存、乱序补偿和系统级 Ack 仍未完成。
- [x] Agent 基础管理：Agent 创建、配置、真实系统用户身份、权限、路由规则、计费、运行、总结、问答、洞察、回复候选、会话列表和前端管理入口已落地。
- [~] Agent-Native 事件链路：Kafka/Outbox、统一 IM 事件、订阅规则、事件调度、幂等记录、私聊默认触发、群聊 @/命令/规则触发、静默记录和审计已具备 MVP；主动私聊、主动拉群、完整审批持久化、checkpoint/resume 和长期任务仍未完成。
- [x] Memory：memory-service、`memory_facts`、用户/群/会话/session 记忆、记忆治理接口、前端记忆管理和 Agent 运行摘要沉淀已落地。
- [~] Memory 增强：当前向量化记忆主要是状态与引用预留，跨会话语义召回、自动抽取确认流和更细的隐私治理仍需增强。
- [x] Settings：settings-service、LLM 预设、Prompt 模板、Skill 上传/列表/详情/编辑/删除、翻译 Prompt 和 Agent 创建时选择 LLM 预设已落地。
- [~] Settings 安全：API Key 当前可用但缺少生产级加密存储、密钥轮换和审计。
- [x] RAG：rag-service、文档上传、txt/md/pdf/docx/图片/代码解析、父子 chunk、Hybrid Search、BM25、RRF、rerank、CRAG、Self-RAG、Adaptive Router、GraphRAG 子图和 Text-to-SQL 路由占位已落地。
- [~] RAG 增强：ppt/pptx、真实 Web RAG 闭环、结构化数据库安全 SQL 执行、模型质检成本控制和生产级 Milvus 运维仍需完善。
- [x] Knowledge Graph：knowledge-service、Kitex RPC、图谱查询、节点详情、关系详情、类型/关系/社区过滤、G6 可视化、邻域查询、最短路径和路径高亮已落地。
- [~] Knowledge Graph 增强：实体归一化、社区摘要持久化、Leiden 社区划分、图谱候选审核和专用图数据库仍未完成。
- [~] Tool / Skill：runtime 已有基础工具、Web Search/RAG 工具、代码/总结类工具、skill-creator 和用户上传 Skill；MCP Gateway、完整工具注册市场、高风险动作持久化审批和工具调用重试仍未完成。
- [~] Governance：Kafka、Transactional Outbox、DTM 基础设施、创建群 DTM Saga、限流、熔断、RPC timeout、本地 INFO/ERR 日志已落地；Prometheus、Jaeger、Grafana、ELK、K8s、CI/CD 和管理后台仍未完成。
- [~] Frontend：聊天、好友、群、Agent、Memory、Settings、RAG、Knowledge Graph、Markdown 渲染、Action Card MVP 已有；移动端、CLI/TUI、管理后台和更完整的审批交互未完成。

## 3. 阶段路线图

### Phase 0：稳定化与验收基线

- [x] 非破坏性 AutoMigrate，服务启动不再 DropTable 清数据。
- [x] 雪花 ID、十位用户 UID、十位群号基础能力。
- [x] Access Token / Refresh Token，Token 携带 role。
- [x] 本地 Zap 日志文件输出，ERR 日志集中归档。
- [x] 基础测试覆盖：JWT、配置、缓存、Outbox、DTM、RAG、Memory、Agent 核心逻辑等。
- [~] 全项目仍缺少统一端到端验收脚本和稳定 CI。

### Phase 1：高级 IM 能力

- [x] 用户资料、头像、好友、好友分组、好友备注。
- [x] 群创建、群号加入、邀请、踢人、转让群主、公告、置顶、禁言、角色设置。
- [x] 私聊/群聊会话、消息发送、图片/文件/语音类型消息、编辑、撤回、引用、@、已读、本地删除。
- [x] 历史消息、会话列表、基础搜索和文件服务。
- [x] 手动翻译：msg-core-service 调用用户 LLM 设置并缓存翻译结果。
- [~] WebSocket 在线推送和 Outbox 事件已接入，但离线推送、上线同步、多端完整同步和乱序补偿仍不完整。
- [ ] 实时审核、自动翻译、客户端本地数据库级缓存和云端漫游策略。

### Phase 2：Agent 会话感知层

- [x] agent-manager-service 与 agent-runtime-service 分离，Agent 作为真实系统用户。
- [x] Agent 配置、昵称、头像、签名、API Key、BaseURL、模型、工作目录、工具策略和运行参数。
- [x] Agent 权限：owner/admin/operator/viewer 的授权、撤销和查询。
- [x] Agent 运行接口：运行、总结、问答、洞察、回复候选、会话列表。
- [x] Agent 长会话 JSONL 持久化，支持跨重启恢复基础上下文。
- [x] Agent 会话上下文读取条数、记忆召回条数、最大输出 Token、创造性和群聊触发方式可配置。
- [~] 未读摘要、“我错过了什么”、主题聚合和更深时间窗口检索仍是 MVP 或待增强。

### Phase 3：Agent-Native IM 原生化

- [x] 统一 IM 事件 `IMEventPayload`，覆盖消息创建、编辑、撤回、已读、文件/图片/语音和群成员变化等基础事件。
- [x] Agent 订阅规则、事件调度器、调度事实表、审计记录、幂等键和 Agent 回复 `client_msg_id` 幂等。
- [x] 私聊 Agent 默认触发，群聊按 @、命令、关键词或静默记录规则触发。
- [x] 前端展示 Agent 思考中、完成、失败等原生状态，并提供 Agent 上下文侧边栏 MVP。
- [~] Context Builder 已能读取 Agent 可见消息窗口和基础附件引用，但 RAG/Memory/Task 的深度融合仍需增强。
- [ ] Agent 主动私聊、主动拉群、主动 @ 用户、创建任务、写文件等行为的授权、频率限制和审计闭环。
- [ ] 运行 checkpoint/resume、心跳、取消、长期任务队列和持久化审批。

### Phase 4：Agent 记忆与用户/群画像

- [x] memory-service 与 `memory_facts` 基础事实记忆表。
- [x] 用户记忆、群记忆、聊天记忆、跨会话个体记忆和 Agent 运行摘要。
- [x] 用户可查看、编辑、删除和关闭自己的记忆。
- [x] 用户发言习惯分析默认本人可见的基础模型。
- [x] 不同 Agent、不同 Session 的记忆隔离。
- [~] 向量化记忆目前是 `vector_status/embedding_ref` 预留，真实语义召回和自动抽取确认流仍需增强。

### Phase 4.5：系统设置、LLM 预设与 Skill

- [x] settings-service 独立 Kitex RPC 服务。
- [x] 用户级 LLM 预设：BaseURL、API Key、模型名、用途和默认标记。
- [x] 创建 Agent 时可选择已保存的 LLM 预设。
- [x] Prompt 模板配置，已服务翻译 Prompt，并可扩展到总结、回复候选、知识抽取。
- [x] Skill 上传文件或压缩包、摘要提取、列表、详情、前端编辑和删除。
- [~] Skill 已能作为 Agent 配置来源，但完整 Skill 版本管理、灰度、权限和执行审计仍需完善。
- [ ] API Key 加密存储、密钥轮换和敏感配置审计。

### Phase 5：RAG 与文档知识库

- [x] rag-service、文档入库、文件上传、文档列表和检索接口。
- [x] 文档解析：txt、Markdown、PDF、docx、图片 OCR、Go/代码文件和通用文本。
- [x] 分层切片：Markdown 标题、父子 chunk、代码结构切分、通用段落切分。
- [x] Hybrid Search：dense + BM25，并用 RRF 合并。
- [x] Model Rerank：检索 topN 后用模型精排 topK。
- [x] CRAG evaluator：按 relevance、coverage、specificity、conflict 输出 correct/incorrect/ambiguous。
- [x] Self-RAG：Retrieve、IsRel、IsSup、IsUse 结构化判断，由应用代码执行检索。
- [x] Adaptive RAG：规则 + LLM Router，选择 direct、project_rag、strict_rag、web_rag、memory_rag 或 tool_action。
- [x] GraphRAG MVP：实体、关系、社区和子图查询。
- [~] Text-to-SQL RAG 当前是路由占位，缺少安全 SQL 执行和结构化数据源管理。
- [ ] ppt/pptx 解析、真实 Web RAG 闭环、RAG 结果人工审核和知识生命周期管理。

### Phase 5.5：知识图谱可视化

- [x] knowledge-service 独立 Kitex RPC 服务。
- [x] api-gateway 通过 RPC 调用 knowledge-service，knowledge-service 再通过 RPC 读取 rag-service 图谱子图。
- [x] 前端知识图谱页面：搜索节点、类型过滤、关系过滤、社区过滤、拖拽缩放、节点详情、边详情和证据展示。
- [x] 节点邻域子图、两节点最短路径查询和路径高亮。
- [x] 图谱查询复用当前过滤条件，避免详情、邻域和路径绕过用户筛选。
- [~] 当前图谱底层事实主要由 rag-service GraphRAG 表承载，尚未形成独立图谱候选审核工作台。
- [ ] 实体归一化、Leiden 社区划分、社区摘要持久化、图谱候选审核和专用图数据库评估。

### Phase 5.6：结构化卡片协议

- [x] Agent Action Card JSON MVP，覆盖审批、任务分配、知识引用、错误诊断和文件学习结果基础字段。
- [x] 前端基础 Card Renderer，可把 Agent JSON 渲染为可读卡片。
- [~] 卡片操作当前偏前端提示或局部接口，缺少统一持久化 action、幂等、权限和审计。
- [ ] 卡片版本兼容策略、旧客户端降级纯文本和通用卡片回调协议。

### Phase 6：Tool / Skill / MCP 工作流

- [x] runtime 基础工具、Web Search、RAG 工具、代码解释/建议/审查摘要/测试建议/文档总结和 skill-creator。
- [x] 用户可上传全局或单 Agent Skill，并在前端查看摘要和编辑内容。
- [~] 工具策略、安全中间件和审批中断已有基础，但审批持久化、恢复和超时清理仍不足。
- [ ] MCP Gateway：统一承接文件解析、RAG、日历、工单、Git、外部 API 等工具调用。
- [ ] 工具注册市场、工具权限、工具调用审计、失败重试和高风险动作二次确认。
- [ ] 从聊天创建任务、整理 PRD、生成会议纪要、日报、周报、Issue/PR/发布记录总结的完整工作流。

### Phase 7：多 Agent 群聊协作

- [ ] Agent Cooperation Service：Leader、子任务、阻塞等待、结果汇总和验收。
- [ ] 多角色 Agent：产品、研发、测试、运维、设计、CEO 等角色配置。
- [ ] 通过 @、命令或规则召集一个或多个 Agent。
- [ ] 多 Agent 上下文共享、任务交接、结果合并、超时、取消、重试、人工接管和审计。
- [ ] 个人私密 Agent 与群协作 Agent 的权限边界。

### Phase 8：治理、观测与生产化

- [x] Kafka 事件流一期和统一事件结构。
- [x] Transactional Outbox，覆盖关键消息和群组事件的发布前崩溃窗口。
- [x] DTM 基础设施和创建群 Saga MVP。
- [x] API Gateway 用户/IP 令牌桶限流。
- [x] Kitex 客户端 timeout、长运行 Agent RPC timeout 特例和 circuit breaker。
- [x] 本地 Zap 日志，INFO 按服务分目录，ERR 集中归档。
- [~] consumer 幂等和死信已有 `pkg/eventbus` 能力，但不是所有消费者都统一接入生产级监控。
- [ ] Prometheus、Jaeger、Grafana、ELK、K6 压测、Kubernetes、滚动升级、灰度发布和 CI/CD。
- [ ] 管理后台：用户、群、系统消息、媒体审核、Agent 审计、知识入库审核和成本监控。
- [ ] 敏感信息识别、脱敏、记忆治理、知识生命周期和 Agent 权限治理生产化。

### Phase 9：客户端与体验扩展

- [x] Web 前端已有聊天、好友、群、Agent、Memory、Settings、RAG 和 Knowledge Graph 页面。
- [x] Markdown 消息渲染、Agent 流式对话样式、Action Card MVP 和知识图谱交互。
- [~] Agent 上下文侧边栏、运行状态、Skill 页面和触发规则页面已有 MVP，但交互细节仍需持续打磨。
- [ ] 管理端前端：媒体审核、系统消息、Agent 审计、知识候选审核和成本面板。
- [ ] CLI/TUI 客户端、移动端客户端或 PWA。
- [ ] 更完整的文件预览、图片查看、语音播放、表情库和组合搜索体验。

## 4. 下一阶段优先级

1. 补齐 Agent 运行生产化：持久化审批、checkpoint/resume、取消、心跳、长期任务队列和运行审计。
2. 完善 IM 同步：离线推送、上线同步、多端游标、Ack/重试/乱序补偿和客户端本地缓存策略。
3. 深化 RAG + Knowledge Graph：图谱候选审核、实体归一化、社区摘要、RAG 来源治理和 ppt/pptx 解析。
4. 推进 Tool / Skill / MCP：统一工具注册、权限、审计、失败重试和高风险动作确认。
5. 建立治理与观测：Prometheus、链路追踪、消费延迟、成本面板、K6 压测和管理后台。

## 5. 状态口径与假设

- `[x]` 表示代码中已有明确服务、接口、数据模型或前端入口，并能形成基本闭环。
- `[~]` 表示主链路已跑通，但还有明显生产化、深度能力或体验缺口。
- `[ ]` 表示当前代码中没有可用闭环，或只是占位/规划。
- 完成度按当前仓库事实判断，不按理想生产级系统判断。
- 微服务事实以 `internal/*-service`、`cmd/*-service`、`idl/*.thrift`、`internal/api-gateway/router/router.go` 和 `docs/update/*` 为依据。
- 当前对外业务命名统一使用 Agent；历史生成代码包名属于内部实现细节，不作为产品接口规划。
