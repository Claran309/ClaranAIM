# GraphRAG LLM 抽取与社区划分升级

## 背景

本次将 `rag-service` 原来的“规则抽取 + MySQL 图谱 MVP”升级为更完整的 GraphRAG indexing 链路：

- 文档分块后优先使用小模型抽取实体和关系。
- 抽取失败或配置缺失时自动降级到原有规则抽取。
- 图谱仍落在 MySQL 事实表，供 `rag-service` 和 `knowledge-service` 共用。
- 基于实体关系重建社区，并生成社区摘要，供 GraphRAG 查询和前端图谱可视化使用。

Text-to-SQL 仍保持当前边界：只做路线识别和提示，不做只读 SQL schema 注册、SQL 生成和执行器闭环。

## 本次改动

### LLM 实体/关系抽取

新增 `LLMGraphExtractor`，使用 OpenAI-compatible `/chat/completions` 调小模型，从 chunk 文本中抽取：

- 实体：`Service`、`DatabaseTable`、`EventTopic`、`API`、`Module`、`Concept`、`Person`、`Organization`、`Product`
- 关系：`CALLS`、`PUBLISHES`、`CONSUMES`、`STORES`、`OWNS`、`DEPENDS_ON`、`CONFIGURES`、`TRIGGERS`、`READS`、`WRITES`、`RELATED_TO`

LLM 只负责输出结构化 JSON；应用代码负责校验、归一化、落库和权限边界。

### 实体归一化

保留并强化原有归一化能力：

- 使用 canonical key 合并同一实体的不同写法。
- 合并 aliases，避免 `msg-core-service`、`msg core service`、`消息核心服务` 变成多个孤立节点。
- 无效类型会回退到本地推断类型。
- 关系的 source/target 必须引用已抽取实体，否则丢弃。

### 社区划分

新增 GraphRAG 社区重建流程：

1. 读取 owner 下完整实体和关系。
2. 基于关系权重构建无向加权邻接图。
3. 用强边连通组件形成初始社区。
4. 做 Leiden 风格的局部移动优化。
5. 对断连社区做二次拆分。
6. 重建 `rag_communities`，并更新实体 `community_id`。

当前是 Go 内部的 Leiden 风格实现，没有引入 Python/igraph/leidenalg 运行时。它能满足 MVP 的社区聚合、弱边隔离和前端社区过滤需求；如果后续需要严格论文级 Leiden，可再引入离线图计算 worker。

### 社区摘要

新增 `LLMGraphCommunitySummarizer`：

- LLM 可根据社区实体和内部关系生成标题、摘要。
- 失败时回退到本地规则摘要。
- 摘要会写入 `rag_communities`，供 GraphRAG prompt 和知识图谱可视化页面展示。

### 服务启动接入

`cmd/rag-service` 现在会在 `RAG_ROUTER_PROVIDER=llm` 且存在可用 API 配置时启用：

- LLM Router
- LLM CRAG Evaluator
- LLM Self-RAG Judge
- LLM Graph Extractor
- LLM Graph Community Summarizer

GraphRAG LLM 默认复用 `RAG_ROUTER_*`，为空时回退 `LLM_DEFAULT_*`，避免重复配置一套小模型密钥。

## 配置说明

现有配置即可启用 GraphRAG LLM 抽取：

```env
RAG_ROUTER_PROVIDER=llm
RAG_ROUTER_BASE_URL=https://open.bigmodel.cn/api/paas/v4
RAG_ROUTER_API_KEY=
RAG_ROUTER_MODEL=glm-4-flash

LLM_DEFAULT_API_KEY=你的默认小模型密钥
LLM_DEFAULT_BASE_URL=https://open.bigmodel.cn/api/paas/v4
LLM_DEFAULT_MODEL=glm-4-flash
```

如果 `RAG_ROUTER_API_KEY` 为空，服务会使用 `LLM_DEFAULT_API_KEY`。

## 验收点

已补充单元测试覆盖：

- LLM JSON 解析、类型归一化、非法关系过滤、证据保留。
- `NewRAGServiceWithGraphExtractor` 注入抽取器和摘要器后，入库时使用 LLM 结果。
- 社区划分能把两个密集子图分成不同社区，并保留社区关键实体。
- settings-service 用户级 Router 启用时，不会丢掉 GraphRAG LLM 抽取器。

## 当前边界

- GraphRAG 是主流 RAG 的增强能力，不替代 Hybrid Search、RRF、Rerank、CRAG/Self-RAG。
- 社区划分是 Go 内部 Leiden 风格实现，不是外部 `leidenalg` 的严格实现。
- 图谱仍存 MySQL；可视化和 GraphRAG 查询共用 `rag_entities`、`rag_relations`、`rag_communities`。
- Text-to-SQL 仍只做路线识别和提示，没有 SQL schema 管理和只读执行器。
