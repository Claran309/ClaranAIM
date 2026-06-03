# 2026-06-03 RAG 上传、启动脚本与知识图谱稳定性修复

## 背景

本轮集中处理本地调试中暴露的几类问题：

- 启动脚本偶发 `SKIP`，但用户关闭可见终端后服务并未按预期重新启动。
- 大文件上传知识库时容易 `fail to fetch` 或 `rpc timeout`，并且前端看不出真实分块数量。
- 文档入库后只显示一个 chunk，单换行长文本、docx/OCR 文本容易被当成一个大段。
- Adaptive RAG 把部分可检索问题误判为直答。
- Agent 仍可能被自己写回的消息再次触发。
- 知识图谱实体和关系噪声过多，前端关系/类型显示不够业务化，画布空间被控制区挤压。
- Web Search Augmentation 在抓取网页失败或耗时过长时直接超时。

## 已完成

### 启动脚本

- 新增 `scripts/service-control.ps1`，统一实现 `start/status/stop/restart`。
- `scripts/start.bat`、`stop.bat`、`restart.bat`、`status.bat` 均转发到同一个 PowerShell 控制脚本。
- 端口检测改为检查所有本地监听地址，不再只看 `127.0.0.1`。
- `status` 会区分：
  - `RUNNING`：当前仓库启动的目标服务。
  - `BLOCKED`：端口被其他进程占用。
  - `STOPPED`：未监听。
- `stop` 只杀当前仓库对应服务进程，避免误杀其他程序。
- 修复 PowerShell `$PID` 只读变量冲突导致 `status` 崩溃的问题。

### RAG 上传与分块

- API Gateway 上传体上限提升到 96MB，RAG 文件单文件上限为 80MB。
- `/rag/upload` 改为异步任务接口，提交后立即返回 `job_id`。
- 新增 `GET /rag/upload/:id` 查询任务状态。
- 上传文件读取完成后，解析、切片、入库和图谱构建在后台 goroutine 中继续执行，用户切换页面不会取消任务。
- api-gateway 使用长任务 RAG RPC client，避免大文档入库仍被普通 RPC deadline 过早截断。
- RAG 入库时直接索引本次创建的 child chunks，不再通过 `ListChunks(limit=500)` 反查，避免大文档只部分写入向量索引。
- 单换行长文本会继续按长度切分，避免 docx/OCR/Markdown 长段被当成一个 chunk。
- 前端上传结果展示任务状态、每个文件状态、子区块数、实体数和关系数。

### Adaptive RAG 与检索结果

- 修正问候规则：`你好，介绍一下自己` 仍走 direct，但 `介绍一下 RAG/项目/文档对象` 不再被误判成问候。
- MCP `search_knowledge` 默认使用 `hybrid`，避免用户明确调用知识检索工具时又被 adaptive router 拒绝检索。
- RAG 检索页面继续展示最终回答、CRAG/Self-RAG 状态和命中来源。

### Web Search Augmentation

- Web Search 搜索源全部失败时返回降级提示，不再把整个工具调用打成硬失败。
- Augment 增加搜索和网页抓取预算，抓正文失败时使用搜索摘要作为兜底上下文。
- 处理慢网页时尽量在 RPC deadline 前返回可用结果。

### Agent 自回复

- `message.created` 事件 payload 增加 `client_msg_id`。
- msg-core-service 发布事件时带上消息的 `client_msg_id`。
- Agent 写回消息使用 `agent:*` 幂等键。
- Agent 事件分发器同时按 `client_msg_id/idempotency_key` 前缀和 `sender_id -> agent_user_id` 反查拦截 Agent 自己发出的消息。
- 补充测试覆盖“Agent 用户发出的普通消息即使没有 agent:* client_msg_id 也不会触发”。

### 知识图谱

- LLM 图谱抽取提示词收紧：只抽长期有价值的服务、表、Topic、API、模块、概念等实体。
- 过滤截图、页面、正文、文件名、图片名、页码等噪声实体。
- 没有明确关系时不再强行生成低质量 `RELATED_TO`。
- 关系过滤要求 source/target 有效、关系有说明或证据、置信度达到基础阈值。
- 规则抽取改为按动词触发点建边：`写入/调用/消费/发布/读取/依赖/配置/触发` 等分别产生有方向的关系。
- 中文省略主语场景会继承本句第一个主语，例如 `agent-manager-service 消费 Topic，写入表，并调用 runtime` 会生成三条正确方向关系。
- 关系按 `source/target/type/chunk` 去重，减少重复边。
- knowledgeclient 对图谱节点和边做稳定排序、去重，前端展示更有序。
- 前端类型和关系显示为中文：服务、数据表、事件主题、调用、写入、发布、消费等。
- 知识图谱页面改为画布优先布局，控制区压缩为顶部横向工具带，右侧详情面板更紧凑。

## 验证

已执行：

- `go test ./internal/rag-service/service`
- `go test ./internal/web-search-service/service`
- `go test ./internal/agent-manager-service/eventconsumer`
- `go test ./internal/msg-core-service/service`
- `go test ./pkg/knowledgeclient`
- `go test ./internal/mcp-gateway-service/service`
- `go test ./internal/api-gateway/handler ./internal/api-gateway/client ./cmd/api-gateway`
- `go test ./pkg/events ./pkg/ragclient ./pkg/websearchclient ./pkg/governance`
- `node --check dist/js/app.js`
- `node --check dist/js/api.js`
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\service-control.ps1 status`

## 当前边界

- 上传任务状态目前保存在 api-gateway 进程内存，网关重启后历史任务不可查；后续可落 MySQL 或 Redis。
- 知识图谱仍是 MySQL 图谱 MVP + Leiden-like 社区划分，不是 Neo4j/NebulaGraph 专用图数据库。
- LLM 图谱抽取质量取决于配置的小模型和输入文档质量，规则抽取已做降噪，但复杂跨段关系仍需要 LLM 抽取和候选审核配合。
- RAG 文档列表接口当前不携带历史文档 chunk 总数；本轮先保证上传任务完成时能看到真实 chunk/entity/relation 数。
