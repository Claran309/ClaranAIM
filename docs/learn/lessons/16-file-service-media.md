# 第 16 课：File Service 与多媒体消息

## 学习目标

这一课专题学习文件服务。你要掌握：

- 为什么文件二进制不走普通 RPC。
- file-service 为什么只保存元数据。
- api-gateway 在文件上传中承担什么职责。
- 图片、文件、语音消息如何进入 msg-core-service。
- `file.uploaded` 和 `voice.transcribed` 对 Agent/RAG 的意义。

## 源码入口

重点阅读：

- `internal/api-gateway/handler/file_handler.go`
- `internal/file-service/model/model.go`
- `internal/file-service/dao/dao.go`
- `internal/file-service/service/service.go`
- `internal/file-service/handler/handler.go`
- `internal/msg-core-service/service/service.go`
- `docs/APIdoc.md`

## 文件链路

当前文件上传链路：

```text
浏览器 multipart 上传
  -> api-gateway FileHandler.UploadFile
  -> 写本地 storage/source 或 MinIO
  -> 调 file-service 保存元数据
  -> 返回 file_id / url / name
  -> 前端把文件引用作为消息 content 发给 msg-core-service
```

file-service 只保存元数据：

- 文件 ID。
- 文件名。
- 文件类型。
- 文件大小。
- content-type。
- URL。
- 上传者。

文件二进制在本地目录或 MinIO。

## 为什么不把文件放消息表

消息表应该服务高频查询和分页。文件二进制会带来：

- 表膨胀。
- 查询变慢。
- 备份困难。
- 冷热分层困难。
- RPC 序列化开销大。

所以正确边界是：

```text
file-service = 元数据
对象存储 = 二进制
msg-core = 消息引用
```

## 消息 content 格式

推荐格式是 JSON：

```json
{
  "id": "file_id",
  "url": "file_url",
  "name": "report.pdf"
}
```

旧格式仍兼容：

```text
[img]...[/img]
[file]...[/file]
[voice]...[/voice]
```

## Agent-native 事件

当 msg-core-service 看到消息类型：

```text
image/file -> file.uploaded
voice      -> voice.transcribed
```

它会写统一 IM 事件 outbox。

未来可以触发：

- RAG 入库。
- 文件总结。
- OCR。
- ASR。
- 知识卡片。
- Action Card：是否加入知识库。

## 本课检查

你应该能回答：

- 为什么文件上传在网关层处理？
- file-service 为什么不存消息？
- msg-core-service 为什么只保存文件引用？
- `file.uploaded` 对 RAG 有什么价值？

## 动手任务

1. 追踪 `/file/upload`。
2. 追踪文件消息发送。
3. 设计一个 PDF 入库事件处理流程。
4. 设计文件删除后 RAG 文档失效策略。

