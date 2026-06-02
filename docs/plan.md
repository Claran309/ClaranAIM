# ClaranAIM 项目路线图

## 1. 项目定位

ClaranAIM 是一个 AIM 系统：Agent + Instant Messaging。IM 负责用户、好友、群聊、消息、文件、历史、推送和会话事实；Agent 负责在真实 IM 上下文中理解消息、调用工具、沉淀知识、生成总结并辅助执行任务。

项目目标不是在聊天页旁边放一个 AI 问答框，而是让 Agent 成为 IM 的原生成员：可以作为真实系统用户进入私聊和群聊，可以被 @，可以基于会话上下文工作，也必须受权限、审计、限流和配置约束。

本文档只记录当前进度、剩余缺口和后续路线。详细架构、接口和历史变更见 `README.md`、`docs/AI assistent/TechArch.md`、`docs/AI assistent/APIdoc.md` 和 `docs/update/`。

## 2. 当前完成度总览

- [x] IM 基础服务：用户、好友、好友分组、群组、群成员、十位用户 UID、十位群号、群号加入、消息发送、编辑、撤回、已读、本地删除、历史查询、消息搜索、文件上传、手动翻译均已有后端接口或基本闭环。
- [~] IM 高级同步：WebSocket、Outbox、消息事实表、用户消息状态、上线同步、离线索引拉取和前端消息缓存已有 MVP；离线推送、多端完整同步、云端漫游、乱序补偿和系统级 Ack 仍需增强。
- [x] Agent 基础管理：Agent 创建、配置、真实系统用户身份、权限、路由规则、计费、运行、总结、问答、洞察、回复候选、会话列表和前端管理入口已落地。
- [~] Agent-Native 事件链路：Kafka/Outbox、统一 IM 事件、订阅规则、事件调度、幂等记录、私聊默认触发、群聊 @/命令/规则触发、静默记录、审计和聊天归档任务入口已具备 MVP；主动私聊、主动拉群、完整审批持久化、checkpoint/resume 和长期任务仍未完成。
- [x] Memory：memory-service、`memory_facts`、用户/群/会话/session 记忆、记忆治理接口、前端记忆管理和 Agent 运行摘要沉淀已落地。
- [~] Memory 增强：Memory-RAG、Milvus 召回、候选记忆和 conversation-intelligence-service 聊天归档 MVP 已落地；已支持 LLM 提炼器、失败重试和归档任务状态页，但冲突治理、更细隐私策略和生产级评估仍需增强。
- [x] Settings：settings-service、LLM 预设、Prompt 模板、Skill 上传/列表/详情/编辑/删除、翻译 Prompt 和 Agent 创建时选择 LLM 预设已落地。
- [~] Settings 安全：LLM API Key 和 MCP Secret 已有应用层 AES-GCM 加密存储 MVP；生产级 KMS、密钥轮换和敏感配置审计仍未完成。
- [x] RAG：rag-service、文档上传、txt/md/pdf/docx/图片/代码解析、父子 chunk、Hybrid Search、BM25、RRF、rerank、CRAG、Self-RAG、Adaptive Router 和 GraphRAG 子图已落地。
- [x] Web Search Augmentation：web-search-service、Kitex RPC、搜索结果、可信源排序、网页正文抓取、正文清洗、相关段落截取、网关接口和 Agent `web_search` 工具已落地。
- [~] RAG 增强：ppt/pptx、模型质检成本控制和生产级 Milvus 运维仍需完善；结构化数据库问答暂不进入当前路线。
- [x] Knowledge Graph：knowledge-service、Kitex RPC、图谱查询、节点详情、关系详情、类型/关系/社区过滤、G6 可视化、邻域查询、最短路径和路径高亮已落地。
- [~] Knowledge Graph 增强：实体 canonical key 归一化、Leiden-like 社区划分、社区摘要持久化和图谱候选审核工作台已有 MVP；严格 Leiden 库级实现、候选结果反写 GraphRAG 和专用图数据库仍未完成。
- [~] Tool / Skill / MCP：runtime 已有基础工具、skill-creator、用户上传 Skill 和 MCP Gateway MVP；内置 WebSearch/Memory/RAG/KnowledgeGraph/ConversationSummary 工具已统一经 MCP 暴露，远程 HTTP/SSE MCP 可配置；完整工具市场、高风险动作持久化审批和工具调用重试仍未完成。
- [~] Governance：Kafka、Transactional Outbox、DTM 基础设施、创建群 DTM Saga、限流、熔断、RPC timeout、本地 INFO/ERR 日志和 admin-service 全局管理台 MVP 已落地；Prometheus、Jaeger、Grafana、ELK、K8s 和生产级 CI/CD 仍未完成。
- [~] Frontend：聊天、好友、群、Agent、Memory、Settings、RAG、Knowledge Graph、管理台、媒体预览、Markdown 渲染、Action Card MVP 已有；移动端、CLI/TUI、完整管理员权限矩阵和更完整的审批交互未完成。

## 3. 阶段路线图

### Phase 0：稳定化与验收基线

- [x] 非破坏性 AutoMigrate，服务启动不再 DropTable 清数据。
- [x] 雪花 ID、十位用户 UID、十位群号基础能力。
- [x] Access Token / Refresh Token，Token 携带 role。
- [x] 本地 Zap 日志文件输出，ERR 日志集中归档。
- [x] 基础测试覆盖：JWT、配置、缓存、Outbox、DTM、RAG、Memory、Agent 核心逻辑等。
- [~] 已有 `scripts/e2e-smoke.ps1` 和 GitHub Actions smoke CI 基线；仍缺少真实浏览器端到端、Docker 依赖编排和稳定测试数据。

### Phase 1：高级 IM 能力

- [x] 用户资料、头像、好友、好友分组、好友备注。
- [x] 群创建、群号加入、邀请、踢人、转让群主、公告、置顶、禁言、角色设置。
- [x] 私聊/群聊会话、消息发送、图片/文件/语音类型消息、编辑、撤回、引用、@、已读、本地删除。
- [x] 历史消息、会话列表、基础搜索和文件服务。
- [x] 手动翻译：msg-core-service 调用用户 LLM 设置并缓存翻译结果。
- [~] WebSocket 在线推送、Outbox 事件、上线同步、离线索引读取和前端缓存合并已接入；离线推送、多端完整同步、Ack/重试和乱序补偿仍不完整。
- [~] 客户端本地缓存已有浏览器 localStorage 最近消息缓存 MVP；IndexedDB、本地数据库级缓存和云端漫游策略未完成。

### Phase 2：Agent 会话感知层

- [x] agent-manager-service 与 agent-runtime-service 分离，Agent 作为真实系统用户。
- [x] Agent 配置、昵称、头像、签名、API Key、BaseURL、模型、工作目录、工具策略和运行参数。
- [x] Agent 权限：owner/admin/operator/viewer 的授权、撤销和查询。
- [x] Agent 运行接口：运行、总结、问答、洞察、回复候选、会话列表。
- [x] Agent 长会话 JSONL 持久化，支持跨重启恢复基础上下文。
- [x] Agent 会话上下文读取条数、记忆召回条数、最大输出 Token、创造性和群聊触发方式可配置。
- [~] 未读摘要、“我错过了什么”

### Phase 3：Agent-Native IM 原生化

- [x] 统一 IM 事件 `IMEventPayload`，覆盖消息创建、编辑、撤回、已读、文件/图片/语音和群成员变化等基础事件。
- [x] Agent 订阅规则、事件调度器、调度事实表、审计记录、幂等键和 Agent 回复 `client_msg_id` 幂等。
- [x] 私聊 Agent 默认触发，群聊按 @、命令、关键词或静默记录规则触发。
- [x] 前端展示 Agent 思考中、完成、失败等原生状态，并提供 Agent 上下文侧边栏 MVP。
- [~] Context Builder 已能读取 Agent 可见消息窗口和基础附件引用，但 RAG/Memory/Task 的深度融合仍需增强。
- [ ] Agent 主动私聊、主动拉群、主动 @ 用户、创建任务、写文件等行为的授权、频率限制和审计闭环。

### Phase 4：Agent 记忆与用户/群画像

- [x] memory-service 与 `memory_facts` 基础事实记忆表。
- [x] 用户记忆、群记忆、聊天记忆、跨会话个体记忆和 Agent 运行摘要。
- [~] conversation-intelligence-service：可创建/处理/重试会话归档任务，可消费消息/IM 事件推进活跃会话游标，并按时间窗口或消息数阈值自动归档。
- [~] 聊天记录长期 RAG：摘要/主题块进入 rag-service，候选记忆进入 memory-service pending 区；已支持规则提炼、可选 LLM 提炼器、失败重试队列、按消息/时间范围过滤和前端归档状态页，但调度指标、失败告警和更高质量的提炼评测仍需增强。
- [x] 用户可查看、编辑、删除和关闭自己的记忆。
- [x] 用户发言习惯分析默认本人可见的基础模型。
- [x] 不同 Agent、不同 Session 的记忆隔离。
- [x] 向量化记忆已有 Memory-RAG MVP

### Phase 4.5：系统设置、LLM 预设与 Skill

- [x] settings-service 独立 Kitex RPC 服务。
- [x] 用户级 LLM 预设：BaseURL、API Key、模型名、用途和默认标记。
- [x] 创建 Agent 时可选择已保存的 LLM 预设。
- [x] Prompt 模板配置，已服务翻译 Prompt，并可扩展到总结、回复候选、知识抽取。
- [x] Skill 上传文件或压缩包、摘要提取、列表、详情、前端编辑和删除。
- [x] Skill 已能作为 Agent 配置来源
- [~] API Key / MCP Secret 应用层加密存储已落地，历史明文可兼容读取

### Phase 5：RAG 与文档知识库

- [x] rag-service、文档入库、文件上传、文档列表和检索接口。
- [x] 文档解析：txt、Markdown、PDF、docx、图片 OCR、Go/代码文件和通用文本。
- [x] 分层切片：Markdown 标题、父子 chunk、代码结构切分、通用段落切分。
- [x] Hybrid Search：dense + BM25，并用 RRF 合并。
- [x] Model Rerank：检索 topN 后用模型精排 topK。
- [x] CRAG evaluator：按 relevance、coverage、specificity、conflict 输出 correct/incorrect/ambiguous。
- [x] Self-RAG：Retrieve、IsRel、IsSup、IsUse 结构化判断，由应用代码执行检索。
- [x] Adaptive RAG：规则 + LLM Router，选择 direct、project_rag、strict_rag、web_rag、memory_rag 或 tool_action。
- [x] GraphRAG MVP：实体、关系、社区、子图查询、实体 canonical key 归一化、Leiden-like 社区划分和社区摘要持久化。
- [x] Web Search Augmentation：独立 web-search-service，不入库、不建向量索引，只在一次请求内搜索、抓网页、清洗正文并返回相关段落。

### Phase 5.5：知识图谱可视化

- [x] knowledge-service 独立 Kitex RPC 服务。
- [x] api-gateway 通过 RPC 调用 knowledge-service，knowledge-service 再通过 RPC 读取 rag-service 图谱子图。
- [x] 前端知识图谱页面：搜索节点、类型过滤、关系过滤、社区过滤、拖拽缩放、节点详情、边详情和证据展示。
- [x] 节点邻域子图、两节点最短路径查询和路径高亮。
- [x] 图谱查询复用当前过滤条件，避免详情、邻域和路径绕过用户筛选。
- [~] 实体 canonical key 归一化、Leiden-like 社区划分、社区摘要持久化和图谱候选审核已有 MVP；仍需审核结果反写、严格 Leiden 库级实现和专用图数据库评估。

### Phase 5.6：结构化卡片协议

- [x] Agent Action Card JSON MVP，覆盖审批、任务分配、知识引用、错误诊断和文件学习结果基础字段。
- [x] 前端基础 Card Renderer，可把 Agent JSON 渲染为可读卡片。

### Phase 6：Tool / Skill / MCP 工作流

- [x] runtime 基础工具、web-search-service 联网搜索增强、RAG 工具、代码解释/建议/审查摘要/测试建议/文档总结和 skill-creator。
- [x] 用户可上传全局或单 Agent Skill，并在前端查看摘要和编辑内容。
- [x] MCP Gateway MVP：新增 mcp-gateway-service、Kitex RPC、工具发现、工具调用、调用审计、内置工具适配和 api-gateway 管理/调试入口。
- [x] 内置 MCP 工具：`web_search`、`search_memory`、`search_knowledge`、`query_knowledge_graph`、`summarize_conversation`。
- [x] 远程 MCP 配置：settings-service 保存用户级/全局/Agent级/会话级 MCP Server，支持 HTTP/SSE JSON-RPC、Secret 脱敏、allow/deny tools、headers、trust level。
- [x] Agent runtime 已接入 MCP Gateway，标准工具调用带 user_id、agent_id、conversation_id 做权限裁剪和审计。
- [x] 工具策略、安全中间件和审批中断已有基础
- [x] 远程 MCP 当前是 HTTP/SSE JSON-RPC MVP；stdio 只保存配置，不在服务端执行。

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
- [ ] Prometheus、Jaeger、Grafana、ELK、
- [ ] 在admin-service或者前端后台管理处添加Prometheus、Jaeger、Grafana、ELK面板连接
- [~] admin-service：独立 Kitex 服务，支持用户/群/媒体/Agent/记忆候选/知识候选/MCP/公告/成本/管理审计的全局聚合视图；细粒度管理员权限、系统消息推送、媒体审核流和成本告警仍需增强。
- [ ] K6 压测、Kubernetes、滚动升级、灰度发布和 CI/CD（暂不考虑）

### Phase 9：客户端与体验扩展

- [x] Web 前端已有聊天、好友、群、Agent、Memory、Settings、RAG、Knowledge Graph 和系统管理台页面。
- [x] Markdown 消息渲染、Agent 流式对话样式、Action Card MVP 和知识图谱交互。
- [~] Agent 上下文侧边栏、运行状态、Skill 页面和触发规则页面已有 MVP，但交互细节仍需持续打磨。
- [~] 管理端前端：系统管理台已覆盖总览、用户、群聊、媒体、Agent、知识审核、MCP、公告、成本和审计 MVP；完整管理员权限矩阵、系统消息推送和媒体审核工作流未完成。
- [~] 文件预览、图片查看和语音播放已有基础；表情库、组合搜索和更完整媒体审核未完成。

## 4. 下一阶段优先级

1. 强化 conversation-intelligence-service：补调度指标、失败告警、归档质量评测、更精确的 msg-core 时间窗口 RPC 和前端归档历史筛选。
2. 补齐 Agent 运行生产化：持久化审批、checkpoint/resume、取消、心跳、长期任务队列和运行审计。
3. 完善 IM 同步：离线推送、多端游标、Ack/重试、乱序补偿、IndexedDB 本地缓存和云端漫游策略。
4. 深化 RAG + Knowledge Graph：审核结果反写 GraphRAG、严格 Leiden / 专用图数据库评估、RAG 来源治理和 ppt/pptx 解析。
5. 推进 Tool / Skill / MCP 生产化：工具市场、权限矩阵、审批持久化、失败重试、限流配额和 stdio MCP 安全执行沙箱。

## 5. 状态口径与假设

- `[x]` 表示代码中已有明确服务、接口、数据模型或前端入口，并能形成基本闭环。
- `[~]` 表示主链路已跑通，但还有明显生产化、深度能力或体验缺口。
- `[ ]` 表示当前代码中没有可用闭环，或只是占位/规划。
- 完成度按当前仓库事实判断，不按理想生产级系统判断。
- 微服务事实以 `internal/*-service`、`cmd/*-service`、`idl/*.thrift`、`internal/api-gateway/router/router.go` 和 `docs/update/*` 为依据。
- 当前对外业务命名统一使用 Agent；历史生成代码包名属于内部实现细节，不作为产品接口规划。
