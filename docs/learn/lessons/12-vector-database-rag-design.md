# 第 12 课：向量数据库版 RAG 设计

## 学习目标

这一课设计未来 rag-service。你要掌握：

- 为什么 RAG 应独立成服务。
- `file.uploaded` 如何触发入库。
- chunk metadata 如何表达权限。
- 向量检索如何接回 Agent runtime。

## 源码入口

重点阅读：

- `cmd/rag-service`
- `internal/rag-service`
- `pkg/events/events.go`
- `internal/agent-runtime-service/graphTool/rag.go`
- `internal/file-service`
- `internal/msg-core-service/service/service.go`

## 为什么独立 rag-service

runtime 适合执行 Agent，不适合管理知识库全生命周期。

rag-service 应负责：

- 文档解析。
- chunk。
- embedding。
- 向量索引。
- 权限过滤。
- 引用管理。
- 入库任务。

runtime 只通过工具调用检索接口。

## 入库链路

```text
用户上传文件
  -> file-service 保存元数据
  -> 用户发送 file/image 消息
  -> msg-core-service 写 file.uploaded IM 事件
  -> rag-service 消费 claran.im.events
  -> 读取文件
  -> 解析文本
  -> chunk
  -> embedding
  -> 写向量库和元数据表
```

入库必须异步，不应阻塞发送消息。

## 表设计草案

`rag_documents`：

```text
id
file_id
source_msg_id
source_event_id
owner_id
conversation_id
group_id
title
content_type
visibility
status
```

`rag_chunks`：

```text
id
document_id
chunk_index
content
token_count
embedding_ref
metadata_json
```

`rag_ingest_jobs`：

```text
id
source_event_id
document_id
status
retry_count
last_error
```

## 权限 metadata

chunk 至少要带：

- owner_id。
- conversation_id。
- group_id。
- source_msg_id。
- file_id。
- visibility。
- created_by。

检索时不能只看向量相似度，还要看用户和 Agent 是否有权访问。

## 检索链路

```text
Agent 调用 RAG tool
  -> runtime 带 user_id / agent_user_id / conversation_id
  -> rag-service.SearchKnowledge
  -> metadata filter
  -> vector search
  -> rerank
  -> 返回 chunks + 引用
  -> Agent 组织回答
```

## 本课检查

你应该能回答：

- 为什么 file.uploaded 是 RAG 入库天然入口？
- 权限过滤为什么不能只靠 Prompt？
- rag-service 和 memory-service 有什么区别？
- 向量库里为什么要保留 metadata？

## 动手任务

1. 设计 `SearchKnowledge` 请求/响应。
2. 设计 `file.uploaded` 消费幂等。
3. 设计用户退群后的知识权限处理。

