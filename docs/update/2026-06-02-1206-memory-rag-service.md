# Change Note: Memory-RAG Service

## 1. What Changed

本次把 `memory-service` 从基础长期记忆 CRUD 重做为一个可召回、可过滤、可治理的 Memory-RAG 服务。

核心变化包括：

- 新增长期记忆向量索引：支持 Hash Embedding 本地降级和 GLM Embedding + Milvus 后端。
- 召回链路改为 `向量召回 -> Metadata 粗过滤 -> MySQL 回源校验 -> 融合打分 -> TopK/MinScore -> 可选 LLM 过滤 -> Prompt 注入`。
- 记忆事实增加 `importance`、`expired_at`、`superseded_by`、`previous_memory_id`、`vector_score`、`final_score`、`score_reason` 等字段。
- 新增 `memory_candidates` 候选记忆表，支持从聊天或 Agent 运行结果中先抽取 pending 候选，再由用户接受或拒绝。
- 新增记忆冲突处理：接受新候选时可以对旧记忆执行 `supersede`、`weaken` 或 `keep`。
- 扩展 `idl/memory.thrift` 并重新生成 Kitex 代码，api-gateway 增加候选记忆管理接口。
- Agent 注入长期记忆时会把当前消息作为 query 传给 memory-service，并明确告诉 LLM 记忆只作为可能相关背景。

这是一次功能扩展、RPC 契约变更、配置变更、测试变更和生成代码变更。

## 2. Why This Change Was Needed

原来的 memory 更像一个“能存、能列、能按上下文取”的事实表。它能满足基础画像，但不适合 Agent 在真实 IM 场景里使用，因为长期记忆会越来越多，只按列表顺序或固定上下文取会带来两个问题：

- 噪声注入：不相关记忆被塞进 prompt，LLM 容易硬套旧背景。
- 权限风险：只信向量库结果不够，向量库只能做候选召回，最终仍要回 MySQL 校验用户、Agent、作用域、启用状态和过期状态。

新的设计把 memory-service 做成一个轻量 RAG 系统，但刻意不引入复杂 GraphRAG、CRAG、Self-RAG 等流程。Memory 的重点不是回答知识库问题，而是给 Agent 提供“可能有用的长期背景”。所以它使用 Dense Vector Search 加治理规则：

```text
向量相似度
  + importance 重要性
  + recency 时效性
  + scope boost 作用域权重
  + TopK 和 MinScore
  + 可选小模型过滤
```

这个取舍的好处是：链路足够清晰，成本可控，权限边界明确，并且用户可以管理、拒绝、过期或替换记忆。

## 3. Files Changed

- `internal/memory-service/model/model.go`: 扩展 `memory_facts` 字段，新增 `memory_candidates` 表模型和候选状态、冲突策略枚举。
- `internal/memory-service/dao/dao.go`: 增加批量回源、召回列表、候选记忆 CRUD，并在迁移时创建 `memory_candidates`。
- `internal/memory-service/service/rag.go`: 新增 Memory-RAG 的向量索引、Embedding Provider、Milvus 后端、本地 hash 后端和可选 LLM 过滤器。
- `internal/memory-service/service/service.go`: 重写召回、融合打分、Prompt 注入、候选记忆接受/拒绝和冲突处理主流程。
- `internal/memory-service/service/service_test.go`: 增加 Memory-RAG 回源校验、融合打分、LLM 过滤、候选记忆和冲突处理测试。
- `idl/memory.thrift`: 扩展 RPC 契约，增加记忆评分字段和候选记忆接口。
- `kitex_gen/memory/**`: 根据新的 `memory.thrift` 重新生成 Kitex 代码。
- `pkg/memoryclient/types.go`: 扩展跨服务 DTO 和 `Service` 接口，供 api-gateway 和 Agent 调用。
- `pkg/memoryclient/rpc_client.go`: 增加新字段映射和候选记忆 RPC 调用。
- `internal/memory-service/handler/handler.go`: 增加 Kitex handler 对新字段和候选接口的适配。
- `internal/api-gateway/handler/memory_handler.go`: 增加候选记忆的 HTTP 管理接口，并支持创建/更新记忆时设置 `importance`。
- `internal/api-gateway/router/router.go`: 注册 `/memory/candidates`、`/memory/candidate/create`、`/memory/candidate/:id/accept`、`/memory/candidate/:id/reject`。
- `cmd/memory-service/main.go`: 启动时根据配置选择 Hash/GLM Embedding、本地/Milvus 索引和可选 LLM 过滤器。
- `config/memory-service.yaml`: 新增 `memory_rag` 配置块。
- `pkg/config/config.go`: 新增 `MemoryRAGConfig` 和 `MEMORY_RAG_*` 环境变量覆盖。
- `internal/agent-manager-service/service/service.go`: Agent 调用 memory recall 时传入当前消息作为 query，并加强注入策略提示。

## 4. Core Flow After The Change

### 记忆写入

```text
api-gateway / memoryclient / agent-manager
  -> memory-service Kitex handler
  -> service.CreateMemory
  -> MySQL memory_facts
  -> vector.Upsert
  -> 更新 vector_status=ready, embedding_ref=memory:{id}
```

这里 MySQL 是事实源，向量库只是召回索引。即使向量写入失败，也不应该让事实丢失；当前实现会尽力写索引，并保留服务可运行。

### 记忆召回

```text
Agent 当前输入
  -> memory-service Recall(query, bot_id, user_id, conversation_id, group_id, session_id)
  -> Milvus / Local Vector Search 召回候选 memory_id
  -> MySQL GetByIDs 回源校验候选
  -> MySQL ListVisibleForRecall 补充高重要性/近期记忆
  -> 检查 bot_id、user_id、scope、enabled、expired_at
  -> 融合打分
  -> MinScore 过滤
  -> TopK 截断
  -> 可选 LLM relevance filter
  -> FormatMemoryContext 注入 prompt
```

注入文本固定包含策略提示：

```text
以下是可能相关的长期记忆。
如果和当前问题无关，不要强行使用。用户当前输入优先级高于记忆。
```

这句话很重要。它把长期记忆从“必须服从的系统事实”降级成“可能有用的背景”，避免 Agent 因为旧记忆覆盖用户当前明确输入。

### 候选记忆治理

```text
聊天 / Agent 运行结果
  -> 抽取候选记忆
  -> memory_candidates(status=pending)
  -> 用户查看
  -> accept
       -> 写入 memory_facts
       -> 按冲突策略处理旧记忆
       -> candidate.status=accepted
  -> reject
       -> candidate.status=rejected
```

候选区的作用是防止自动抽取污染长期画像。比如 Agent 从一次临时玩笑里抽出“用户喜欢 XXX”，不应该直接写进长期记忆，而应该先进入 pending。

## 5. Key Code Concepts

### Dense Vector Search

`MemoryVectorIndex` 是长期记忆向量索引接口。当前有两个实现：

- `localMemoryVectorIndex`: 使用 hash embedding，适合测试和本地降级。
- `MilvusMemoryVectorIndex`: 使用 Milvus 保存记忆向量，适合真实召回。

向量召回只负责找候选，不负责最终信任判断。

### Metadata Filter

向量库写入时同步保存 `bot_id`、`user_id`、`group_id`、`conversation_id`、`scope`、`visibility` 等元数据。Milvus 查询目前用 `bot_id == X and user_id == Y` 做粗过滤，本地索引也会检查这些字段。

粗过滤不是权限终点，只是减少无关候选。

### MySQL Fact Check

向量库返回 memory_id 后，service 会调用 `repo.GetByIDs` 回源读取 `memory_facts`。只有满足以下条件的记忆才会进入下一步：

- `bot_id` 与当前 Agent 匹配。
- `user_id` 与当前用户匹配。
- `enabled=true`。
- `expired_at IS NULL`。
- `scope` 与当前 group/conversation/session 边界匹配。

这就是“不只看向量分数”的关键。

### Score Fusion

召回分数不是单纯的向量相似度，而是：

```text
final_score =
  vector_weight * vector_score
  + importance_weight * importance
  + recency_weight * recency
  + scope_weight * scope_boost
```

默认权重在 `config/memory-service.yaml` 和 `pkg/config/config.go` 中配置：

- `vector_weight`: 0.45
- `importance_weight`: 0.25
- `recency_weight`: 0.15
- `scope_weight`: 0.15

这让系统能同时考虑“语义相关、用户认为重要、最近出现、上下文更近”四类信号。

### Optional LLM Relevance Filter

如果 `MEMORY_RAG_LLM_FILTER_ENABLED=true`，召回后的候选会再交给小模型判断哪些真正有用。小模型只输出结构化 JSON：

```json
{"keep_ids":[123,456]}
```

应用代码再根据 `keep_ids` 过滤候选。LLM 在这里是判断器，不是权限执行者。

### Memory Governance

`memory_candidates` 是治理层。它让自动抽取的记忆先进入 pending，再由用户或规则接受。接受时可以处理冲突：

- `supersede`: 旧记忆设为 disabled，并写 `expired_at`、`superseded_by`。
- `weaken`: 旧记忆重要性减半，并写 `superseded_by`。
- `keep`: 保留旧记忆，只记录新旧时间线。

例如“用户不懂 Kafka”被“用户已经学完 Kafka 基础”替代时，就应该用 `supersede` 或 `weaken`，而不是让两条冲突记忆同时高权重存在。

## 6. Important Implementation Details

- `memory_facts` 仍保留 `bot_id` 字段名，这是为了兼容现有 Agent 实例 ID 和旧 IDL/数据库语义。当前业务上它代表 Agent 实例 ID。
- `CreateMemory` 写 MySQL 后会尝试写向量索引，并把 `VectorStatus` 更新为 `ready`。Milvus 不可用时，服务启动阶段会降级到本地向量索引。
- `Recall` 不会直接信任 Milvus 返回结果，必须回 MySQL 校验事实和权限。
- 用户只能管理 `OwnerUserID == 当前用户` 的记忆和候选记忆。
- `ListCandidates` 强制按 `OwnerUserID` 裁剪，避免用户看到别人的候选画像。
- `FormatMemoryContext` 只有在召回到记忆时才返回文本，不会空注入。
- 配置支持环境变量覆盖，敏感信息不需要写进 YAML。
- 本次没有实现前端候选记忆页面，只暴露了 HTTP 接口和 RPC 链路；前端可以基于这些接口继续补治理界面。

常用环境变量：

```text
MEMORY_RAG_ENABLED=true
MEMORY_RAG_EMBEDDING_PROVIDER=glm
MEMORY_RAG_EMBEDDING_URL=https://open.bigmodel.cn/api/paas/v4/embeddings
MEMORY_RAG_EMBEDDING_API_KEY=你的Embedding密钥
MEMORY_RAG_EMBEDDING_MODEL=embedding-3
MEMORY_RAG_MILVUS_ENABLED=true
MEMORY_RAG_MILVUS_ADDRESS=127.0.0.1:19530
MEMORY_RAG_MILVUS_COLLECTION=claran_memory_facts
MEMORY_RAG_VECTOR_CANDIDATE_K=80
MEMORY_RAG_MIN_SCORE=0.05
MEMORY_RAG_LLM_FILTER_ENABLED=false
MEMORY_RAG_LLM_FILTER_BASE_URL=https://open.bigmodel.cn/api/paas/v4
MEMORY_RAG_LLM_FILTER_API_KEY=你的小模型密钥
MEMORY_RAG_LLM_FILTER_MODEL=glm-4-flash
```

## 7. How To Verify

已执行以下验证：

```powershell
$env:GOCACHE='D:\CodeStudy\GoProjects\src\ClaranAIM\.gocache'
go test ./internal/memory-service/service ./internal/memory-service/... ./pkg/memoryclient ./internal/api-gateway/handler ./internal/api-gateway/router ./internal/agent-manager-service/service ./cmd/memory-service
```

结果：通过。

又执行全仓库测试：

```powershell
$env:GOCACHE='D:\CodeStudy\GoProjects\src\ClaranAIM\.gocache'
go test ./...
```

结果：通过。

## 8. What To Read Next

- `internal/memory-service/service/service.go`: 读 `Recall`、`collectRecallCandidates`、`scoreRecallCandidates`，理解 Memory-RAG 主链路。
- `internal/memory-service/service/rag.go`: 读 `MemoryVectorIndex`、`MilvusMemoryVectorIndex`、`LLMMemoryRelevanceFilter`，理解向量召回和轻量过滤器。
- `internal/memory-service/model/model.go`: 读 `MemoryFact` 和 `MemoryCandidate`，理解事实记忆与候选记忆的区别。
- `pkg/memoryclient/types.go`: 读跨服务 DTO，理解其他服务应该如何调用 memory-service。
- `internal/api-gateway/handler/memory_handler.go`: 读候选记忆接口，理解前端治理页面可用的入口。
