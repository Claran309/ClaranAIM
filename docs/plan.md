# ClaranAIM 统一规划文档

## 1. 核心定位

ClaranAIM 是一个面向多人在线场景的 AIM 系统：AIM = Agent + Instant Messaging。IM 是基础聊天室能力，负责用户、好友、群聊、文本/图片/文件/语音消息、历史消息、实时推送、多端同步和消息治理；Agent 是增强层，负责在真实 IM 会话上下文中理解消息、总结讨论、沉淀知识、调用工具、辅助决策并执行任务。

AIM 的长期目标不是“在聊天里加一个 AI 按钮”，而是让 Agent 成为 IM 的原生协作层。传统 IM 解决“把人连接起来”，ClaranAIM 要解决“让人和 Agent 在同一个上下文中协作”。

## 2. Agent-Native AIM 规划

### 2.1 Agent 在 IM 中的基础定位

- [ ] 会话理解者：持续理解私聊、群聊、文件、语音和上下文变化。
- [ ] 信息整理者：自动总结群聊消息，提取结论、待办、风险和关键分歧。
- [ ] 知识沉淀者：把高价值聊天内容整理成 RAG 知识库条目。
- [ ] 协作执行者：根据会话意图调用 Tool、Skill、MCP 或业务 API。
- [ ] 个体助手：理解单个用户的表达习惯、偏好、长期目标和历史行为。
- [ ] 群体助手：理解群体协作模式，发现会议低效、信息遗漏、重复争论、责任不清等问题。

### 2.2 真实会话中的 Agent 能力

- [ ] 即时总结：用户离线后回来，生成“错过了什么”。
- [ ] 日总结/周总结：按时间窗口总结项目进展、结论、阻塞项。
- [ ] 主题总结：围绕某个议题聚合跨时间消息。
- [ ] 决策总结：提取谁提出了什么方案、最终倾向是什么、还有什么未确认。
- [ ] 待办提取：把聊天中的承诺和安排转成结构化任务。
- [ ] 回复候选：根据当前上下文生成可一键选用的回复建议。
- [ ] 用户发言习惯点评：分析表达清晰度、上下文完整度、语气风险和协作盲点。
- [ ] 私密性边界：用户画像和发言点评默认只对本人可见。

### 2.3 Agent 产品形态

- [ ] Agent 作为群成员：可以被 @，也可以根据规则主动出现。
- [ ] Agent 作为个人助手：处理私密总结、提醒、改写和个人记忆。
- [ ] Agent 作为群助手：服务群空间，负责公告、总结、任务、知识库和协作质量。
- [ ] Agent 作为工作流入口：从一句聊天触发工具链，例如整理 PRD、创建任务、生成纪要。
- [ ] Agent 作为知识管家：把长期聊天沉淀为可搜索、可引用、可更新的知识资产。
- [ ] Agent 作为多角色团队：产品、研发、测试、运维、设计、CEO 等 Agent 在群中协作。

### 2.4 Tool、Skill、MCP、Memory、RAG、Policy

- [ ] Tool：提供搜索、查数据库、创建任务、读文件、发通知等具体能力。
- [ ] Skill：提供群聊总结、故障复盘、会议纪要、知识入库等场景流程。
- [ ] MCP：连接外部工具生态，让 Agent 接入第三方或本地能力。
- [ ] Memory：保存用户偏好、群聊背景、长期项目状态和历史互动模式。
- [ ] RAG：把聊天知识、文档知识、组织知识变成可检索上下文。
- [ ] Policy：控制 Agent 能看什么、能做什么、什么时候需要用户确认。

### 2.5 反思后的架构原则

`docs/consideration.md` 对当前方向的核心提醒是：Agent-Native IM 不是在 IM 旁边挂一个机器人，而是把 IM 的消息、文件、群事件、系统通知和任务流都变成 Agent 可感知的事件。因此后续演进遵循以下原则：

- Agent 优先作为事件订阅者和协作者存在，其次才是 HTTP 问答接口。
- WebSocket 推送只承载轻量通知，完整内容和离线补偿通过游标拉取完成。
- 系统消息、离线消息和多端同步都必须有 Ack、重试、顺序游标和幂等处理。
- Agent 输出优先支持结构化 JSON，再由前端渲染为 Action Card、审批卡、任务卡、知识引用卡等。
- RAG 回答必须能解释“来源于哪里、用户是否有权看、版本是什么、无法确认时是否拒答”。
- 多 Agent 协作需要独立编排边界，不把 Leader、阻塞等待、重试和验收逻辑塞进 runtime 主链路。

## 3. 统一开发阶段

### Phase 0：稳定化与验收基线


### Phase 1：高级 IM 能力补完


- [ ] 实现离线推送与上线同步，msg-history-service 维护离线游标。
- [ ] 实现多端消息同步，同一用户多设备共享已读、草稿、置顶和通知设置。
- [ ] 实现消息本地存储与云端漫游。
- [ ] 建立推送 + 拉取模型：WebSocket 只推轻量事件，客户端按消息 ID/游标拉取完整内容。
- [ ] 为离线消息补 Ack、重试、顺序确认和乱序补偿策略。
- [ ] 增强历史消息搜索，支持发送者、消息类型、附件内容、全局/当前会话范围组合过滤。
- [ ] 实现客户端本地缓存与云端漫游协作：服务端保存事实消息，客户端缓存最近会话，用户本地删除/隐藏状态独立维护。
- [ ] 实现实时审核。
- [ ] 实现实时多语言翻译。
- [ ] 文本翻译短期接入 LLM，长期支持可配置翻译 API、结果缓存和按会话语言偏好自动翻译。

### Phase 2：Agent 会话感知层

- [ ] 实现未读摘要。
- [ ] 实现“我错过了什么”会话摘要。
- [ ] 将 Agent @ 响应扩展为通用事件订阅：消息、文件、群成员变化、系统消息、任务变化均可触发 Agent 判断是否响应。
- [ ] 为 Agent 主动行为补权限边界：主动私聊、拉群、@用户、创建任务、写文件都必须有来源、授权和审计记录。
- [ ] 支持创建者邀请 Agent 入群，Agent 作为普通群成员展示，但所有主动行为附带 Agent 标记和审计来源。
- [ ] 支持 Agent 主动发起私聊或会话邀请，但必须经过创建者授权、频率限制和用户可见的操作记录。
- [ ] 完善 Agent 在线状态：区分可用、运行中、等待确认、停用和配置异常。

说明：Phase 2 已完成基础后端闭环：Agent 真实系统用户身份、agent-runtime-service、agent-manager 权限表、HTTP `/agent/*` 接口和 Kafka `message.created` @Agent 触发已接入；Agent @ 分发使用 `agent_dispatch_records` 和 msg-core-service `client_msg_id` 避免 Kafka 重投造成重复回复。工具确认当前是进程内 MVP，能跑通交互链路，但生产化需要持久化审批、checkpoint/resume 和超时清理。深度时间窗口检索、主题聚合、未读专用摘要、向量化记忆和多 Agent 协作继续放在后续阶段增强。

阶段反思：当前 Agent 仍然有较强“用户点击按钮后执行”的味道。下一步应把 `/agent/run` 视为人工入口，把 Kafka/Outbox 事件视为原生入口，让 Agent 更像 IM 内的协作者，而不是外挂问答框。

### Phase 3：Agent-Native IM 原生化重构

目标：把 Agent 从“被用户点击按钮调用的 Bot”重构为 IM 的原生成员。Agent 不应只等待 HTTP `/agent/run`，而应订阅消息、文件、群事件、系统通知、任务变化等 IM 事件，在权限允许时理解上下文、选择沉默或行动、调用工具、生成结构化结果并以真实用户身份回到会话。

核心原则：

- Message as Event：IM 每一条消息、引用、@、表情反应、文件上传、语音转写、群成员变化和系统通知都应进入统一事件模型。
- Context Awareness：Agent 处理事件前必须能看到当前会话、参与者、引用关系、历史窗口、附件、群角色、任务状态、知识库命中和权限边界。
- Agent as User：Agent 使用真实系统用户身份参与私聊和群聊，可被 @、可入群、可被加好友、可发消息，但所有主动行为必须可配置、可审计、可关闭。
- Work not Chat：Agent 的差异不在“能不能聊”，而在“能不能替用户工作”：查 RAG 并带来源、整理 Git/工单/会议纪要、安排日程、提取待办并 @负责人、识别截图报错并给解决方案。
- Event First, HTTP Second：HTTP 接口保留为人工入口和管理入口，Agent 原生入口应是 Kafka/Outbox 事件订阅与异步任务。

重构任务：


- [ ] Context Builder 继续补齐附件摘要、群成员角色、相关记忆、RAG 召回、任务状态和更细粒度权限说明。
- [ ] 文件内容理解增强：图片 OCR、PDF/Word/Markdown 解析、真实语音 ASR、RAG 入库和带来源回答仍需接入文件/语音处理服务。
- [ ] 将上下文读取明细、RAG 命中、工具调用、文件解析和 Action Card 操作继续纳入统一审计。
- [ ] 建立端到端验收脚本：文件上传触发解析、任务卡片确认、前端状态展示和真实 Kafka 重投。

需要影响的模块：

- `pkg/events`：从消息推送事件扩展为 IM 统一事件契约。
- `msg-core-service`：继续作为消息事实源，同时为 Agent 提供可见性裁剪后的上下文读取。
- `agent-manager-service`：从 Agent 管理 + @消费器升级为 Agent 订阅、路由、权限、审计和 dispatch 管理面。
- `agent-runtime-service`：从同步执行接口升级为事件上下文执行器，支持异步运行、心跳、取消、checkpoint 和结构化输出。
- `rag-service / memory-service`：承接消息、文件、总结和知识候选的沉淀。
- `websocket-gateway / frontend`：展示 Agent 原生状态、上下文侧边栏、Action Card 和审批流。

阶段边界：本阶段重点是“Agent 原生事件架构”和“上下文感知执行链路”，不是完整多 Agent 编排。多 Agent Leader、子 Agent 阻塞等待和协作验收继续放在 Phase 7。

Phase 3 MVP 说明：当前已经落地统一 `IMEventPayload`、`claran.im.events` topic、`agent_subscription_rules`、`agent_audit_records`、Agent Event Dispatcher、消息/IM 双 topic 消费、私聊默认触发、群聊 @/规则触发、静默记录和前端 Agent 原生状态/上下文侧栏。msg-core-service 已把 `file/image/voice`、编辑、撤回、已读和群成员变化纳入统一 IM 事件 outbox，Dispatcher 会把附件引用注入 Agent 上下文，并优先使用 payload `idempotency_key` 做业务幂等。Context Builder 目前基于 msg-core-service 的 Agent 可见历史窗口进行权限裁剪；RAG/Memory/Task 召回、Action Card 持久化审批、真实 ASR/OCR 和文件内容解析仍属于 Phase 5/5.6/6 的增强项。

补充进展：Agent 触发规则管理已统一使用 `/agent/route/*`，旧 `/bot/route/*` HTTP 兼容入口已移除。`agent_keyword`、`agent_command`、`agent_record` 会自动镜像到 `agent_subscription_rules`；删除 route 会同步删除镜像规则。前端“路由规则”已改为“Agent 触发规则”，面向用户展示关键词触发、命令触发和静默记录。

### Phase 4：Agent 记忆与用户/群画像

- [x] 实现基础长会话记忆：runtime 使用 session key 与 JSONL 持久化，支持跨重启恢复基本上下文。
- [x] 实现 memory-service。
- [x] 在不引入向量库的前提下先实现基础事实记忆表：用户偏好、群背景、项目状态、Agent 运行摘要。
- [x] 建立用户记忆：偏好、常用表达、长期目标、历史交互模式。
- [x] 建立群记忆：群背景、项目状态、关键成员角色、长期讨论主题。
- [x] 实现聊天记忆，覆盖私聊和群聊。
- [x] 实现跨会话用户个体记忆。
- [x] 实现用户发言习惯分析。
- [x] 将用户发言习惯分析默认设置为仅本人可见。
- [x] 支持用户查看、编辑、删除和关闭自己的记忆。
- [x] 支持向量化记忆，为跨会话召回和个性化 Agent 提供基础。
- [x] 保证不同 Agent、不同 Session 的记忆隔离，避免记忆串线。

Phase 4 MVP 已落地：`memory_facts` 保存可编辑事实记忆，范围覆盖 `user/group/conversation/session`，类型覆盖偏好、表达习惯、长期目标、群背景、项目状态、聊天摘要和 Agent 运行摘要。API 网关提供 `/memory/*` 用户治理接口，前端提供“记忆管理”；agent-manager 在调用 runtime 前按 bot/user/conversation/session 召回记忆并注入提示词，调用成功后沉淀一条私有运行摘要。向量化目前是 `vector_status/embedding_ref` 预留状态，不引入真实向量库；自动发言习惯分析是可手动维护的 MVP，后续再接异步抽取与用户确认流。

### Phase 4.5：系统设置、LLM 预设与手动翻译

- [x] 新增 settings-service，保存用户级 LLM 预设和 Prompt 模板。
- [x] 用户可预设多个 OpenAI-compatible BaseURL、API Key、模型名和用途。
- [x] 创建 Agent 时可选择已保存的 LLM 预设，由 api-gateway 解析为 Agent 配置。
- [x] 翻译功能放入 msg-core-service，用户手动点击翻译时触发。
- [x] 翻译按用户、消息、目标语言和源内容 hash 缓存，避免重复 LLM 调用。
- [x] 翻译 Prompt 可在系统设置中配置；默认目标是其他语言转中文。
- [x] settings-service 从 api-gateway 本地门面改为独立内部 HTTP 服务；后续可再升级为 Kitex RPC。
- [ ] 后续对 API Key 做加密存储或接入密钥管理服务。

边界说明：本阶段不做自动翻译。自动翻译会让每条消息都进入 LLM 调用路径，成本、隐私、延迟和限流影响都更大；当前先做用户显式触发的手动翻译。

### Phase 5：聊天知识沉淀与 RAG

- [ ] 实现 rag-service。
- [ ] 接入向量数据库。
- [ ] 实现结构感知切分：标题层级、父子 chunk、表格/代码块/列表保护。
- [ ] 实现向量粗筛 + reranker 精排 + 最终上下文组装的双阶段检索。
- [ ] 实现知识引用链：回答携带来源消息、文件、版本、时间、权限范围和可信度。
- [ ] 实现拒答策略：知识库无法确认或用户无权访问时明确拒答，而不是编造答案。
- [ ] 支持知识库创建、删除、启用和范围配置。
- [ ] 支持私有知识库、群知识库和公共知识库。
- [ ] 支持上传 pdf、md、doc、ppt 等文档。
- [ ] 从群聊中识别接口约定、业务规则、故障经验、FAQ、客户反馈等知识候选。
- [ ] 生成知识卡片：标题、摘要、来源消息、相关人员、标签、可信度。
- [ ] 增加人工审核队列，支持群主、管理员或知识负责人确认入库。
- [ ] 支持知识过期提醒、冲突检测和相似知识合并。
- [ ] 支持 RAG 回流会话，在相关讨论中主动引用已有知识。
- [ ] 支持文件即知识入口：PDF、Word、Markdown、图片 OCR、语音转写等进入统一解析和入库流水线。
- [ ] 多模态内容入库前必须经过权限和审核校验，Agent 只能使用当前用户有权访问且审核通过的内容。

### Phase 5.5：Agent + 知识图谱

- [ ] 先用 MySQL 实现轻量知识图谱事实表和边表，验证产品链路。
- [ ] 抽取实体：用户、群、Agent、文件、话题、任务、决策、风险、知识卡片。
- [ ] 抽取关系：提及、讨论、负责、依赖、确认、反对、来源于、属于群、由 Agent 生成。
- [ ] 将 Agent 总结、insights、RAG 知识卡片转成可审核的图谱候选。
- [ ] 为每条图谱事实保留来源消息、创建者、可信度、可见范围和更新时间。
- [ ] 前端提供“关系视图”基础展示：某个话题相关人员、结论、文件、任务和风险。
- [ ] 后续按规模评估 Neo4j、NebulaGraph 或 TuGraph；在关系复杂度没有压垮 MySQL 前不强行引入图数据库。

合理性评估：知识图谱很适合 AIM，因为 IM 天然产生人、群、消息、文件、任务、结论之间的关系。它不应该替代 RAG，而是补足“谁和谁、什么依赖什么、结论从哪里来”的结构化关系。影响模块主要包括 agent-runtime-service 的结构化抽取、rag/memory 服务的数据沉淀、msg-core-service 的消息来源引用、api-gateway 的查询接口和前端的关系展示。建议先做轻量 MySQL MVP，再根据查询复杂度迁移专用图数据库。

### Phase 5.6：结构化卡片协议

- [x] 定义 Agent Action Card JSON MVP，覆盖审批、任务分配、知识引用、错误诊断和文件学习结果的基础字段。
- [x] 前端实现基础 Card Renderer，把后端结构化 JSON 渲染为可读卡片；卡片操作暂为本地提示。
- [ ] 增加卡片版本号和兼容策略，旧客户端无法识别时退化为纯文本摘要。
- [ ] 卡片操作必须带 action_id、幂等键、权限上下文和审计信息。
- [ ] 将现有工具审批卡从前端定制逻辑逐步迁移为通用卡片协议。

合理性评估：Agent 不应该只输出纯文本。审批、任务、日程、知识引用都天然是结构化交互。卡片协议能减少前后端为每个场景重复开发 UI，但必须控制版本兼容和权限校验，否则会变成难维护的半成品协议。

### Phase 6：Tool / Skill / MCP 工作流

- [ ] 为 agent-runtime-service 接入 Tool 调用。
- [ ] 为 agent-runtime-service 接入 Skill 场景能力。
- [ ] 为 agent-runtime-service 接入 MCP 工具生态。
- [ ] 新增 MCP Gateway，统一承接文件解析、RAG、日历、工单、Git、外部 API 等工具调用。
- [ ] MCP Gateway 为每次工具调用记录调用方、参数摘要、权限上下文、耗时、结果状态和审计 ID。
- [ ] 支持天气查询、代码执行、Web 搜索、数据库查询、业务系统调用等工具能力。
- [ ] 实现工具注册、工具权限和工具调用审计。
- [ ] 实现 Agent 异步任务和长任务进度。
- [ ] 实现工具调用失败重试。
- [ ] 为高风险动作增加二次确认。
- [ ] 支持从聊天生成会议纪要。
- [ ] 支持从聊天创建任务。
- [ ] 支持从聊天整理 PRD。
- [ ] 支持从聊天总结 Issue、PR 和发布记录。
- [ ] 支持从聊天生成日报和周报。
- [ ] 将 Agent 行为写入审计日志。

### Phase 7：多 Agent 群聊协作

- [ ] 新增 Agent Cooperation Service，负责 Agent Leader、子任务、阻塞等待、结果汇总和验收。
- [ ] 支持产品 Agent、研发 Agent、测试 Agent、运维 Agent、设计 Agent、CEO Agent 等角色型 Agent。
- [ ] 支持不同 Agent 配置不同 Prompt、Skill、工具权限和能力边界。
- [ ] 支持通过 @ 召集一个或多个 Agent。
- [ ] 支持通过命令召集一个或多个 Agent。
- [ ] 支持通过规则自动召集一个或多个 Agent。
- [ ] 支持多 Agent 路由，按群、关键词、意图或用户指定进行分发。
- [ ] 支持 Agent 之间任务交接。
- [ ] 支持 Agent 之间共享上下文。
- [ ] 支持多 Agent 结果合并。
- [ ] 支持多 Agent 协作的超时、取消、重试、人工接管和审计。
- [ ] 支持 Agent 作为群成员参与讨论。
- [ ] 支持个人私密 Agent 只服务本人。
- [ ] 支持“召集团队”式群聊智能协作。

多agent协作：

agent leader

agent输出可@他人

于是可以

用户表明要求给agentleader，agentleader@多agent，并验收成果

子agent可@单个或多个子agent，得到回复前阻塞
根据被@时@的主人更改prompt

### Phase 8：治理、观测与生产化

- [x] 引入 Kafka 事件流一期：承载群组成员事件与消息实时推送事件。
- [ ] 扩展 Kafka 事件流二期：承载文件事件、Agent 事件、知识入库事件、搜索索引事件和审计事件。
- [ ] 为所有 Kafka consumer 引入统一 processed-events 幂等表和消费延迟监控。
- [ ] 为 Agent 事件增加行为审计 topic：agent.invoked、agent.completed、agent.failed、agent.tool_called、agent.card_action。
- [ ] 引入 Nacos 或等价配置中心。
- [x] 完善服务超时控制：普通 Kitex RPC 默认启用固定 timeout；Agent 长运行链路单独使用 `governance.agent_rpc`，`timeout_ms: 0` 表示不设置固定客户端 deadline，后续由异步任务、心跳、取消和审计判断任务是否死亡。
- [ ] 完善服务降级。
- [ ] 完善服务重试。
- [x] 完善服务熔断：Kitex 客户端默认启用 circuit breaker。
- [x] 完善限流和成本控制：API Gateway 已接入用户/IP 令牌桶限流，Agent 计费已严格按模型 usage token 记录；生产多实例限流仍需 Redis/网关层共享实现。
- [x] 接入并默认开启 DTM 分布式事务基础设施：Docker Compose 提供 dtm-server，`pkg/config` 默认启用 DTM，`pkg/dtm` 提供 Saga 构建器；已用于创建群组时协调 group-service 创建群元数据和 msg-core-service 创建群聊会话，当前不改造高频消息链路。
- [ ] 引入 Prometheus 指标采集。
- [ ] 引入 Jaeger 链路追踪。
- [ ] 引入 Grafana 可视化。
- [ ] 引入 ELK 日志聚合。
- [ ] 完善 Zap 结构化日志。
- [ ] 监控在线人数、消息吞吐量、Agent 响应延迟、Agent 工具调用耗时和错误率。
- [ ] 使用 K6 压测登录、会话列表、消息收发、群聊消息、历史查询、文件上传、Agent 对话和 Agent 总结。
- [ ] 推进 Kubernetes 部署。
- [ ] 支持滚动升级、灰度发布和环境隔离。
- [ ] 推进 CI/CD 流水线。
- [ ] 建立 Agent 权限治理。
- [ ] 建立敏感信息识别与脱敏机制。
- [ ] 建立 Agent 行为审计机制。
- [ ] 建立记忆治理机制。
- [ ] 建立知识库生命周期管理机制。
- [ ] 建立管理层后台：用户、群、系统消息、媒体审核、Agent 审计、知识入库审核和成本监控统一入口。
- [ ] 媒体审核队列覆盖图片、视频、文件和语音转写，管理端可通过、驳回、标记敏感或要求人工复核。
- [ ] 系统消息管理支持单份系统消息正文、投递范围、有效期、撤回和用户已读游标查询。
- [ ] 成本监控按用户、群、Agent、模型、工具、RAG 检索和多 Agent 协作维度聚合。

### Phase 9：客户端与体验扩展

- [ ] 优化 Web 前端 UI，使聊天、群管理、Bot、知识库、工具审批等工作流更清晰。
- [ ] 支持 Markdown 消息渲染。
- [ ] 支持 Agent 流式输出。
- [ ] 支持结构化 Action Card 展示与操作。
- [ ] 支持 Agent 上下文侧边栏：展示当前会话摘要、相关知识、任务、风险、引用来源和可执行动作。
- [ ] 支持表情包能力：图片快速保存为表情，建立用户私有表情库、缩略图和快速发送入口。
- [ ] 强化前端搜索体验：时间范围、发送者、消息类型、附件内容和当前会话/全局范围组合筛选。
- [ ] 管理端前端覆盖媒体审核、系统消息、Agent 审计、知识候选审核和成本面板。
- [ ] 提供 CLI/TUI 客户端。
- [ ] 评估移动端客户端或 PWA。
- [ ] 优化前端消息提示、会话切换、搜索、文件预览、图片查看和语音播放体验。

## 4. 架构与性能演进

### 4.1 服务拆分演进

- [ ] 保持 HTTP 网关与内部 RPC 服务分层，浏览器不直接访问内部微服务。
- [x] 保持 WebSocket 网关与 msg-core-service 解耦，消息核心同事务写 `message.*` Outbox 事件，Outbox worker 发布 Kafka，WebSocket 网关消费后推送；HTTP `/push` 仅作为兼容入口。
- [ ] 将历史、归档、冷存储、离线同步逻辑逐步从 msg-core-service 拆到 msg-history-service。
- [ ] 未来 AI 服务可按需要使用 Python 或独立运行时，但通过 RPC/API 与 Go 主系统解耦。
- [ ] Agent 路由未来可扩展为 API Gateway 级别的路由分发。

### 4.2 消息与推送性能

- [x] 消息存储保持 MySQL 作为服务端事实来源，`group.*` 与 `message.*` 事件通过 Transactional Outbox/Kafka 做写后处理。只有在 Redis Stream + AOF + 消费 ACK + 重放补偿完备时，才考虑 Redis 缓冲写入，不能把普通 Redis 缓存当消息 WAL。
- [x] 消息推送从 HTTP 调用 `/push` 演进为 MySQL Outbox + Kafka 事件总线，`/push` 保留为兼容入口。
- [ ] 会话列表从每次查 DB 拼装演进为 Redis Sorted Set 维护。
- [ ] 在线状态从 Redis String + TTL 演进为 Redis Set + 定时刷新。
- [x] 历史消息分页使用游标分页 `before_id`。
- [ ] 批量用户查询从逐个查缓存演进为 Redis Pipeline 批量查询。

### 4.3 数据库与迁移

- [x] 启动阶段改为非破坏性 AutoMigrate，禁止 DropTable 清空开发数据。
- [ ] 引入 golang-migrate 或等价迁移工具，覆盖字段删除、类型变更和数据回填。
- [ ] 为消息编辑、撤回、引用、已读游标、离线同步、知识库、记忆和审计补充增量迁移策略。

### 4.4 计费与成本

- [x] 当前 Agent 计费使用 Eino `ResponseMeta.Usage` 中的真实 Token 用量，不再按字符数估算。
- [ ] 扩展更多模型供应商的 usage 字段兼容映射，并对缺失 usage 的模型接入做告警和配置拦截。
- [ ] 为 Agent 工具调用、RAG 检索、长任务、多 Agent 协作建立成本记录。
- [ ] 为群空间、用户、Bot 增加配额、限流和成本告警。

## 5. API 与服务能力规划

### 5.1 已实现服务端口

| 服务 | 端口 | 状态 |
|------|------|------|
| user-service | 9001 | 已完成，支持 Agent 系统用户标记 |
| group-service | 9002 | 已完成 |
| msg-core-service | 9003 | 已完成 |
| msg-history-service | 9004 | 已完成 |
| file-service | 9005 | 已完成 |
| agent-manager-service | 9006 | 已完成 |
| api-gateway | 8080 | 已完成 |
| websocket-gateway | 8081 | 已完成 |

### 5.2 待新增或增强服务

- [x] agent-runtime-service：Agent 执行面，负责基础上下文、工具调用、JSONL 长会话和结构化理解；长期任务、流式事件、多 Agent 协作待增强。
- [ ] rag-service：知识库、文档上传、向量检索、聊天知识入库。
- [ ] memory-service：用户记忆、群记忆、项目状态和向量化记忆。
- [ ] msg-filter-service：敏感内容检测、实时审核、多语言翻译。
- [x] msg-core-service Kafka consumer：消费 `group.*` 事件，同步群聊 conversation 和参与者。
- [x] websocket-gateway Kafka consumer：消费 `message.*` 事件，推送在线 WebSocket 连接。
- [ ] event-service 或更多 Kafka consumer：处理消息后处理、搜索索引、AI 摘要、知识候选和审计事件。
- [ ] notification-service：处理离线推送、上线同步和跨端通知。
- [x] Outbox worker 一期：group-service 和 msg-core-service 内置 worker，扫描 MySQL `event_outbox` 表，将业务事务内写入的待发布事件可靠投递到 Kafka。
- [ ] 通用 processed-events 表：为所有 Kafka 消费者提供统一幂等去重；当前 Agent @ 分发链路已先使用专用 `agent_dispatch_records` 和消息 `client_msg_id` 做幂等。

## 6. 安全、权限与治理原则

- [ ] Agent 必须基于真实 IM 上下文工作，而不是孤立问答。
- [ ] Agent 的主动性必须可配置、可审计、可关闭。
- [ ] 知识入库必须有人类确认或可回滚机制。
- [ ] 用户画像和发言点评必须默认私密，不能变成组织监控。
- [ ] 工具调用必须有权限边界，高风险动作必须确认。
- [ ] 会话可见性、文件可见性、知识库范围、记忆范围、工具调用权限必须统一治理。
- [ ] 敏感信息识别、脱敏、审计日志和管理员策略必须覆盖 Agent 行为。
- [ ] 用户必须能查看、编辑、删除自己的记忆。
- [ ] 内部 Bot 的 API Key 必须隐藏，自部署 Bot 的密钥只允许授权用户管理。

## 7. 当前待办与验收重点

- [ ] 继续自查历史消息、群权限、多端同步等边界问题。
- [ ] 验证不同会话、不同 Agent、不同 Session 之间消息和记忆不串线。
- [ ] 验证文件上传、元数据保存、MinIO/本地存储和下载链路稳定。
- [ ] 验证语音消息按住录音、松手发送、会话内播放体验稳定。
- [ ] 验证群成员权限、禁言、管理员、群主转让和群公告。
- [ ] 验证前端会话列表、未读红点、历史消息、图片预览、文件下载、语音播放。
- [ ] 为核心 IM、Agent、RAG、Memory、Tool 调用建立分层测试与验收脚本。




