# ClaranAIM 压力面完整复盘：2026-07-04

> 这版按完整线程原始编号整理，不再按后半段上下文重构。
>
> 结构：每题包含“我的问题 / 你的回答 / 推荐答案 / 注意点”。对于你没答或答“不知道”的题，保留原状态，方便明天优先补。

## 0. 背景与训练规则

### 原始场景

你说明这次答辩会像大厂压力面：面试官会先细看项目，然后围绕项目实现、链路、技术栈、问题和不足追问。项目是 vibe coding 做出来的，所以你担心很多知识不具备。

我给你的训练原则是：

- 不按八股背知识点，而是按“项目点 -> 背后机制 -> 源码入口 -> 失败场景 -> 不足与改进”准备。
- 每题你先答，我指出漏洞，再给推荐答案。
- 答不上来就看源码。

### 准备文档

你要求先准备一份明天可继续学习的文档，覆盖 Outbox、DTM、Redis、限流、降级、Agent runtime、Milvus、Neo4j、RAG、GraphRAG、管理层、配置层、高级 CRUD、坑点和亮点。

产出文件：

- [defense-prep-start-here.md](D:/CodeStudy/GoProjects/src/ClaranAIM/docs/learn/defense-prep-start-here.md)

## 1. 开场与 Agent 原生

### 第 1 题：项目总览

**我的问题**

请用 2 分钟介绍 ClaranAIM：

- 项目是什么？
- 和普通 IM + AI 聊天助手有什么区别？
- 核心技术链路是什么？
- 最大亮点和最大不足是什么？

**你的回答**

> ClaranAIM 是一个将 IM 系统与 Agent 产业紧密结合的项目。Agent 能作为真实用户存在于 IM 的用户与会话体系中，可以被拉入群聊或私聊。AIM 还提供向量库记忆召回、知识图谱、私有知识库与 RAG 等能力。
>
> 核心技术链路是发送消息 -> CRUD & Outbox -> 发送事件 -> Agent 层监听事件并判断是否触发 Agent 行为。
>
> 最大亮点是 Agent 用户化原生，可让 Agent 在每个消息后隐秘思考；不足是很多功能和 Agent 进阶功能都是 MVP Demo。

**推荐答案**

> ClaranAIM 是一个 Agent-Native IM 系统。它不是在聊天软件旁边加一个 AI 问答按钮，而是把 Agent 作为 IM 系统里的原生成员来设计。Agent 有真实系统用户身份，可以进入私聊和群聊，可以被 @，可以读取自己有权限看到的会话上下文，也可以调用 Memory、RAG、Skill、MCP 等能力完成任务。
>
> 技术链路上，普通消息先由 `msg-core-service` 写入 `messages`、`message_user_states`，并在同一个 MySQL 事务里写入 `event_outbox`。Outbox Worker 再发布 Kafka 事件，驱动 WebSocket 推送、Agent Dispatcher、conversation-intelligence 等消费者。Agent 被触发后由 manager 调 runtime，生成回复后以 `agent_user_id` 作为 sender 写回 msg-core-service。
>
> 亮点是把 IM 消息事实、可靠事件、Agent 身份、权限审计和 RAG/Memory 串成闭环；不足是 Agent runtime、上下文管理、工具审批、多 Agent 协作、RAG/GraphRAG 质量和生产治理仍偏 MVP。

**注意点**

- 不要说“隐秘思考”，改成“事件订阅、静默记录、审计可追踪”。
- 不要说“CRUD & outbox”，要说清楚消息事实表、用户状态表、事务 Outbox、Kafka、WebSocket、Agent Dispatcher。

### 追问 1.1：为什么 Agent 必须是真实系统用户？

**我的问题**

为什么 Agent 一定要作为真实系统用户存在？为什么不直接在消息表加 `sender_type = agent`？

**你的回答**

> 这涉及 Agent 原生。如果只是聊天机器人就同质化、毫无新意。本项目让 Agent 原生存在于 IM 中。Agent 作为真实 user 可以复用 User 模块链路，也可以将 User 模块功能打包为 MCP 供 Agent 调用，便于统一审计消息事实和业务日志。只用 `sender_type=agent` 不符合 Agent 原生设计哲学。

**推荐答案**

> Agent 作为真实系统用户，核心是复用 IM 的身份和权限模型。IM 的会话参与者、群成员、好友、消息 sender、已读状态、本地删除、审计和权限校验都围绕 `user_id` 建模。如果 Agent 只是 `sender_type=agent`，它没有自然的用户 ID，不能进入 `conversation_participants`，也很难复用已有权限、消息投递、审计和管理能力。
>
> 项目创建 Agent 时会通过 user-service 创建 `is_system=true` 的系统用户，并把返回的 ID 保存为 `AgentUserID`。之后 Agent 被 @、参与私聊/群聊、发送回复、读取上下文，都能复用 IM 原生链路。

**注意点**

- 不要说“为了新意”，说“为了身份、权限、参与者、上下文、审计统一”。
- “User 模块打包 MCP”不要当成现状，除非能指出实现。

### 追问 1.2：Agent 创建链路

**我的问题**

Agent 创建时具体发生什么？从 API 请求进入开始，讲到 Agent 系统用户创建、Agent 配置落库、权限初始化。

**你的回答**

> 不知道。

**推荐答案**

> 创建入口是 API Gateway 的 `/api/v1/agent/create`。网关绑定参数，并从 JWT 取当前用户作为 owner，而不是信任前端传 owner。如果传了 `llm_profile_id`，会调用 settings-service 解析 LLM 配置。然后网关构造 `CreateBotReq`，通过 Kitex 调 agent-manager-service。
>
> agent-manager 校验 Agent 类型和模型配置。`internal` Agent 可使用平台默认模型；`custom` Agent 需要 API Key、BaseURL、模型名。随后 manager 调 user-service 注册 `is_system=true` 的系统用户，返回的用户 ID 保存为 `AgentUserID`。
>
> 最后 manager 写 `bots` 表，保存模型配置、system prompt、skills dir、workspace root、tool policy、owner id、AgentUserID 等，并写 `bot_permissions` 给创建者 owner 权限。

**注意点**

- `AgentUserID` 是 Agent 原生链路的核心桥。
- 表名叫 `bots` 是历史遗留，业务语义已经是 Agent。

### 追问 1.3：系统用户成功但 bots 写失败

**我的问题**

agent-manager 先调用 user-service 创建系统用户，再写 `bots` 表。如果系统用户创建成功，但 `bots` 写入失败，会发生什么？怎么改？

**你的回答**

> 会生成一个永远不会被调用的幽灵 user。这确实是不足。改进方法：1. 创建系统用户时同步写 bot 配置并标记是否存在 bot 表对应字段，然后 worker 扫描重试；2. 用 DTM 做分布式一致性事务，把写 user、写 bot 封装为 Saga；3. 直接回退产生。

**推荐答案**

> 会留下一个系统用户已创建、但没有任何 Agent 配置绑定的孤儿系统账号。它一般不会被正常触发，因为没有 `bots.AgentUserID` 配置，但会污染用户表、影响管理台和审计。
>
> 改进可以有三类：第一，失败后立即补偿，agent-manager 调 user-service 的幂等删除/禁用系统用户接口。第二，创建流程状态化，先写 `bots` 为 `creating`，创建系统用户成功后回填 `agent_user_id` 并置为 active，失败则标记 failed。第三，用 DTM Saga，把创建系统用户和创建 Agent 配置作为两个分支，失败时补偿。

**注意点**

- 不要让 user-service 感知 agent-manager 的 `bots` 表，否则服务边界反了。
- 这是低频跨服务一致性问题，适合补偿/Saga。

### 追问 1.4：为什么表和代码还叫 bot？

**我的问题**

为什么对外叫 Agent，但源码和表里还有 `bot`、`bots`、`bot_permissions`、`kitex_gen/bot`？怎么解释？有什么风险？

**你的回答**

> 早期只是想做 IM + Bot 聊天平台，后期转为 Agent 原生 IM，但早期 AIBOT 已经落库落表，所以后续 Agent 语义字段都有 bot 的身影。归根原因是项目规划不足，会带来语义歧义的工程风险。

**推荐答案**

> 这是项目演进留下的历史命名问题。早期更接近 IM + Bot，所以 IDL、生成代码和数据库表使用了 `bot` 命名。后续定位升级为 Agent-Native IM，但直接全量重命名会涉及数据库迁移、IDL 重生成、RPC 改动和历史数据兼容风险，所以当前采用渐进迁移：对外产品语义和 HTTP 路由统一叫 Agent，内部部分表名和生成代码暂时保留 bot。
>
> 风险是新人理解成本高、代码搜索和维护成本上升、审计/权限/监控命名不一致。后续应通过分阶段 migration 和兼容层统一领域命名。

**注意点**

- 不要只说“规划不足”，要说“历史兼容与渐进迁移”。

### 追问 1.5：普通文本消息完整链路

**我的问题**

从 `/message/send` 开始，讲清普通文本消息经历哪些步骤：网关、RPC、msg-core-service、数据库表、Outbox、Kafka、WebSocket、Agent Dispatcher。

**你的回答**

> 前端获取消息信息和元数据并包装调用 `/message/send`。API 网关校验后转为 RPC 调消息发送接口。msg-core-service 校验后将消息落库，同步写 outbox 表，更新会话参与者每个用户的本地消息视图，清除缓存，发布 msgcreated 事件和 Claran.im event。第一个被 WS 监听推送，第二个被 Agent 监听，Agent Dispatcher 判断过滤、静默或触发，触发后进行后续任务。

**推荐答案**

> 前端调用 API Gateway 的 `/api/v1/message/send`。网关做 JWT 鉴权、参数绑定，再转 Kitex RPC 到 `msg-core-service.SendMessage`。
>
> `msg-core-service` 的核心逻辑在 `SendMessageExt`：校验会话、发送者是否为参与者、群状态/禁言、引用消息等。通过后在一个数据库事务里写 `messages`，更新 `conversations.updated_at`，为参与者写或 upsert `message_user_states`，并写入 `event_outbox(message.created)`。
>
> 事务提交后，Outbox Worker 发布 Kafka topic `claran.message.events`。WebSocket Gateway 消费消息事件并推给在线用户。Agent Manager 也消费消息事件，由 Agent Dispatcher 判断私聊默认触发、群聊 @ 或订阅规则触发，触发后调用 runtime，最后 Agent 以 `AgentUserID` 写回 msg-core-service。
>
> 文件、语音、编辑、撤回、已读、群成员变化等会进入更广义的 `claran.im.events`，供 Agent-Native 事件流使用。

**注意点**

- 普通文本主要是 `message.created` -> `claran.message.events`。
- `message_user_states` 是用户级消息状态，不是“更新参与者”。
- 静默/record 不等于一定进入记忆候选。

## 2. 消息队列、Kafka 与事件契约

### 消息队列基础补课

**你的问题**

你突然对消息队列基础内容迷糊：事件类型、topic、管道、消息本身能否承载信息。问：什么叫事件类型是 `message.created`，topic 是 `claran.message.events`？和 `claran.im.events` 有什么区别？

**推荐答案**

> Kafka topic 是消息放在哪条事件流里；`Envelope.Type` 是这条事件表达的业务动作；`Envelope.Payload` 是具体业务数据。
>
> `message.created` 是事件类型，表示“一条消息事实已经创建并落库”。`claran.message.events` 是 Kafka topic，承载消息领域事件，如 `message.created`、`message.edited`、`message.recalled`、`message.read`。
>
> `claran.im.events` 是 Agent-Native 统一 IM 事件流，不只承载消息，还承载文件上传、语音转写、表情、群成员变化、系统通知、任务变化等更广义的 IM 事件，并携带 Agent 决策需要的上下文、权限和幂等信息。

**注意点**

- topic 决定“去哪条流”。
- type 决定“发生了什么”。
- payload 决定“这件事的具体数据是什么”。

### 追问 2.1：为什么必须通过 Outbox？

**我的问题**

为什么不能在 `SendMessageExt` 写完 `messages` 后，直接调用 Kafka 或 WebSocket 推送？为什么一定要通过 Outbox？画出直接推送的故障窗口。

**你的回答**

> 方案 A：直接写 MySQL -> 发 Kafka/WebSocket。如果写 MySQL 后 Kafka 挂了，就丢失该消息链路，造成数据库和业务事实不一致。
>
> 方案 B：MySQL 事务里同时写 messages 和 event_outbox。Outbox 和业务事实一起落库，后续由 Outbox Worker 扫描并分发 Kafka，即使 Kafka 挂了，Outbox 记录还在，可以重试。

**推荐答案**

> MySQL 和 Kafka/WebSocket 不是同一个事务资源。写完 `messages` 后直接发 Kafka，如果 DB 提交成功但进程在发 Kafka 前崩溃，或者 Kafka 暂时不可用，就会出现“消息已经落库，但事件副作用没有发生”：历史消息能查到，但 WebSocket 没推送、Agent 没触发、异步后处理不知道。
>
> 反过来先发 Kafka 再提交 MySQL 也有问题：消费者可能收到事件，但数据库事务后来回滚，下游看到不存在的消息事实。
>
> Outbox 把 `messages`、`message_user_states`、`event_outbox(message.created)` 放在同一个 MySQL 事务。事务成功则消息事实和待发布事件同时存在；事务失败则一起回滚。Kafka 不可用时 Worker 后续重试。

**注意点**

- Outbox 解决“不丢事件”，不解决“只处理一次”。

### 追问 2.2：发布成功但 MarkPublished 前崩溃

**我的问题**

Outbox Worker 发布 Kafka 成功后，还没把记录标记为 `published` 就崩溃。重启后会怎样？会不会重复？靠什么解决？

**你的回答**

> 不知道。

**推荐答案**

> 会重复发布。Worker 顺序是 `FetchDue -> Publish -> MarkPublished`。如果 Kafka 已发布成功但 `MarkPublished` 前崩溃，MySQL 中 outbox 仍是 `pending/retrying`，重启后会再次被扫描发布。
>
> 因此 Outbox 是 at-least-once，不是 exactly-once。重复靠消费者幂等解决。例如 Agent 链路用 `agent_dispatch_records(event_id, agent_user_id)`，Agent 回复再用稳定的 `client_msg_id = agent:{sourceEventID}:{agentUserID}` 兜底。

**注意点**

- 记住短句：Outbox 防丢，不防重。

### 追问 2.3：locked_until 的作用

**我的问题**

`locked_until` 是干什么的？既然有 `pending/retrying`，为什么还要设置 `locked_until`？能彻底防重复发布吗？

**你的回答**

> 不知道，应该是防并发状态下的数据竞争？

**推荐答案**

> `locked_until` 是 Outbox Worker 的租约锁。`pending/retrying` 只表示事件需要被发布，不表示“正在被某个 Worker 处理”。多个 Worker 并发扫表时，如果没有锁，可能同时拿到同一条记录并重复发 Kafka。
>
> `FetchDue` 只取未锁定或锁已过期的事件，并把 `locked_until` 设置到未来一段时间。这样大多数情况下多个 Worker 不会处理同一条记录。
>
> 但它不能彻底防重复：发布成功后 MarkPublished 前崩溃、处理超过 lockFor、DB 锁和 Kafka 发布不是同一事务，都仍可能重复。

**注意点**

- `locked_until` 是并发租约，不是 exactly-once。

### 追问 2.4：消费者幂等怎么做？

**我的问题**

WebSocket 推送消费者、Agent 触发消费者、未来搜索索引消费者分别怎么做幂等？哪些能接受重复，哪些绝对不能重复？

**你的回答**

> 我只知道 Agent 部分有 agent_client_id 做幂等，eventid + agentid 的幂等表；IM 里也有类似幂等。我不清楚项目实际上怎么做幂等。

**推荐答案**

> 不是 `agent_client_id`，而是 `agent_dispatch_records(event_id, agent_user_id)`。此外项目还有通用 `processed_events` 和 `NewReliableHandler` 框架。
>
> WebSocket 重复主要是体验问题，可以靠 `processed_events` 和前端 `msg_id/client_msg_id` 去重。
>
> Agent 重复是高风险，会导致重复 LLM 调用、重复回复、工具重复执行，所以用 `agent_dispatch_records(event_id, agent_user_id)` 防重复触发；回复写回再用稳定 `client_msg_id` 防重复消息。
>
> 搜索/RAG 索引消费者应使用 `processed_events` + 业务 upsert，如 `msg_id/document_id/chunk_id` 唯一键，避免重复索引污染召回。

**注意点**

- Agent 重复最危险：成本、工具副作用、重复回复。

### 追问 2.5：为什么 handler 成功后才提交 offset？

**我的问题**

Kafka consumer 为什么 handler 成功后才提交 offset？如果先提交再执行 handler，会怎样？如果 handler 成功但提交 offset 失败，会怎样？

**你的回答**

> handler 成功后才提交 offset 是为了保证消息先被消费才被标记为抛弃，做到 at-least-once 而不是丢失。先提交 offset 再执行 handler，中途挂掉会消息丢失。如果 handler 成功但提交 offset 失败，会重复，所以要下游幂等。

**推荐答案**

> offset 不是“抛弃标记”，而是消费进度。项目逻辑是 `FetchMessage -> DecodeEnvelope -> handler -> CommitMessages`。handler 成功后提交 offset，保证事件至少被业务处理一次。若先提交 offset 后 handler 崩溃，Kafka 会认为该 group 已处理，重启不会再投递，导致业务事件丢失。
>
> 如果 handler 成功但 offset 提交失败，业务副作用已经发生，但 Kafka 还会重投，所以消费者必须幂等。这就是 at-least-once 的典型代价。

**注意点**

- DecodeEnvelope 失败项目会 commit，因为脏消息重试也无法恢复。

### 追问 2.6：Kafka message key 为什么用 conversation_id？

**我的问题**

Kafka 消息使用 `envelope.Key` 作为 key。为什么设置 key？`message.created` 为什么通常用 `conversation_id`？和 partition、顺序性有什么关系？

**你的回答**

> bzd。

**推荐答案**

> Kafka 只能保证单个 partition 内有序，不保证全局有序。producer 会根据 key hash 选择 partition。对 IM 来说最重要的是同一会话内消息顺序，而不是全系统全局顺序，所以 `message.created` 用 `conversation_id` 作为 key，让同一会话事件尽量落到同一个 partition。

**注意点**

- key 不是幂等键，也不是唯一 ID；主要服务分区和局部顺序。

### 追问 2.7：热门 conversation 的热点分区问题

**我的问题**

某个 conversation 特别热门，所有消息都用同一个 `conversation_id` 做 key，会不会有问题？怎么优化？如果不用它做 key，又牺牲什么？

**你的回答**

> 不清楚。所有消息用同一个 conversation_id 做 key，消息量过大时可能出现小范围顺序错误，可以根据消息更新时间做排序检验？

**推荐答案**

> 真正问题不是“小范围顺序错误”，而是热点分区：同一个 key 会 hash 到同一个 partition，超热门群聊会导致单 partition 和单 consumer 成为瓶颈，consumer lag 增加，实时推送和 Agent 触发延迟。
>
> 如果改成随机 key 或 message_id，可以提升吞吐，但会牺牲会话内顺序。下游需要 sequence/version、客户端重排、幂等状态机处理乱序。当前 IM 场景默认保留 `conversation_id` 是合理的，但超大群可用更多 partition、大群独立 topic、conversation_id+bucket、服务端 sequence 等优化。

**注意点**

- 顺序和吞吐是 trade-off。

### 追问 2.8：created/edited/recalled 使用同一 key

**我的问题**

`message.created`、`message.edited`、`message.recalled` 都发到 `claran.message.events`，并用 `conversation_id` 做 key，有什么好处？如果 edited/recalled 用 `message_id` 做 key，会有什么风险？

**你的回答**

> 我不知道，只会部分：好处是顺序严格处理，坏处是性能积压。如果 edited/recalled 用 message_id 做 key，会导致顺序乱序，需要下游校验重排。

**推荐答案**

> 这些事件属于同一会话时间线。用 `conversation_id` 做 key，可以让同一会话的创建、编辑、撤回等事件落在同一个 partition，获得 partition 内顺序保证。WebSocket、Agent、索引消费者更容易按正确顺序处理。
>
> 如果 created 用 conversation_id，而 edited/recalled 用 message_id，它们可能进入不同 partition。消费者可能先收到撤回/编辑，再收到创建，导致客户端、索引、Agent 上下文错乱。除非下游有完善 sequence/version 重排机制，否则同一会话时间线最好用一致 key。

**注意点**

- 这不是端到端绝对顺序，仍需 msg_id/version 兜底。

### 追问 2.9：Envelope.Type 与 payload Type/EventType

**我的问题**

`Envelope.Type`、`MessagePayload.Type`、`IMEventPayload.EventType` 为什么容易混？怎么解释语义边界？

**你的回答**

> Envelope.Type 代表消息本身类别，程序依据它路由到不同 topic，是消息本身标识。payload 里的 type 标识业务语义中消息本身是普通消息还是文件上传等特殊消息，属于业务语义。

**推荐答案**

> `Envelope.Type` 是事件总线层的事件类型，表示发生了什么领域事件，并决定 topic 和消费者分发，如 `message.created`、`file.uploaded`。
>
> `MessagePayload.Type` 更偏 WebSocket 前端协议里的推送类型，告诉前端怎么渲染。
>
> `IMEventPayload.EventType` 是 Agent-Native 统一 IM 事件载荷中的业务事件类型，给 Agent Dispatcher 做业务决策。
>
> 简单说：Envelope 的 type 管事件路由，payload 里的 type/event_type 管业务消费语义。

**注意点**

- `Envelope.Type` 不要说“消息本身类别”，要说“事件类型”。

### 追问 2.10：新增事件忘记 Topic 映射

**我的问题**

如果新增 `message.pinned`，只定义常量但忘记维护 `Envelope.Topic()` 映射，会发生什么？怎么测试或防止？

**你的回答**

> 该消息不会被下游捕获，导致消息链路断掉，不知道怎么防止。

**推荐答案**

> `Envelope.Topic()` 返回空字符串，KafkaPublisher 会报 `event topic is empty`。Outbox Worker 发布失败，记录进入 retrying，超过 maxRetries 后进入 dead。业务事实可能已落库，但下游 WebSocket、Agent、索引都不会收到事件。
>
> 防止方式：表驱动测试所有事件类型都能映射非空 topic；统一构造函数收敛事件创建；维护事件注册表 `event_type -> topic -> payload schema/version`，CI 或启动期校验。

**注意点**

- 当前项目风险是事件常量和 topic 映射手工维护。

### 追问 2.11：Kafka / Outbox / Consumer Idempotency 边界

**我的问题**

总结 Kafka、Outbox、consumer idempotency 分别解决什么，为什么不能互相替代？

**你的回答**

> Kafka 负责基础设施建设，提供消息队列 SDK 给上层 eventbus 包装，负责消息收发。Outbox 解决双写一致性，确保 Kafka 挂掉后也能保证消息传递。consumer idempotency 是什么？幂等？

**推荐答案**

> Kafka 是事件总线，负责事件进入 broker 后的存储、分区、分发、消费者组、offset 和重放。
>
> Outbox 解决业务数据库与 Kafka 发布之间的双写窗口：业务表和 event_outbox 同事务提交，Kafka 不可用时后续重试。
>
> Consumer idempotency 是消费者幂等，解决 Kafka/Outbox 至少一次投递带来的重复消费问题。重复收到同一事件时，不重复产生不可接受的业务副作用。
>
> Kafka 不能替代 Outbox，因为 Kafka 不知道 MySQL 事务是否成功；Outbox 不能替代 Kafka，因为它没有高吞吐分区、消费者组、offset；二者都不能替代消费者幂等，因为至少一次意味着可能重复。

**注意点**

- 短句：Kafka 负责分发，Outbox 负责可靠发布，消费者幂等负责重复投递下不重复副作用。

## 3. Outbox 具体实现

### 追问 3.1：event_outbox 字段

**我的问题**

讲 `event_outbox` 关键字段：`id`、`aggregate_type`、`aggregate_id`、`event_type`、`event_key`、`payload`、`status`、`retry_count`、`next_retry_at`、`locked_until`、`published_at`、`last_error`。

**你的回答**

> 不知道。

**推荐答案**

> 可以按五组讲：事件身份、业务聚合关联、发布状态、重试控制、并发锁。
>
> `id/event_type/event_key/payload` 描述事件本身。`payload` 保存完整 `Envelope` JSON。`aggregate_type/aggregate_id` 关联业务聚合，如 `message + msg_id`，方便排查、审计、重放。`status/published_at` 表示 pending/retrying/published/dead 和发布时间。`retry_count/next_retry_at/last_error` 支持失败退避重试和排障。`locked_until` 是 Worker 并发扫描时的租约锁。

**注意点**

- Outbox 是应用层 WAL。

### 追问 3.2：event_type/event_key 冗余是不是反范式

**我的问题**

`payload` 已有 `Envelope.Type` 和 `Envelope.Key`，为什么表还冗余 `event_type` 和 `event_key`？是不是反范式？有什么工程价值？

**你的回答**

> Outbox 本质属于消息数据传递，payload 的 type 用于保存下游层级 type，便于下游提取消息信息，所以需要单独维护上游 type，像 envelope 一样。反范式是什么？

**推荐答案**

> 这是有意反范式。同一份信息在 payload JSON 和独立字段中冗余存储。这样可以直接按 `event_type`、`event_key` 查询、建索引、排查、重放，不需要每次解析 JSON。
>
> 比如查所有失败的 `message.created` 或按某个 conversation_id 重放，字段查询比 JSON 解析更简单可靠。

**注意点**

- 反范式不是错误，有时是为了查询和运维效率。

### 追问 3.3：aggregate 与 event 的区别

**我的问题**

`aggregate_type/aggregate_id` 和 `event_type/event_key` 有什么区别？为什么 `message.created` 里 `aggregate_id=msg_id`，`event_key=conversation_id`，不是都用 message_id？

**你的回答**

> aggregate_type/aggregate_id 是消息聚合说明，不参与实际业务，仅作为可观测性部件。event_type/event_key 属于消息类型的反范式。

**推荐答案**

> `aggregate_type/aggregate_id` 表示这条事件属于哪个业务对象，用于排查、审计、重放，例如 `message + msg_id`。`event_type/event_key` 表示事件语义和 Kafka 分区维度。
>
> 对 `message.created`，`aggregate_id=msg_id` 是为了定位具体消息事实；`event_key=conversation_id` 是为了同一会话事件进入同一 partition，保持会话维度顺序。

**注意点**

- aggregate_id 管“这是谁产生的事件”；event_key 管“发 Kafka 时按什么排队”。

### 追问 3.4：retrying 和 dead

**我的问题**

Outbox 为什么有 `retrying` 和 `dead`？为什么失败后不无限重试？dead 后怎么处理？

**你的回答**

> retrying 和 dead 是为了保证消息可消费性，不能因单次失败抛弃消息。dead 反映内部问题，转向死信队列人工排查。无限重试会造成性能消耗。dead 后应该放进死信队列人工审计。

**推荐答案**

> `retrying` 表示临时失败，稍后重试，如 Kafka 短暂不可用、网络超时、leader 切换。`dead` 表示连续失败超过上限，移出普通重试循环，需要人工介入。
>
> 无限重试会占用 Worker、打爆 Kafka、制造日志噪声，让毒丸消息长期卡住资源。dead 后应告警，查看 `last_error/payload/event_type`，修复配置或代码后通过 `Requeue` 放回 pending。

**注意点**

- 项目当前是 outbox 表内 dead 状态，不一定是独立 DLQ topic。

### 追问 3.5：next_retry_at 退避重试

**我的问题**

为什么用 `next_retry_at` 做退避？Kafka 挂 10 分钟，所有事件每秒重试会怎样？

**你的回答**

> 不知道。为了防止同一个错误快速判断失败多次？如果 Kafka 挂 10 分钟，所有事件都每秒重试，会导致所有消息 dead。

**推荐答案**

> `next_retry_at` 用于退避重试。失败后不要立刻疯狂重试，而是逐步延后，如 1s、2s、4s、8s，最大约 60s。
>
> Kafka 挂 10 分钟时，如果所有事件每秒重试，会导致 Worker 空转、数据库频繁更新 retry_count/last_error、日志暴涨、Kafka 恢复前被无效请求冲击、恢复后被积压洪峰打爆，并可能让大量事件快速进入 dead。

**注意点**

- 后续可加全局熔断和恢复限速。

### 追问 3.6：轮询 Outbox 的不足

**我的问题**

Outbox Worker 当前轮询 `event_outbox`。轮询有什么不足？规模变大怎么优化？

**你的回答**

> 轮询会导致无法感知消息高峰期，减弱削峰填谷，可能导致某段时间过多消息进入 outbox，worker 负荷过大。优化是增加感知线，达到阈值立即启用一次 worker。

**推荐答案**

> 轮询优点是简单可靠，通知丢了也不丢事件，因为 MySQL outbox 是事实源。缺点是低峰空扫、高峰延迟、积压时吞吐有限，扫描和锁定给数据库压力。
>
> 优化：自适应 polling；事务提交后轻量唤醒但仍以 outbox 为事实源；增加批量和并发 Worker；按 event_type/聚合分片；监控 pending/retrying/dead、最大积压时间；规模大后拆独立 outbox publisher。

**注意点**

- 轮询本身是削峰的一种，不要说它减弱削峰。

### 追问 3.7：published 记录堆积

**我的问题**

Outbox 表越来越大，`published` 记录越积越多，会有什么问题？怎么归档/清理？注意什么？

**你的回答**

> 表越来越大会导致数据积压、削减性能。应该定期删档或放入其他数据库，只清理时间序列的 published 标记数据，且不应太近。

**推荐答案**

> `published` 长期堆积会让表和索引膨胀，影响扫描、备份、查询和存储成本。应设置保留窗口，只清理已经 published 且超过保留期的事件；有审计/重放需求先归档到历史表、对象存储或日志系统，再分批删除。
>
> 不能清理 pending/retrying/dead；不能一次大事务删海量数据；要考虑消费者滞后和审计保留。规模更大可用分区表。

**注意点**

- 用“归档/清理策略”，不要说“删档”。

### 追问 3.8：事件结构升级兼容

**我的问题**

Outbox payload 保存完整 Envelope。如果以后 `MessagePayload` 新增字段或改字段名，老 outbox 事件还能发布吗？消费者怎么兼容版本？

**你的回答**

> 不知道。

**推荐答案**

> 新增字段通常没问题，旧 JSON 反序列化时新字段为零值。但改字段名、删除字段、改变语义会破坏兼容。比如旧 payload 是 `target_user_ids`，新消费者只读 `receiver_ids`，就会丢目标用户。
>
> 事件演进原则：尽量只新增字段；`Envelope.Version` 真正用起来；消费者宽松读取；重大变更新事件类型或 v2 payload；先升级消费者兼容新旧，再升级生产者。

**注意点**

- 当前项目版本兼容策略比较基础，可以承认。

### 追问 3.9：事务提交失败与 HTTP 响应失败

**我的问题**

Outbox 和业务表同事务。如果事务提交失败会怎样？如果事务提交成功但 HTTP 响应失败，客户端怎么避免重复发送？

**你的回答**

> 事务提交失败会把错误往上抛，返回前端标识操作出错。如果事务提交成功但 HTTP 响应返回失败，会导致用户无法识别错误信息，可能多次重复操作，所以要做幂等。

**推荐答案**

> 事务提交失败时，`messages`、`message_user_states`、`event_outbox` 都回滚，消息事实不存在，Kafka 后续也不会发布，客户端可重试。
>
> 事务提交成功但响应失败时，服务端已写入消息和 outbox，下游事件也会发生，但客户端以为失败，可能再次发送。需要 `client_msg_id`：客户端重试必须复用同一个值，服务端用唯一约束识别同一次发送的重试，避免重复消息。

**注意点**

- `client_msg_id` 是发送请求幂等，不是 Kafka 消费幂等。

### 追问 3.10：Outbox 总结

**我的问题**

用一段话总结：Outbox 解决什么，没解决什么，为什么比直接发 Kafka 更适合消息发送链路？

**你的回答**

> Outbox 解决 Kafka 和数据库双写一致性，没解决重复问题。比直接发 Kafka 更适合消息发送链路，因为能做到最终一致性。

**推荐答案**

> Outbox 解决业务数据库和 Kafka 发布之间的双写一致性。消息发送时，`messages`、`message_user_states` 和 `event_outbox(message.created)` 在一个 MySQL 事务提交。事务提交后 Worker 异步发布 Kafka，即使 Kafka 不可用或服务在 DB 提交后、Kafka 发布前崩溃，事件也不会丢。
>
> 但 Outbox 不解决 exactly-once。发布成功但未标记 published、消费者 handler 成功但 offset 提交失败，都可能重复。所以还需要消费者幂等。
>
> 它比直接发 Kafka 更适合高频消息链路，因为主链路只做可靠写库和事件记录，下游异步消费，降低耦合和延迟。

**注意点**

- 短句：Outbox 防丢，不防重；保证最终一致，不保证只处理一次。

## 4. DTM

### 追问 4.1：DTM 和 Outbox 边界

**我的问题**

DTM 在 ClaranAIM 解决什么？和 Outbox 边界是什么？为什么消息发送不用 DTM，而创建群适合 DTM Saga？

**你的回答**

> DTM 负责群聊创建最终一致性，属于群聊领域；Outbox 是消息领域。消息发送不用 DTM 因为高频，用 DTM 会造成非必要性能消耗，创建群低频重要，适合 Saga。不熟项目中 DTM 方法和原理。

**推荐答案**

> Outbox 是事件可靠发布模式；DTM Saga 是跨服务补偿事务模式。它们不是按群聊/消息领域划分，而是按问题类型划分。
>
> DTM 适合低频、跨服务、需要显式补偿的流程，如创建群同时涉及 group-service 创建群资料/成员，msg-core-service 创建群会话/参与者。后续分支失败时，需要补偿前面已提交分支。
>
> 消息发送是高频核心链路，只需保证消息事实落库后事件最终发布，用 MySQL 事务 + Outbox 更轻量。每条消息走 DTM 会引入全局事务、分支调用、补偿管理和额外延迟，不划算。

**注意点**

- 高频事实落库后的异步副作用用 Outbox；低频跨服务写操作用 DTM Saga。

### 追问 4.2：为什么预生成 group_id/conversation_id？

**我的问题**

创建群时为什么预生成 `group_id` 和 `conversation_id`？为什么不能先让 group-service 创建群拿 group_id，再让 msg-core-service 创建会话？

**你的回答**

> 不知道。

**推荐答案**

> DTM Saga 分支在提交前要把 action/compensate/data 编排好交给 DTM Server。后一个分支不适合依赖前一个分支返回值再动态拼参数。
>
> api-gateway 预生成 `group_id` 和 `conversation_id`，使 group-service 和 msg-core-service 分支都拥有完整参数，并且补偿时也知道删哪个 group、哪个 conversation。固定 ID 也支持重试幂等，不会每次生成新群或新会话。

**注意点**

- 这是 DTM Saga 参数独立和幂等的核心。

### 追问 4.3：Saga 补偿 vs MySQL rollback

**我的问题**

Saga 补偿和数据库事务 rollback 有什么区别？`/dtm/group/create_compensate` 等价 MySQL rollback 吗？

**你的回答**

> saga 是低频，DTM 可以少问点。

**推荐答案**

> Saga 补偿不是数据库 rollback。Rollback 发生在本地事务提交前，外界看不到中间状态；Saga 每个分支是已经提交的本地事务，后续失败时只能调用反向业务操作撤销。
>
> 比如 group-service 创建群已提交，后续会话创建失败，DTM 调 `/dtm/group/create_compensate` 执行 `DeleteGroup(group_id)`。这不是 MySQL rollback，而是业务补偿。补偿可能重复调用，所以必须幂等。Saga 提供最终一致，不是强一致。

**注意点**

- 记四句：低频跨服务；正向+补偿；补偿不是 rollback；分支和补偿要幂等。

## 5. Redis

### 追问 5.1：Redis 用在哪些地方？

**我的问题**

ClaranAIM 里 Redis 主要用在哪些地方？不要只说缓存，要说 cache-aside、防穿透、防击穿、TTL 抖动、在线状态/会话状态。

**你的回答**

> 充当缓存基础设施，用 cache-aside 策略，即写后删除。另外作为分布式锁应对缓存击穿，附加随机过期时间防止缓存雪崩和穿透。在线状态/会话状态不清楚。

**推荐答案**

> 项目 Redis 主要用于 cache-aside 缓存和部分状态缓存设计。封装了 key 策略、TTL、随机抖动、空值缓存和防击穿锁。空值缓存防穿透，TTL 抖动防雪崩，SETNX 锁加 Lua 安全释放防热点 key 击穿。
>
> 用户、好友、群、群成员、用户会话列表等适合走这套策略。在线状态有 `online:user:{id}` 策略预留，但当前 WebSocket 在线连接主要还是网关内存 Hub，分布式 presence 需要补强。

**注意点**

- TTL 抖动防雪崩；空值缓存防穿透；SETNX 锁防击穿。

### 追问 5.2：为什么写后删除缓存？

**我的问题**

cache-aside 为什么通常是写数据库后删除缓存，而不是写数据库后直接更新缓存？风险是什么？

**你的回答**

> 写数据库后直接更新缓存，如果两个请求同时打入，可能出现缓存不正确。写后删除可以防止这一点，不重构缓存，让下一次查询打 DB 重构。

**推荐答案**

> 写 DB 后直接更新缓存看起来实时，但多实例、网络延迟、重试场景下，缓存更新顺序可能和 DB 提交顺序不一致，旧值可能覆盖新值。写后删除让数据库继续作为事实源，写请求只使缓存失效，下一次读从 DB 重建，降低乱序风险。
>
> 但写后删除也有风险：删除缓存失败会留下旧缓存；读写并发可能把旧值重新回填。可通过 TTL、重试删除、延迟双删、binlog 订阅、版本校验等补偿。

**注意点**

- cache-aside 不是强一致，是降低不一致窗口。

### 追问 5.3：穿透、击穿、雪崩

**我的问题**

解释缓存穿透、击穿、雪崩，并对应项目手段。

**你的回答**

> 缓存穿透是多次访问数据库不存在值，无法建立缓存，大量请求打 DB，可用缓存空值或布隆过滤器。击穿是多个请求同时打同一个数据导致 DB 压力激增，可用分布式锁。雪崩是大量缓存同一时刻过期，大量请求直接打 DB，可用过期时间抖动。

**推荐答案**

> 穿透：查不存在数据，每次 miss 打 DB。项目用短 TTL 空值缓存，生产可加布隆过滤器。
>
> 击穿：热点 key 过期，大量请求同时 miss 打 DB。项目用 SETNX 锁，只有抢到锁的请求回源并回填。
>
> 雪崩：大量 key 同时过期导致集中回源。项目用 TTL 随机抖动错开过期时间。

**注意点**

- 击穿要强调“热点 key 过期”。

### 追问 5.4：Redis 锁释放为什么用 Lua？

**我的问题**

Redis 锁释放为什么用 Lua 判断 token 后删除？直接 `DEL lockKey` 有什么问题？

**你的回答**

> Lua 脚本的原子性保证强一致性。

**推荐答案**

> 不是泛泛保证强一致性，而是防止误删别人的锁。请求 A 获取锁 token=A，执行超过 TTL，锁过期；请求 B 获取同一锁 token=B；A 结束后如果直接 DEL，就会删掉 B 的锁。
>
> 所以释放时必须原子执行：`GET lockKey == token` 才 `DEL`。Lua 保证判断和删除在 Redis 内原子执行。

**注意点**

- 核心词：随机 token、防误删、判断+删除原子。

### 追问 5.5：Redis Sentinel vs Cluster

**我的问题**

当前 Redis 有哪些不足？如果问“为什么用 Sentinel 不用 Cluster？Cluster 更强啊”，怎么回答？

**你的回答**

> Redis Sentinel 和 Cluster 主要用在限流方面，不太清楚。当前项目缺陷主要是没有 Redis 集群。

**推荐答案**

> Sentinel 和 Cluster 不是主要用于限流，而是 Redis 高可用/扩展方案。Sentinel = 主从复制 + 哨兵监控 + 自动故障转移，重点是高可用；Cluster = 数据分片 + 多主多从 + 自动故障转移，重点是水平扩容 + 高可用。
>
> Sentinel 适合数据量/QPS 未达到分片需求，但需要主从高可用；Cluster 适合单机容量或吞吐成为瓶颈，需要水平分片。
>
> 当前项目更准确是单机开发配置。生产演进可先 Sentinel 降低运维复杂度、获得故障转移；当在线状态、分布式限流、缓存规模上来后，再考虑 Cluster。

**注意点**

- 不要说项目已完整实现 Redis 高可用。

## 6. 限流与降级

### 追问 6.1：限流不足与生产级设计

**我的问题**

当前项目限流是什么级别？有什么不足？如果要改生产级分布式限流，怎么设计？

**你的回答**

> 本题还没正式回答，后续你调整顺序进入 Agent/RAG。

**推荐答案**

> 当前限流偏基础，更多是网关或单实例内存令牌桶/治理配置，适合开发和单实例场景。主要不足是多网关实例下限流状态不共享，不能按用户、IP、接口、Bot、会话、LLM token 成本做精细控制，也缺少全局配额、降级策略和限流观测。
>
> 生产级可用 Redis + Lua 实现分布式令牌桶/滑动窗口：key 维度包括 `user_id`、IP、接口、Bot、conversation、tenant；Lua 保证扣减原子性；结合不同接口权重和 LLM token budget；命中限流时返回明确错误或降级为排队/低频任务；配合熔断、超时、降级、监控告警。

**注意点**

- Agent 场景下要加“token 成本限流”和“工具调用限流”。

## 7. Agent Runtime / Agent-Native 链路

### Agent 追问 A1：@Agent 后如何触发并回复？

**我的问题**

用户在 IM 里 @Agent 后，Agent 怎么被触发并回复？按链路讲：`message.created` -> `claran.message.events` -> agent-manager -> Dispatcher -> runtime -> 以 `agent_user_id` 写回 msg-core。

**你的回答**

> 消息发出时产生事件与载荷，Agent dispatcher 消费事件并判断是否触发。这里存在 mentionID=agent，所以触发，调用 runtime，思考后调用 msg 写回 msgcore。

**推荐答案**

> 用户消息先由 msg-core-service 写消息事实和 Outbox，然后发布 `message.created` 到 `claran.message.events`。agent-manager 启动时消费 `claran.message.events` 和 `claran.im.events`，统一进入 `AgentEventDispatcher`。
>
> Dispatcher 把旧 `MessagePayload` 和新 `IMEventPayload` 解码成统一 `agentEvent`，先过滤非触发事件、Agent 自回声，再根据私聊默认、群聊 @、订阅规则判断 trigger/record/ignore。触发前用 `agent_dispatch_records(event_id, agent_user_id)` 做幂等。
>
> 然后读取 Agent 可见会话上下文，调用 `agentService.ChatWithBot`，由 manager 转 runtime。拿到回复后，调用 msg-core-service `SendMessage`，以 Agent 的 `agent_user_id` 作为 sender，并带稳定 `client_msg_id=agent:{sourceEventID}:{agentUserID}` 写回。

**注意点**

- 不是只看 mentionID，还有过滤、规则、幂等、审计。

### Agent 追问 A2：为什么回复写回 msg-core 而不是直接推 WS？

**我的问题**

为什么 Agent 回复要用 `agent_user_id` 写回 msg-core-service，而不是 agent-manager 直接 WebSocket 推一条 AI 回复？

**你的回答**

> 这就是 Agent 原生。Agent 回复复用 msg-core-service 发送链路，所以回复也是普通 IM 消息事实，进入审计、WebSocket 推送、历史消息和后续事件链路。这体现了 Agent 作为真实用户而不是外部聊天机器人。

**推荐答案**

> 直接 WebSocket 推只是在线展示，不是业务事实。离线用户收不到，历史查不到，多端同步不完整，撤回编辑审计不好做，也不会进入后续事件链路。
>
> 项目让 Agent 拥有真实 `agent_user_id`，回复时调用 msg-core-service 的 `SendMessage`，以 `agent_user_id` 作为 sender 写入 `messages`。这样 Agent 回复和普通消息一样进入 `messages`、`message_user_states`、Outbox、Kafka、WebSocket、历史、审计和后续 Agent/会话智能链路。

**注意点**

- 短句：WebSocket 推送只是在线展示，msg-core 写消息才是业务事实。

### Agent 追问 A3：如何避免 Agent 自回声？

**我的问题**

Agent 回复也会生成 `message.created`。为什么不会再次触发自己形成无限循环？项目用了哪些防自回声机制？

**你的回答**

> 项目里 dispatcher 做了回声过滤，具体实现不清楚。

**推荐答案**

> 多层防线：第一，Agent 写回时使用 `client_msg_id = agent:{sourceEventID}:{agentUserID}`，Dispatcher 看到 `client_msg_id` 或 `idempotency_key` 以 `agent:` 开头就跳过。第二，msg-core-service 会在 metadata 中标记 `agent_generated=true`、`source=agent`。第三，Dispatcher 根据 `sender_id` 查询是否是某个 Agent 的 `agent_user_id`，如果发送者本身是 Agent 系统用户则静默。第四，`agent_dispatch_records(event_id, agent_user_id)` 防 Kafka 重投重复触发。

**注意点**

- 记四层：client_msg_id 前缀、metadata、sender_id 是 Agent、dispatch 幂等。

### Agent 追问 A4：Manager 和 Runtime 为什么拆？

**我的问题**

Agent Manager 和 Runtime 分别负责什么？为什么不把配置、事件分发、LLM 执行、工具调用都放一个服务？

**你的回答**

> Agent Manager 负责 Agent 配置管理层和数据库事实写入，Agent Runtime 负责 Agent 执行面。这样分微服务更好，具体怎么说？

**推荐答案**

> `agent-manager-service` 是控制面，负责 Agent 配置、系统用户、权限、订阅/路由规则、Kafka 事件消费、Dispatcher、`agent_dispatch_records`、任务状态、审计、计费、调用 runtime 并把回复写回 msg-core。
>
> `agent-runtime-service` 是执行面，负责 LLM 执行、Eino/DeepAgent、工具调用、MCP/Skill、工作区沙箱、RAG/Memory 上下文使用、多轮运行态、生成回复。
>
> 拆分原因是控制面需要稳定、短事务、可审计；执行面长耗时、高成本、依赖外部模型和工具，失败率更高。拆开后可独立扩容、限流、熔断、降级和观测。

**注意点**

- 承认不足：当前部分 runtime 调用仍同步，理想是 task worker 化。

### Agent 追问 A5：一次 Chat 的 runtime 主链路

**我的问题**

Agent Runtime 一次 Chat 会经历哪些步骤？从接收输入到返回回复，涉及 LLM profile、system prompt、工具、RAG/Memory 吗？

**你的回答**

> 不知道。

**推荐答案**

> manager 的 `ChatWithBot` 先查 Bot 配置、检查启用状态和权限、应用默认 LLM 配置、生成 sessionID、召回 Memory、构造 `RuntimeBotConfig`，然后 RPC 调 runtime。
>
> runtime 收到 `RunAgentReq` 后校验配置，创建或复用 Eino DeepAgent，加载 JSONL session 历史，把历史消息和当前输入组成 AgentInput，调用 Agent.Run，收集模型输出和 token usage，写入 session store，返回 reply/usage/session_id。
>
> DeepAgent 构建时会组装 OpenAI-compatible ChatModel、system prompt、ToolPolicyInstruction、FileSystemInstruction、WorkspaceSandbox、Skill middleware、SafeToolMiddleware、Eino tools、可选 tracing。

**注意点**

- Memory 主要由 manager 侧预注入；RAG/GraphRAG/MCP 主要作为工具暴露给 Agent。

### Agent 追问 A6：Memory / RAG / Milvus 关系

**我的问题**

项目里的 Memory、RAG、Milvus 三者是什么关系？Memory 和 RAG 都能召回，有什么区别？

**你的回答**

> RAG 和 memory 都依赖 Milvus。memory 的召回是简易 RAG 策略，RAG 里包含分层检索、混合检索、rerank、CRAG、Self-RAG 等复杂范式。

**推荐答案**

> Milvus 是向量数据库/向量索引基础设施。Memory 和 RAG 是两个使用向量检索的业务系统。
>
> Memory 面向 Agent 长期记忆，存用户偏好、会话事实、Agent 运行摘要、候选记忆等，核心维度是 bot/user/conversation/session/scope/type/visibility，召回强依赖身份和可见性。
>
> RAG 面向文档和知识库，存文档 chunk、文件知识、项目资料，链路包括解析、chunk、embedding、向量/关键词/混合检索、rerank、GraphRAG、CRAG/Self-RAG、答案合成。
>
> Milvus 可作为二者向量索引底座，但项目也有降级路径，不能说没有 Milvus 就完全不能跑。

**注意点**

- 不要说“都依赖 Milvus”，要说“可使用/优先使用 Milvus”。

## 8. RAG / Milvus

### RAG/Milvus 追问 R1：文档入库链路

**我的问题**

RAG 文档入库时，从上传文档到 Milvus，经历哪些步骤？讲出 parse、chunk、embedding、metadata、vector index、graph build。

**你的回答**

> 文档上传并传输到 RAG 节点，后端切分为 parent chunks 和 child chunks，每个 parent chunk 有摘要，便于分层检索。然后 chunk 用 embedding 入库。GraphRAG 不清楚。

**推荐答案**

> 入库先创建 `rag_documents` 元数据，记录 owner、title、source、source_type、visibility、group/conversation 等权限信息。然后对内容 parse 和分块：parent chunk 保留较大语义上下文，child chunk 用于精确向量检索，child 关联 `parent_chunk_id`。
>
> 每个 child chunk 生成 embedding，写入 vector index。生产可用 Milvus，collection 字段主要是 `chunk_id/document_id/vector`；完整文本、权限、source_type 等仍在 MySQL 的 `rag_chunks/rag_documents`。
>
> 入库后，如果 source_type 适合建图，进入 `buildGraph`：抽取 entity/relation，清洗、归一化、去重、过滤后写 graph store，并重建 community summary。

**注意点**

- Milvus 不存完整业务事实。

### RAG/Milvus 追问 R2：为什么 parent-child chunk？

**我的问题**

为什么要 parent-child chunk？为什么不直接固定大小 child chunk 全部向量检索？

**你的回答**

> 小 chunk 检索准但上下文不完整；大 chunk 上下文完整但 embedding 被稀释。Parent-child 是对同一内容切大块和小块，child 用于检索，命中后通过 parent_id 找 parent 或相邻 child 给 LLM。Parent 太大时用 parent summary + hit child，或 child 前后邻居。切分要有 overlap，并按 Markdown 标题、PDF/Word 段落、代码结构切。

**推荐答案**

> 你的答案已经很好。核心是解决检索精度和上下文完整性的矛盾。child chunk 语义聚焦，向量检索更准；但缺上下文。parent chunk 上下文完整，但 embedding 容易被多主题稀释且可能撑爆 prompt。
>
> 所以 child 用于检索，命中后通过 `parent_chunk_id` 找 parent，再把 parent 摘要、命中 child、相邻 child 或截断 parent 作为回答上下文。

**注意点**

- overlap 防语义切断，但过大导致重复召回和存储膨胀。

### RAG/Milvus 追问 R3：为什么 Milvus 后还要查 MySQL？

**我的问题**

Milvus 搜索返回 `chunk_id` 和 score。为什么还要回 MySQL 查 chunk/document？为什么不把所有文本和权限信息塞进 Milvus？

**你的回答**

> 不知道。

**推荐答案**

> Milvus 负责向量相似度搜索，不是业务事实库。它适合返回 TopK 的 chunk_id/document_id/score。完整 chunk 文本、document 标题、owner、visibility、group_id、conversation_id、source_type、审核状态等在 MySQL。
>
> RAG 不能只按相似度返回，还要做权限过滤和答案合成。把复杂文本和权限都塞进 Milvus，会让向量库承担业务数据库职责，更新、删除、审计、降级重建都更难。MySQL 保业务事实，Milvus 保向量索引。

**注意点**

- 短句：Milvus 负责“相似”，MySQL 负责“事实和权限”。

### RAG/Milvus 追问 R4：Milvus / embedding 降级

**我的问题**

如果 Milvus 不可用会怎样？embedding provider 没配置会怎样？这些降级能不能说成生产级？

**你的回答**

> Milvus 不可用，项目会以不清楚的方式降级 MySQL。如果没有 embedding，就降级分块 + MySQL。

**推荐答案**

> 两个降级点：Milvus 不可用 -> `LocalVectorIndex`；embedding provider 不可用 -> `hashEmbedding`。
>
> `LocalVectorIndex` 是进程内 map 保存 chunk 向量并做 cosine 搜索，保证演示和本地开发流程可跑，但不持久、不可多实例共享、无 Milvus 索引能力、规模大性能差。
>
> `hashEmbedding` 是伪向量，保证 pipeline 不崩，但不具备真实语义表示能力。
>
> 所以二者都不能吹成生产级。生产应使用真实 embedding provider + Milvus，并在管理台和监控中显示是否处于降级模式。

**注意点**

- 短句：LocalVectorIndex 保流程，hashEmbedding 保不崩；都不是生产级语义检索。

## 9. GraphRAG / Neo4j

### GraphRAG 追问 G1：为什么需要 GraphRAG？

**我的问题**

已经有向量 RAG，为什么还要 GraphRAG？GraphRAG 解决普通向量检索什么问题？

**你的回答**

> 不知道。

**推荐答案**

> 向量 RAG 擅长找语义相似文本片段，但不擅长关系型、多跳型、全局结构型问题。GraphRAG 把文档中的实体、关系和社区摘要抽取出来，如 service、table、topic、API、module 等。
>
> 查询时可围绕命中实体扩展邻居和多跳关系，回答“谁调用谁、谁写哪张表、哪个服务消费哪个 topic”这类问题。它补足普通 RAG 在全局结构、多跳推理、实体归并和图谱可视化治理上的不足。

**注意点**

- 短句：向量 RAG 找相似片段，GraphRAG 找实体关系网络。

### GraphRAG 追问 G2：从 chunk 到 entity/relation

**我的问题**

项目里 GraphRAG 构建怎么做？从 chunk 到 entity/relation，再到 Neo4j 或内存图谱，讲一遍。

**你的回答**

> 程序会调用 LLM 从 chunk 里抽取实体和关系，再经过过滤等处理，再入 Neo4j。

**推荐答案**

> GraphRAG 构建发生在 RAG 文档入库后。`IngestDocument` 保存文档和 chunks、写入向量索引后，如果 source_type 适合建图，就调用 `buildGraph`。它从候选 chunk 抽取实体和关系。
>
> 抽取器有规则抽取 fallback，也可以配置 `LLMGraphExtractor`。抽取结果经过清洗、归一化、去重、过滤弱实体/弱关系、保留证据等步骤，避免噪声写入图谱。
>
> 最后通过 `graphStore` 写入图谱后端。生产可用 Neo4j，未启用时使用内存/本地降级图谱。写完后还会构建 community summary。

**注意点**

- 不是只有 LLM；也有规则 fallback。
- 不是一定入 Neo4j；可能是内存/降级 graph store。

### GraphRAG 追问 G3：Neo4j 负责什么？内存图谱为什么不是生产级？

**我的问题**

Neo4j 在这里负责什么？如果 Neo4j 不启用，内存图谱降级有什么问题？为什么不能说成生产级？

**你的回答**

> Neo4j 对比其他数据库更适合高效存储图结构数据。

**推荐答案**

> Neo4j 在项目里负责持久化 GraphRAG 抽取出的实体和关系，并支持按实体查询邻居、多跳路径、文档子图和前端可视化。相比 MySQL，Neo4j 的优势是图遍历和关系查询更自然。
>
> 如果 Neo4j 不启用，项目可降级到内存图谱，保证本地演示和开发流程。但内存图谱不持久、多实例不共享、重启可能丢失、数据量大后内存和查询性能不可控，也缺少 Neo4j 的索引、约束和图查询能力。因此只能说是降级/MVP。

**注意点**

- 短句：Neo4j 管实体关系持久化和图遍历；内存图谱保演示流程，不保生产能力。

## 10. 项目亮点与不足

### 综合收束题：Demo 质疑

**我的问题**

如果面试官说：“你这个 Agent + RAG + GraphRAG 听起来很多都是 Demo，你自己觉得项目亮点和不足分别是什么？”怎么答？

**你的回答**

> 亮点是打通了链路，缺点是都是 demo。

**推荐答案**

> 最大亮点不是某个单点 AI 功能，而是把 Agent 放进 IM 原生链路里。Agent 不是外部机器人，而是有真实 `agent_user_id` 的系统用户，能复用消息发送、历史记录、权限、审计、Outbox/Kafka 事件和 WebSocket 推送。用户在私聊或群聊中触发 Agent 后，消息事实先落库，再通过事件触发 Agent Dispatcher，最后 Agent 以真实用户身份写回 msg-core-service，这条链路是闭合的。
>
> 第二个亮点是知识增强能力已有完整骨架：Memory 做长期记忆，RAG 做文档 chunk、embedding、向量检索和 rerank，Milvus 作为向量索引，GraphRAG 用实体关系和 Neo4j/内存图谱补足多跳关系查询。它们是 Agent 能力边界的一部分，而不是孤立页面功能。
>
> 不足要具体说：Agent Runtime 仍有同步路径，任务异步化、流式输出、工具审批、服务降级要加强；RAG/GraphRAG 的 LocalVectorIndex、hashEmbedding、内存图谱是开发降级，不是生产级；GraphRAG 抽取质量依赖规则和 LLM，需要审核和评估集；在线状态、限流、消费者幂等覆盖还要补。

**注意点**

- 不要说“都是 demo”，要说“核心链路已打通，高级能力已有骨架，生产级治理和质量评估仍需补齐”。

## 11. CloudDisk 迁移理解

你给出旧项目：

```text
D:\CodeStudy\GoProjects\src\ClaranCloudDisk
```

我们把它作为理解 AIM 的锚点：

| CloudDisk | AIM |
|---|---|
| Gin 单体路由 | Hertz Gateway + Kitex 多服务 |
| handler -> service -> dao | gateway handler -> RPC -> service -> repo |
| MySQL 元数据 + MinIO 文件 | MySQL 消息事实 + Kafka/WS 事件 + Milvus/Neo4j 知识 |
| Redis 文件缓存/分片状态 | Redis 缓存、在线状态、限流、短期状态 |
| 文件权限围绕 user_id | 会话权限围绕 user_id，Agent 也要 AgentUserID |
| 同步上传下载 | 消息写库 + Outbox + Kafka 异步 |

可背话术：

> 我之前在 CloudDisk 中掌握了单体 Go Web 项目的分层架构、JWT、Redis、对象存储、分片上传、限流和权限控制。AIM 是在这个基础上的复杂化：从单体变成微服务，从同步调用变成事件驱动，从文件存储变成消息事实和 Agent runtime，从普通缓存扩展到在线状态、分布式限流和异步链路辅助。

## 12. 异步专项

**你的问题**

项目中哪些地方是异步的？

**推荐答案**

主要有：

1. Outbox Worker 异步发布 Kafka。
2. Kafka Consumer 异步消费：WebSocket、Agent、msg-core 群事件同步、conversation-intelligence。
3. WebSocket Hub 和 client read/write goroutine。
4. Agent 是事件异步触发，但 runtime 当前仍在 consumer 内同步等待，`agent_tasks` 为后续 worker 化预留。
5. conversation-intelligence 定时归档。
6. WebSocket 在线状态定时写 Redis。
7. DTM 分支 HTTP 服务后台启动，但它不是高频事件链路。

**注意点**

- 区分 goroutine 异步、Kafka 事件异步、Outbox 可靠异步、定时任务异步。
- DTM 是低频事务补偿，不是消息推送机制。

## 13. 后续还没补完的三块

你最后说明：

> 明天下午就要去了，现在还差 MCP、WS、conversation-intelligence，准备明天再弄。

明天优先顺序：

1. WebSocket：主链路强相关。
2. conversation-intelligence：事件沉淀为长期知识。
3. MCP：Agent 外部工具扩展层。

明天开场可以说：

```text
我们继续答辩准备。今天只补三块：WebSocket、conversation-intelligence、MCP。请先带我看源码入口，再让我讲链路，然后追问失败场景、技术债和推荐答案。
```

## 14. 综合评价

### 你的优势

- 能抓住 Agent 原生这个项目核心。
- CloudDisk 给了你 Redis、JWT、权限、对象存储、限流、分层架构的工程基础。
- 不会时愿意说“不知道”并看源码，这比乱编强。
- 对 MVP 和不足有真实认识。

### 当前短板

- 事件系统是最大短板：topic、event type、partition、offset、Outbox、幂等、重试、dead、版本兼容都需要继续熟。
- Agent runtime 需要继续补源码：manager/runtime 边界、上下文、工具、Skill、sandbox、usage/billing。
- RAG/GraphRAG 能讲大意，但需要记清降级边界，避免把 LocalVectorIndex/hashEmbedding/内存图谱说成生产级。
- 表达上要避免“为了新意”“隐秘思考”“都是 demo”这类容易被打穿的说法。

### 粗略评分

```text
项目定位：7/10
Agent 原生：7/10
消息/Outbox/Kafka：6/10
Redis：6.5/10
DTM：5.5/10
Agent runtime：5/10
RAG/Milvus：6/10
GraphRAG/Neo4j：5.5/10
抗追问稳定性：5.5/10
```

### 最后一段可背总结

> ClaranAIM 的核心不是单点 AI 调用，而是 Agent-Native IM。第一条主线是 IM 消息事实链路：消息先落 MySQL，再通过 Outbox 可靠发布 Kafka，驱动 WebSocket、Agent 和会话智能。第二条主线是 Agent 原生身份：Agent 是 `is_system=true` 的系统用户，有 `AgentUserID`，能复用会话参与者、消息 sender、权限和审计。第三条主线是知识增强：Memory、RAG、Milvus、Neo4j/GraphRAG、Skill 和 MCP 为 Agent 提供长期上下文和工具能力。
>
> 当前项目的价值在于核心链路打通，技术债在于高级能力仍偏 MVP：Agent runtime 任务异步化、工具审批、上下文压缩、RAG 质量评估、GraphRAG 审核、Redis 高可用、限流降级和可观测治理都需要继续补齐。
