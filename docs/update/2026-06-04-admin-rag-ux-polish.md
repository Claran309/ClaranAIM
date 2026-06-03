# 2026-06-04 管理台与 RAG 交互体验修复

## 背景

本次修改集中处理管理面板可用性和知识库/RAG 前端体验问题：

- 管理面板位置偏移、视觉粗糙、功能入口不清晰。
- 用户治理、媒体预览、成本看板缺少足够的操作反馈和可视化。
- 知识录入上传/解析阶段前端闪烁，且大文件上传时缺少进度感知。
- Adaptive RAG 检索时状态反馈不足，命中来源卡片空白过大。

## 已完成修改

### 管理台布局

- 将管理台改成独立的宽屏治理控制台布局，避免继续套用欢迎页居中布局导致偏移。
- 新增管理台顶部 Hero 区、治理入口区、指标总览区和可滚动内容区。
- 调整管理台在窄屏下的响应式规则，防止表格和成本图挤出容器。
- 统一管理台内按钮、表格、筛选条、卡片和媒体预览卡的边框、阴影、hover 动效和密度。

### 用户与群聊治理

- 用户列表支持按关键词、状态、角色筛选。
- 用户封禁/解封按钮接入 `user-service` 的 `UpdateStatus` RPC，经由 `api-gateway` 的 `/admin/users/:id/status` 路由调用。
- 群聊治理页支持搜索、复制群号，并明确提示群聊封禁当前未启用。

说明：群聊封禁没有伪装成可用能力。当前 `group-service` 的 `groups` 模型和 `group.thrift` 没有可逆 `status` 字段，`msg-core-service` 发送链路也没有群封禁拦截。要真正支持群封禁，需要后续补齐 group 状态模型、IDL/RPC、admin-service 写入、msg-core 发送拦截和前端状态展示。

### 媒体与成本看板

- 媒体管理支持图片预览、音频播放、普通文件预览/下载入口。
- 修复媒体页筛选值在请求前被清空导致筛选无效的问题。
- 成本页增加当前页成本摘要、近几日柱状图、模型占比圆环图。
- 成本调用明细默认折叠，减少表格对主视觉区域的挤占。

### RAG 上传与录入体验

- RAG 文件上传从 `fetch` 改为 XHR，以便获取上传进度。
- 上传阶段新增进度卡，显示上传百分比、文件大小、文件数量和用时。
- 后台解析阶段继续显示任务状态、完成数、失败数、chunk 数、实体数、关系数和更新时间。
- 右下角知识入库浮动气泡支持上传中、解析中、完成、失败状态。
- 临时上传任务不会持久化到本地任务队列，避免刷新后轮询不存在的 `upload-*` 任务。
- 知识录入主结果区采用局部 DOM 更新，减少上传/轮询阶段闪烁。

### RAG 检索体验

- Adaptive RAG / 文档检索 / 混合检索 / GraphRAG 加载态增加阶段提示、用时和超时提示。
- 检索结果保留 AI 总结区，并将命中来源改为紧凑可展开卡片。
- 命中来源显示文档 ID、Chunk ID、分数、原因和折叠原文，减少每栏大量空白。

## 验证

已执行：

```powershell
node --check dist\js\app.js
node --check dist\js\api.js
go test ./internal/api-gateway/handler ./internal/user-service/service ./internal/conversation-intelligence-service/service ./internal/rag-service/service ./internal/agent-manager-service/service
```

结果：全部通过。

## 后续建议

- 真正实现群聊封禁：补齐 group status 数据模型、RPC、admin-service、msg-core 发送拦截和审计记录。
- 为 RAG 上传任务补服务端更细粒度进度：解析、分片、Embedding、Milvus 写入、GraphRAG 抽取分别上报。
- 管理台继续补媒体审核、Agent 审计详情、成本按时间范围筛选和导出。
