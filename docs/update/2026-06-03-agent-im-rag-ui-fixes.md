# 2026-06-03 Agent / RAG / 前端体验修复

## 背景

本次修复集中处理 Agent 原生 IM 使用中的几个实际问题：Agent 回复自己的消息、图片进入会话后 OCR 能力不可见、RAG 上传错误提示不清晰、知识库清空按钮无效、Skill/记忆/联网搜索浮窗可用性差，以及 Text-to-SQL 功能残留。

## 已完成

- Agent 事件调度会忽略任意 Agent 系统用户发出的事件，避免多个 Agent 或同一 Agent 对自己的回复继续触发。
- Agent 运行摘要改为只提取“当前触发内容”，并在写入前检查已有记忆，避免重复生成相同的 system prompt 摘要。
- 图片/文件媒体消息 payload 改为兼容 JSON 格式，携带 `content_type` 和 `size`；旧的 `url|id|name` 格式仍可解析。
- msg-core-service 修复 `[img]...[/img]` / `[file]...[/file]` / `[voice]...[/voice]` 包裹内容解析，确保 file.uploaded 事件能得到真实 `file_id`、文件名和 MIME。
- Agent 附件上下文中，图片类附件会提示可以使用图片识别能力，避免只把 OCR 藏在 RAG 上传链路里。
- API Gateway 新增 `POST /api/v1/file/:id/ocr`，复用文件元数据和对象存储读取逻辑，对图片调用 GLM-OCR 并返回文本。
- 聊天图片消息增加“识别图片”按钮，可直接调用 OCR 并展示 Markdown 结果。
- OCR 429、鉴权失败、文件过大等错误改为面向用户的提示，不再只显示原始状态码。
- RAG 上传清空按钮现在会清空标题、来源、内容、文件选择、可见性、类型和结果区。
- 删除 Text-to-SQL 的前端入口、工具 schema 和当前 plan 状态表述；结构化统计问题改走知识库混合检索。
- Agent 会话上下文侧边栏重做为状态、会话 Agent、最近上下文和动作区，可读性更好。
- Skill 管理改为 Skill 中心工作台页面，支持上传、编辑、列表展示，不再塞进小浮窗。
- 记忆管理入口改为进入记忆中心工作台，正式记忆和候选记忆使用完整页面管理。
- 联网搜索改为知识工作台下的独立页面，展示搜索、增强上下文和来源，不再使用小浮窗。

## 验证

- `node --check dist/js/app.js`
- `node --check dist/js/api.js`
- `go test ./internal/msg-core-service/service -run "TestSendMediaMessagePublishesAgentNativeIMEvent|TestSendImageMessageParsesWrappedJSONAttachment|TestSendVoiceMessagePublishesVoiceTranscribedEventEnvelope"`
- `go test ./internal/api-gateway/handler ./internal/agent-manager-service/eventconsumer ./internal/agent-manager-service/service ./pkg/documentparser ./internal/rag-service/service`

## 仍需后续补齐

- Agent 自动调用图片 OCR 的完整工具闭环，建议下一步给 file-service 增加受权限控制的文件字节读取 RPC，再由 MCP Gateway 暴露图片识别工具。
- 需要浏览器实测 Skill 中心、记忆中心、联网搜索页和上下文侧边栏的响应式表现。
- OCR 结果目前是一次性识别展示，是否自动写入知识库或候选记忆仍应由用户或 Agent 策略决定。
