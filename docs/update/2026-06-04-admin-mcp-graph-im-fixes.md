# 2026-06-04 管理台、MCP、GraphRAG 与 IM 前端修复记录

## 修复范围

- 系统公告：新增登录后全局可见的右上角公告铃铛，普通用户可读取最近系统公告，管理员仍在治理台维护公告。
- 群封禁：补齐 group-service、admin-service、api-gateway 和 msg-core-service 的状态链路；被封禁群聊会拒绝继续发消息。
- 管理台媒体：图片、视频、音频在媒体列表中直接展示预览；媒体类型过滤兼容 `file_type` 和 `content_type`。
- 管理台成本：柱状图、圆环图和模型列表增加 hover 数据提示，能看到日期、模型、调用量和成本。
- MCP：`web_search` 使用长超时 RPC 客户端，默认限制抓取规模；`summarize_conversation` 优先使用当前 Agent 运行上下文中的 `conversation_id`，避免模型传错 ID。
- Agent 自回复：事件分发器会跳过 Agent 自己生成的消息、带 Agent metadata 的消息和可识别的 Agent echo，避免持续引用并回复自己。
- 翻译错误：第三方模型限流或鉴权失败时返回 provider、model、base_url 和错误链路，用户能看出是哪一个外部模型出问题。
- 会话串消息：前端渲染当前消息时再次按 `conversation_id` 过滤，避免 WebSocket 或缓存乱序导致其他会话消息显示到当前窗口。
- RAG 上传：上传气泡支持拖拽和位置保存；上传/解析状态保存到本地，离开页面后再进入会恢复任务并继续轮询。
- Knowledge Graph：空图时后端返回可解释诊断信息，knowledge-service 保留该信息，前端画布直接展示原因。

## GraphRAG 质量补强

- 会话摘要、聊天归档、conversation topic 继续禁止进入知识图谱。
- 配置了 LLM 抽取器时仍优先使用模型抽取，避免规则噪声覆盖模型判断。
- 大文档模型抽取全空时启用克制的主题骨架兜底，避免 601+ chunk 文档完全没有图谱。
- 图谱查询按文档范围为空时会提示：
  - 文档没有分块；
  - 文档类型不进入图谱；
  - 有分块但实体/关系未通过质量过滤；
  - 已抽实体但缺少有效关系。

## 已验证命令

```powershell
node --check dist\js\app.js
node --check dist\js\api.js
go test ./internal/group-service/... ./internal/admin-service/... ./internal/api-gateway/handler ./internal/msg-core-service/service ./internal/mcp-gateway-service/service
go test ./internal/rag-service/service ./internal/knowledge-service/... ./internal/agent-manager-service/eventconsumer ./pkg/knowledgeclient
go test ./cmd/mcp-gateway-service ./cmd/group-service ./cmd/admin-service ./cmd/api-gateway
```

## 剩余注意

- GraphRAG 的最终质量仍依赖 Router/GraphRAG 小模型配置是否可用；如果模型 Key 或 BaseURL 不通，系统会显示诊断而不是静默失败。
- API Gateway 自身限流没有完全关闭；本次主要把第三方翻译模型限流链路解释清楚。
- 本次没有主动停止、占用或切换用户正在使用的端口。
