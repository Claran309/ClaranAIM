# 2026-06-01 RAG Service 与 GraphRAG 集成说明

## 本次目标

本次新增独立 `rag-service`，把知识库入库、Hybrid Search、GraphRAG 子图、CRAG 质量门、Self-RAG 检查点和 Adaptive RAG 路由统一放到服务端，并在前端增加“知识库 / 知识图谱”工作区。

## 已实现内容

- 新增 `idl/rag.thrift` 与 `kitex_gen/rag/*`，内部服务通过 Kitex RPC 调用 RAG 能力。
- 新增 `cmd/rag-service`、`internal/rag-service`、`pkg/ragclient`。
- 新增 RAG 表模型：
  - `rag_documents`
  - `rag_chunks`
  - `rag_entities`
  - `rag_relations`
  - `rag_communities`
- 入库链路：
  - 支持手动 JSON 文本入库，也支持前端上传知识库文件。
  - 上传文件由 api-gateway 解析为 UTF-8 文本后，通过 Kitex RPC 写入 `rag-service`。
  - 文档写入 MySQL。
  - 文档自动建立 parent/child 分层分块：检索只使用 child 小块，回答上下文返回 parent 摘要、parent 正文或命中的 child 摘录。
  - 优先使用 GLM `embedding-3` 生成向量；未配置或调用失败时降级为本地 hash embedding，保证入库链路不中断。
  - 只有 child 小块写入向量索引接口，parent 大块保存在 MySQL 作为上下文与摘要承载。
  - 文档内容抽取实体、关系和社区摘要，形成 GraphRAG MVP 图谱。
- 检索链路：
  - Adaptive Router 先判断问题路线；规则能确定的直接走规则，规则不确定的再交给 LLM classifier。
  - Hybrid Search 使用 child 小块做 Dense 向量召回 + BM25 稀疏召回两路结果。
  - 两路召回结果通过 RRF（Reciprocal Rank Fusion，默认 `k=60`）融合排名，避免手写线性权重把 dense 或 sparse 某一路压死。
  - RRF 结果按 `parent_chunk_id` 聚合去重，最终来源返回 parent chunk；`reason` 会保留 `child_chunk_id` 和 `parent_chunk_id` 便于追踪命中链路。
  - RRF 取 top30 后进入模型 Reranker，模型读取 query + parent context 重新给相关性分数，再返回最终 topK；Reranker 不可用时降级本地轻量精确匹配。
  - CRAG 在 rerank 后执行四维 evaluator：Relevance、Coverage、Specificity、Conflict，并输出 `correct`、`incorrect` 或 `ambiguous`。
  - Self-RAG 输出 `Retrieve`、`IsRel`、`IsSup`、`IsUse` 四个检查点。
  - GraphRAG 搜索会返回相关实体和关系。
  - Text-to-SQL 路由已能识别结构化问题，目前返回安全执行计划，不直接执行 SQL。
- 前端新增：
  - 侧栏“知识”入口。
  - 知识录入页面。
  - RAG 检索问答页面。
  - CRAG / Self-RAG 状态展示。
  - SVG 知识图谱可视化。
  - 图谱节点点击查看实体摘要和关系数量。
- Agentic RAG 入口：
  - `agent-runtime-service` 新增 `search_knowledge_base` 工具。
  - 工具通过 `pkg/ragclient` RPC 调用 `rag-service`。
  - 工具只允许模型传 `query/mode/limit`，当前用户和会话 ID 由 runtime 注入上下文，避免 Agent 伪造 `viewer_id` 绕过权限裁剪。
- 文件解析：
  - `txt` / `md` / `markdown`：按 UTF-8 文本解析，自动去除 BOM 并规范空行。
  - `go/js/ts/py/java/c/cpp/rs/sql/json/yaml` 等代码或配置文件：按 UTF-8 原文解析；其中 Go 已按声明结构切分，其他语言当前先走段落/长度兜底，后续可接 tree-sitter 或语言专用 planner。
  - `pdf`：优先使用本地轻量文本抽取；如果没有解析出文本，且 api-gateway 配置了 OCR provider，会调用 GLM-OCR 兜底解析扫描件。
  - `png/jpg/jpeg/webp/bmp/gif/tif/tiff`：配置 OCR provider 后可直接作为图片知识库文件上传并抽取文本。
  - `docx`：读取 `word/document.xml` 提取正文段落文本；复杂表格、批注、页眉页脚和图片 OCR 不在本轮范围。

## 文档 OCR 配置

上传知识库文件时，OCR 能力在 `api-gateway` 层启用。普通文本、Markdown、代码、可复制文本 PDF 和 docx 仍优先走本地解析；只有扫描件 PDF 本地解析失败，或上传图片文件时，才调用 OCR。

```powershell
DOCUMENT_OCR_PROVIDER=glm
DOCUMENT_OCR_URL=https://open.bigmodel.cn/api/paas/v4/layout_parsing
DOCUMENT_OCR_API_KEY=<本地私密配置>
DOCUMENT_OCR_MODEL=glm-ocr
```

实现说明：

- `pkg/documentparser.ParseWithOptions` 接收 `OCRProvider`，解析器本身不直接读取环境变量，避免测试或内部调用时意外访问外部网络。
- `api-gateway` 启动时根据 `DOCUMENT_OCR_*` 初始化 `GLMLayoutOCRProvider`。
- 上传扫描件 PDF 时：先尝试本地 PDF 文本流抽取，失败后再调用 OCR。
- 上传图片时：必须配置 OCR provider，否则返回“图片文件需要配置OCR服务后才能解析”。
- GLM-OCR 返回内容会优先抽取 `md_results`、`markdown`、`text`、`content` 等字段，统一规范化后写入 RAG。

## 分层索引与大小 Chunk

当前 RAG 分块已经从“扁平 chunk”改为 parent/child 两层：

```text
query
  -> 只搜索 child chunks
  -> Dense + BM25 分别排名
  -> RRF 融合排序
  -> 按 parent_chunk_id 聚合
  -> 返回 parent summary + parent content，长 parent 返回 parent summary + 命中 child 摘录
  -> 放进答案合成和 sources
```

分片策略：

- Markdown：`#` 作为文档标题；`##` 作为 parent chunk；`###`、段落和较短文本片段作为 child chunk。
- PDF / Word / 普通文本：先解析成规范化文本，再按段落聚合 parent；child 默认按更小文本窗口切分。标题识别不准时走段落 + 长度兜底。
- Go 代码：按 `package/import`、`type`、`interface`、`func`、`method` 等声明切 parent；函数前注释会跟随函数进入同一个 parent；超长函数再拆成较小 child。其他语言代码目前可上传入库，但先走通用文本分片。
- 聊天记录：按连续消息窗口建立 parent，当前默认约 20 条消息一个 parent，child 按 1-3 条消息的小窗口生成，适合先摘要再入库。

parent summary 当前是本地抽取摘要：优先取标题和首段，限制长度，避免上传时额外调用 LLM。后续更合理的做法是接入 settings-service 中的 `rag_router` 或专门的小模型预设，在异步任务里为 parent 生成更好的摘要，并把失败摘要任务放入重试队列，避免拖慢上传接口。

## Embedding 配置

RAG 服务支持通过环境变量切换 embedding provider。本地 `.env` 已配置为 GLM `embedding-3`，API Key 只保存在本地环境文件中，不写入仓库文档。

```powershell
RAG_EMBEDDING_PROVIDER=glm
RAG_EMBEDDING_URL=https://open.bigmodel.cn/api/paas/v4/embeddings
RAG_EMBEDDING_API_KEY=<本地私密配置>
RAG_EMBEDDING_MODEL=embedding-3
RAG_EMBEDDING_DIMENSION=0
```

说明：

- `RAG_EMBEDDING_DIMENSION` 是传给 GLM embedding 接口的 `dimensions` 参数，`0` 表示不传该字段，使用模型默认维度。
- `RAG_EMBEDDING_DIM` 是项目内部向量索引维度，默认 `256`；外部 embedding 返回维度与内部维度不一致时会截断或补零，以保证当前本地索引和后续 Milvus collection 维度稳定。
- `rag-service` 启动时如果 provider 不是 `glm`，或 API Key 为空，会自动使用本地 hash embedding。
- GLM 请求失败不会让用户上传知识库失败，服务会记录 warning 并使用 hash embedding 兜底。

## Rerank 配置

Rerank 已从本地关键词微调升级为模型精排。链路为：

```text
Hybrid Search(child dense + child BM25)
  -> RRF 融合
  -> parent 聚合
  -> top30 parent contexts
  -> GLM rerank 模型读取 query + documents
  -> 按 relevance_score 得到最终 topK
```

配置项：

```powershell
RAG_RERANK_PROVIDER=glm
RAG_RERANK_URL=https://open.bigmodel.cn/api/paas/v4/rerank
RAG_RERANK_API_KEY=<本地私密配置>
RAG_RERANK_MODEL=rerank
```

说明：

- `documents` 使用 parent context：短 parent 为 parent summary + parent content；长 parent 为 parent summary + 命中 child 摘录。
- rerank 返回的 `relevance_score` 会覆盖 RRF 分数用于最终排序，`sources.reason` 会追加 `model_rerank=...`。
- rerank 接口不可用、返回异常或未配置时，服务降级为本地 `local_rerank`，避免 RAG 搜索直接失败。

## Milvus 集成状态

项目已经新增 `VectorIndex` 接口，并提供两个实现：

- `LocalVectorIndex`：本地开发降级索引，保证不启动 Milvus 时也能跑通 RAG 流程和测试。
- `MilvusVectorIndex`：真实 Milvus 适配器，使用 `github.com/milvus-io/milvus-sdk-go/v2@v2.4.2`。

之前没有直接引入 `github.com/milvus-io/milvus/client/v2` 的原因：

- 本地缓存存在 `github.com/milvus-io/milvus/client/v2@v2.6.5`。
- 该 SDK 的 `go.mod` 要求 Go `1.25.8`。
- 当前项目 `go.mod` 是 Go `1.25.3`。
- 当前已改用兼容性更好的 `github.com/milvus-io/milvus-sdk-go/v2@v2.4.2`，不再依赖 `github.com/milvus-io/milvus/client/v2@v2.6.5`。

当前 `MilvusVectorIndex` 行为：

- 启动时连接 Milvus。
- collection 不存在时自动创建 `chunk_id`、`document_id`、`vector` 三列。
- 使用 `chunk_id` 作为主键，`vector` 为 FloatVector。
- 建立 `COSINE + FLAT` 索引。
- 入库时执行 Upsert + Flush。
- 搜索时返回 Milvus TopK 命中的 `chunk_id` 和相似度分数。
- 如果 Milvus 配置启用但容器不可用，rag-service 会记录 warning 并降级到 `LocalVectorIndex`，避免本地开发启动失败。

Milvus 容器启动命令：

```powershell
docker compose -f deployment/docker/milvus/docker-compose.milvus.yml up -d
```

服务配置项：

```yaml
milvus:
  enabled: true
  address: "127.0.0.1:19530"
  collection: "claran_rag_chunks"
```

环境变量覆盖：

```powershell
$env:MILVUS_ENABLED="true"
$env:MILVUS_ADDRESS="127.0.0.1:19530"
$env:MILVUS_COLLECTION="claran_rag_chunks"
```

## Adaptive RAG Router

Adaptive RAG 当前已从“mode 选择器”升级为 Router / Classifier。核心原则是先路由，再检索；LLM 只输出结构化判断，应用代码执行检索、工具或兜底。

输入：

- 用户问题。
- 当前会话/群上下文 ID。
- 可用知识源。
- 当前用户身份和权限边界。

输出结构：

```json
{
  "route": "project_rag",
  "complexity": "medium",
  "need_retrieve": true,
  "sources": ["project_docs", "code_chunks"],
  "strategy": "hybrid_rerank",
  "mode": "hybrid",
  "retrieval_source": "project_docs",
  "query": "agent_dispatch_records event_id agent_user_id",
  "reason": "问题依赖当前项目内部实现"
}
```

路线：

- `direct`：简单问题，不检索，直接回答。
- `project_rag`：普通项目/知识库问题，走项目文档或代码 chunk 检索。
- `strict_rag`：复杂项目问题或高风险问题，走 Hybrid + Rerank + CRAG + Self-RAG。
- `web_rag`：实时/最新问题。当前 rag-service 返回 Web RAG 路由提示，真正 Web Search 由 agent-runtime 工具链执行。
- `memory_rag`：私有记忆问题。当前 memory-service 是 MySQL 事实记忆 MVP，尚未接 RAG/Milvus，rag-service 返回 memory 路由提示。
- `tool_action`：执行动作，不走 RAG 检索，交给 Agent 工具审批/执行链路。

Router 实现是规则 + LLM 混合：

- 明显问候、最新价格/版本、当前项目/代码、私有记忆、动作请求、高风险问题，直接由规则分类。
- 规则不确定时才调用 LLM Router，降低成本，也避免纯规则过死。
- LLM Router 输出结构化 JSON，不能执行工具，不能绕过权限。
- Search 执行检索时仍使用当前 `viewer_id/group_id/conversation_id` 做服务端权限裁剪。

## CRAG Evaluator、Self-RAG 与 LLM Router

CRAG 当前链路：

```text
用户问题
  -> Router 判断需要 RAG
  -> Hybrid Search / Milvus child 检索
  -> RRF parent 聚合
  -> Model Reranker
  -> CRAG Evaluator
       ├─ correct   -> 内部资料回答
       ├─ incorrect -> Web/询问用户搜索兜底
       └─ ambiguous -> 内部 + 外部合并
  -> LLM/当前合成器生成
```

CRAG evaluator 评估四项：

- `Relevance`：资料是否和问题相关。
- `Coverage`：资料是否覆盖问题需要的关键点。
- `Specificity`：资料是否足够具体，而不是泛泛相关。
- `Conflict`：资料之间是否互相矛盾。

项目内置 CRAG evaluator 复用 RAG Router 的小模型配置：

- 优先读取 `RAG_ROUTER_API_KEY` / `RAG_ROUTER_BASE_URL` / `RAG_ROUTER_MODEL`。
- 为空时回退 `.env` 中当前默认 LLM：`LLM_DEFAULT_API_KEY` / `LLM_DEFAULT_BASE_URL` / `LLM_DEFAULT_MODEL`。
- 小模型不可用、返回格式异常或未配置时，降级 `RuleCRAGEvaluator`，仍会输出四维分数和 label。

模型输出格式：

```json
{
  "label": "ambiguous",
  "score": 0.56,
  "relevance": 0.72,
  "coverage": 0.45,
  "specificity": 0.62,
  "conflict": 0.10,
  "reason": "资料提到了 Agent 调度，但没有解释 event_id 和 agent_user_id 的业务含义"
}
```

接口兼容：

- `crag_action` 现在返回 `correct` / `incorrect` / `ambiguous`。
- `self_check.note` 会追加 CRAG 四维分数和 reason。
- 旧前端如果只做字符串展示不受影响；如果按旧值 `use_internal/web_fallback/merge_internal_and_web` 分支，需要同步兼容新 label。

Self-RAG 当前四个检查点：

- `Retrieve`：是否需要检索。由 Adaptive Router / Self-RAG Retrieve 判断器输出结构化 JSON：`route`、`complexity`、`retrieve`、`retrieval_source`、`sources`、`strategy`、`query`、`mode`、`reason`。
- `IsRel`：检索到的文档是否相关。由 Self-RAG judge 在 rerank + CRAG 后读取 query、sources 和 answer 判断；小模型不可用时按 CRAG 规则降级。
- `IsSup`：当前回答是否有文档支撑。由 Self-RAG judge 判断；小模型不可用时按 CRAG label 降级。
- `IsUse`：当前答案是否对用户有用。由 Self-RAG judge 判断；小模型不可用时按是否有来源和答案降级。

关键边界：

- Self-RAG 的 LLM 只是“判断器”，不会执行工具、访问 Milvus、访问 Web、访问数据库，也不能绕过权限。
- LLM 输出结构化判断，例如：

```json
{
  "retrieve": true,
  "retrieval_source": "project_docs",
  "query": "agent_dispatch_records event_id agent_user_id",
  "mode": "hybrid",
  "reason": "问题涉及项目内字段含义，需要查项目文档"
}
```

- 应用代码拿到判断后，用当前用户的 `viewer_id/group_id/conversation_id` 权限上下文执行 `hybridRetrieve`，确保工具调用和权限裁剪始终在服务端代码里。
- `self_check.note` 会记录 `retrieval_source`、实际用于检索的 `retrieval_query`、CRAG 四维分数和 Self-RAG judge 结果，便于前端和日志排查。

RAG Router 配置：

```powershell
RAG_ROUTER_PROVIDER=llm
RAG_ROUTER_BASE_URL=
RAG_ROUTER_API_KEY=
RAG_ROUTER_MODEL=glm-4-flash
```

说明：

- `RAG_ROUTER_PROVIDER=rule` 时只使用本地规则。
- `RAG_ROUTER_PROVIDER=llm` 时，会调用 OpenAI-compatible `/chat/completions`。
- 项目内置 Router 优先读取 `RAG_ROUTER_API_KEY` / `RAG_ROUTER_BASE_URL`；如果这两个值为空，会自动回退到 `.env` 中当前正在使用的 `LLM_DEFAULT_API_KEY` / `LLM_DEFAULT_BASE_URL`。
- Router 模型名优先使用 `RAG_ROUTER_MODEL`，为空时回退 `LLM_DEFAULT_MODEL`。
- 用户可以在“系统设置 - LLM 预设”中新增用途为 `rag_router` 的“RAG 路由小模型”预设，并设为默认；该用户执行 RAG 检索时会优先使用自己的小模型配置。
- 用户级 `rag_router` 预设不可用、调用失败或未配置时，会回退到项目内置 Router；项目内置 Router 也不可用时再降级为本地规则。
- router 小模型只返回 JSON：`retrieve`、`mode`、`reason`。
- 寒暄、闲聊、不需要项目知识的问题会走 `direct`，跳过向量检索。
- 需要文档事实、项目知识、关系推理的问题会进入 `hybrid`、`graphrag` 或 `text_to_sql`。
- router 调用失败不会阻断搜索，会自动降级为规则路由。

## API

所有 HTTP 接口由 `api-gateway` 暴露，内部通过 `rag-service` RPC 完成。`rag-service` 当前 Kitex 监听地址为 `127.0.0.1:9012`，避免和 `settings-service:9009` 冲突。

### 写入知识

`POST /api/v1/rag/ingest`

请求体：

```json
{
  "title": "项目RAG说明",
  "content": "知识正文",
  "source": "manual",
  "source_type": "markdown",
  "visibility": "private",
  "group_id": "0",
  "conversation_id": "0"
}
```

### 上传知识库文件

`POST /api/v1/rag/upload`

请求类型：`multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| file | file 或 file[] | 是 | 支持 `txt`、`md`、`markdown`、`pdf`、`docx`，单个文件最大 20MB |
| title | string | 否 | 文档标题；为空时使用文件名 |
| visibility | string | 否 | `private` / `group` / `public`，默认 `private` |
| group_id | int64 | 否 | 群知识可见范围 |
| conversation_id | int64 | 否 | 会话知识可见范围 |

返回示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "files": [
      {
        "success": true,
        "file_name": "project.md",
        "chunk_count": 3,
        "entity_count": 8,
        "relation_count": 7,
        "msg": "写入成功"
      }
    ]
  }
}
```

多文件上传时按文件返回结果；单个文件解析失败不会阻断其他文件。

### RAG 检索

`POST /api/v1/rag/search`

请求体：

```json
{
  "query": "Agentic RAG 为什么需要 Hybrid Search？",
  "mode": "adaptive",
  "limit": "8",
  "group_id": "0",
  "conversation_id": "0"
}
```

返回包含：

- `answer`
- `sources`
- `graph_nodes`
- `graph_edges`
- `route`
- `crag_action`
- `self_check`

### 知识图谱

`GET /api/v1/rag/graph?query=&limit=80`

返回：

- `nodes`
- `edges`
- `communities`

### 文档列表

`GET /api/v1/rag/documents?limit=20&offset=0`

## RAG 策略选择

本轮采用组合方案：

- 默认走 Adaptive RAG，先降低不必要 LLM/检索调用。
- 文档类问题走 Dense + BM25 双路召回，并使用 RRF 融合结果；后置模型 rerank 用 query + chunk 重新计算相关性，输出最终 topK。
- 关系、多跳、影响分析走 GraphRAG。
- 结构化统计问题走 Text-to-SQL 路由，但暂不执行 SQL。
- 检索质量不足、覆盖不足、资料过泛或资料冲突时由 CRAG evaluator 给出兜底/合并建议。
- Self-RAG 检查点用于前端和 Agent 后续判断答案质量。

## 当前边界

- GraphRAG 目前是启发式实体/关系抽取，不是 LLM 抽取。
- 社区划分目前是轻量规则聚类，不是 Leiden 算法。
- Hybrid Search 当前已是 Dense + BM25 + RRF；BM25 为本地实现，暂未接 Elasticsearch/OpenSearch/Lucene。
- Reranking 已接入 GLM rerank provider；未配置或调用失败时才使用本地轻量 tie-breaker。
- Self-RAG 的 `Retrieve`、`IsRel`、`IsSup`、`IsUse` 已支持小模型结构化判断，并保留规则降级路径。
- CRAG 当前已接入 LLM evaluator，并保留本地规则 evaluator 作为降级路径。
- Text-to-SQL 当前只识别路线和返回计划，不绑定真实只读 SQL 执行器。
- Agentic RAG 当前已经有 `search_knowledge_base` 工具入口，Agent 可自主检索知识库并结合结果回答；后续还需要让 Agent 能基于检索质量自动改写 query、多轮调用 RAG/WebSearch、沉淀知识候选。

这些边界是为了先保证服务、接口、前端和测试都能稳定跑通，再逐步替换为真实模型和 Milvus SDK。

## 后续建议

1. 在本地启动 Milvus 后，用真实容器跑一组端到端入库和检索验证。
2. 为 Reranker 增加用户级 settings-service 预设、调用限流和成本审计。
3. 用 LLM 抽取实体/关系，并实现 Leiden 社区划分。
4. 为 Self-RAG judge 增加用户级模型预设、成本统计、缓存和限流，降低重复判断成本。
5. 强化 `search_knowledge_base` 工具策略，让 Agent 能自动改写 query、切换 Hybrid/GraphRAG/Text-to-SQL 路线，并记录工具调用审计。
6. 给 Text-to-SQL 增加数据源 schema 注册、只读 SQL 生成、SQL 审计和执行沙箱。

## 验证记录

已执行：

```powershell
$env:GOCACHE='D:\CodeStudy\GoProjects\src\ClaranAIM\.gocache'
go test ./internal/rag-service/... ./pkg/ragclient ./internal/api-gateway/handler
node --check dist/js/api.js
node --check dist/js/app.js
```

结果：上述命令均通过。
