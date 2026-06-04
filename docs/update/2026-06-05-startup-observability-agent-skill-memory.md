# 2026-06-05 主页面、可观测性、Agent Skill 与 Memory 修复

## 本次修复

- 主页面进入不再串行等待群组、Agent 缓存、会话、好友、上线同步和工作台渲染；这些任务改为后台并发加载，慢服务只影响对应局部区域。
- Prometheus compose 增加 `host.docker.internal:host-gateway`，解决 Docker 内 Prometheus 无法解析宿主机 metrics 地址导致 Grafana No Data 的问题。
- 如果 Prometheus 容器是在本次修改前创建的，需要用户主动重建该容器后才会应用新的 `extra_hosts`；本次修复不会自动停用或替换用户正在使用的容器。
- 管理后台可观测性页删除本地启动命令块，改为 Grafana、Prometheus、Jaeger、Kibana 的使用提示；Kibana Data View 固定提示为 `claran-services-*` 和 `@timestamp`。
- Agent 私聊自回声增加入口层和 resolve bot 后的双重静默审计，静默原因统一写 `silent_agent_echo`，日志包含 sender、conversation、participants、client_msg_id 和 metadata。
- Agent 编辑页新增 Skill smoke test，复用真实 Agent 运行链路，最长等待 10 分钟，并显示 marker、诊断和模型原始回复。
- Agent run 后写长期 memory 的规则收紧：普通会话摘要、检索结果、临时上下文不再入库；只有用户明确要求记住的偏好、学习状态、长期目标和稳定项目事实才沉淀。

## 验收结果

- `node --check dist/js/app.js`
- `node --check dist/js/api.js`
- `docker compose -f deployment/docker/observability/otel/docker-compose.yaml config`
- `go test ./internal/agent-manager-service/eventconsumer ./internal/agent-manager-service/... ./internal/api-gateway/handler`
- `go test ./internal/agent-runtime-service/... ./pkg/observability ./pkg/knowledgeclient/... ./internal/rag-service/...`
- `go test ./internal/api-gateway/handler ./pkg/governance ./pkg/config`

以上检查均已通过。
