# 2026-06-03 知识图谱抽取质量治理

## 背景

本轮继续排查知识图谱实体抽取和关系划分不合理的问题。重点不是增加新能力，而是减少噪声节点和错误边，让 GraphRAG 图谱更接近“可解释的项目结构”。

## 已完成

- 实体过滤增强：
  - 过滤 `event_id`、`client_msg_id`、`status`、`created_at` 等元数据字段。
  - 过滤 `pending`、`completed`、`failed` 等状态枚举值。
  - 过滤源码文件名、Markdown/JSON/YAML/图片/PDF/docx 等文件名。
  - 过滤 `在线用户`、`当前用户`、`文件内容`、`图片内容` 等弱业务泛词。
  - 过滤带关系词前缀的中文误抽取片段，例如 `也叫消息核心服务`。

- 实体类型推断增强：
  - `*-gateway` 现在识别为 `Service`，例如 `websocket-gateway`。
  - 避免把字段名误判为 `DatabaseTable`。

- 关系划分增强：
  - 英文关系词按词边界匹配，避免 `readiness`、`thread_local_state` 误触发 `READS`。
  - 同一句多个动词触发点会分别建边。
  - 中文省略主语会继承本句第一个主语，例如 `agent-manager-service 消费 Topic，写入表，并调用 runtime`。
  - 增加实体类型策略过滤不合理边：
    - 数据表不能 `CALLS` 服务。
    - 事件 Topic 不能 `WRITES` 数据表。
    - 数据表不能 `OWNS` 事件 Topic。
    - 服务/API/模块/产品才允许发起 `CALLS`、`WRITES`、`READS`、`CONSUMES` 等动作型关系。

- 回归测试增强：
  - 覆盖字段/状态/文件名不进入图谱。
  - 覆盖英文关系词边界。
  - 覆盖不合理类型边过滤。
  - 覆盖正常服务写表、服务消费 Topic、服务调用服务仍可保留。

## 验证

已执行：

- `go test ./internal/rag-service/service`
- `go test ./pkg/knowledgeclient ./internal/knowledge-service/service`
- `node --check dist/js/app.js`
- `node --check dist/js/api.js`

## 当前边界

- 规则抽取已尽量“少而准”，复杂跨段关系仍建议依赖 LLM 抽取器和图谱候选审核。
- 当前仍是 MySQL 图谱 MVP + Leiden-like 社区划分，不是专用图数据库。
- 对真实项目文档，仍建议通过前端图谱审核入口人工确认关键实体和关系。
