# 2026-06-04 RAG、管理台、记忆与知识图谱体验修复

## 修改范围

- 修复 RAG 上传任务前端状态归一化问题，右下角知识入库气泡现在会展示任务状态、耗时、完成数和进度条。
- 修复 RAG 检索结果字段兼容问题，前端同时兼容 Kitex JSON 的小写字段和大写字段，避免误显示“没有生成模型总结”。
- 强化 RAG 检索结果展示：保留 AI 总结区、检索诊断、紧凑来源卡片、可折叠原文和命中元信息。
- 修复默认 GraphRAG 规则抽取为空时不触发兜底图谱的问题。默认规则抽取为空会生成有限的文档主题骨架；配置 LLM 抽取器时仍尊重 LLM 抽取结果，避免规则噪声覆盖。
- 按文档查看知识图谱时，如果关系为空会触发一次重建，避免旧数据或抽取失败导致文档长期无图谱。
- 收紧 Agent 运行摘要入长期记忆的策略：短期闲聊、测试、寒暄不再写正式长期记忆；只有带长期价值、项目状态、偏好、目标、配置等信号的内容才会入库。
- 统一了全局 `btn-primary`、`btn-secondary`、`btn-small` 的基础视觉样式，并补齐之前缺失的 `btn-secondary` 样式。

## 关键修复点

### RAG 上传进度

上传任务现在不只依赖当前页面的结果区域，任务会进入右下角浮动气泡并持久化到本地。轮询失败时不会直接消失，而是显示失败原因并继续重试；完成或失败后保留结果，用户可以手动关闭。

### RAG 检索总结

前端原先只读 `answer/sources/self_check` 小写字段。如果后端经 Kitex/JSON 返回 `Answer/Sources/SelfCheck`，页面会误判没有模型总结。现在统一通过字段兼容读取，检索后会优先展示生成答案，并保留来源折叠区。

### GraphRAG 生成

默认服务构造时会配置规则抽取器，之前代码把它误认为“用户已配置 LLM 抽取器”，导致兜底图谱不会触发。现在新增 `llmGraphEnabled` 标志，仅在真实配置 LLM 抽取器时禁用兜底。

### 记忆治理

`agent-manager-service` 自己写入运行摘要的路径新增长期价值过滤，避免“所有聊天内容都进记忆”。短期或低价值内容仍可在当前会话上下文临时使用，但不会沉淀为长期记忆。

## 验证

已执行：

```powershell
node --check dist\js\app.js
node --check dist\js\api.js
go test ./internal/agent-manager-service/service ./internal/rag-service/service ./internal/api-gateway/handler ./internal/user-service/service ./internal/conversation-intelligence-service/service
```

结果均通过。

