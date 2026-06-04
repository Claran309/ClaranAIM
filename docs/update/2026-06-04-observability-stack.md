# 2026-06-04 可观测性一条龙接入

## 本次完成

- 新增 `pkg/observability`：
  - 每个服务启动独立 `/metrics` HTTP 端口。
  - 通过 OTLP HTTP 向 Collector 输出 trace。
  - 通过 OTLP HTTP 向 Collector 输出基础 metrics。
  - 本地 Prometheus 指标覆盖服务启动、依赖检查、HTTP、Kitex RPC 和业务事件。
- 所有 `cmd/*/main.go` 均接入 `observability.InitService`。
- api-gateway 新增 HTTP 中间件：
  - 记录 route、method、status、duration。
  - 为请求创建 OpenTelemetry span。
- Kitex client/server 统一 governance middleware：
  - 记录 RPC service、method、success/error、duration。
  - 为 RPC 创建 OpenTelemetry span。
- 关键业务指标接入：
  - Outbox publish success/retry/dead。
  - Kafka consumer success/failed/dead。
  - WebSocket online_connections。
  - RAG ingest/search duration。
  - Memory recall duration 和 recall_hits。
  - MCP tool call duration/status。
- 新增本地 Docker 开发栈：
  - `deployment/docker/observability/elk/docker-compose.yaml`
  - `deployment/docker/observability/otel/docker-compose.yaml`
  - Collector、Prometheus、Jaeger、Grafana、Elasticsearch、Kibana、Logstash、Filebeat 配置。
- 管理后台新增“可观测性”Tab：
  - Grafana 总览。
  - Jaeger Trace。
  - Kibana 日志。
  - Prometheus Targets。

## 启动方式

先启动 ELK：

```powershell
docker compose -f deployment/docker/observability/elk/docker-compose.yaml up -d
```

再启动 OTel / Prometheus / Jaeger / Grafana：

```powershell
docker compose -f deployment/docker/observability/otel/docker-compose.yaml up -d
```

然后启动 ClaranAIM 服务。Prometheus 默认通过 `host.docker.internal:<metrics_port>` 抓宿主机 Go 服务指标。

## 默认入口

- Grafana: `http://127.0.0.1:8086`
- Prometheus: `http://127.0.0.1:8084`
- Jaeger: `http://127.0.0.1:8085`
- Kibana: `http://127.0.0.1:5601`
- Elasticsearch: `http://127.0.0.1:9200`
- OTel HTTP: `http://127.0.0.1:8082`
- OTel gRPC: `127.0.0.1:8079`

## 服务 Metrics 端口

- api-gateway: `19080`
- websocket-gateway: `19081`
- user-service: `19101`
- group-service: `19002`
- msg-core-service: `19003`
- msg-history-service: `19004`
- file-service: `19005`
- agent-manager-service: `19006`
- agent-runtime-service: `19007`
- memory-service: `19008`
- settings-service: `19009`
- rag-service: `19112`
- knowledge-service: `19113`
- web-search-service: `19114`
- conversation-intelligence-service: `19015`
- mcp-gateway-service: `19016`
- admin-service: `19017`

## 验证方法

1. 启动任意服务后访问：

```powershell
curl http://127.0.0.1:19080/metrics
```

应能看到 `claran_` 前缀指标。

2. 打开 Prometheus Targets：

```text
http://127.0.0.1:8084/targets
```

已启动服务应为 `UP`，未启动服务为 `DOWN` 是正常状态。

3. 打开 Grafana：

```text
http://127.0.0.1:8086
```

默认账号为 `admin / admin123`，应自动出现 `ClaranAIM Overview` dashboard。

4. 发起一次前端请求后，在 Jaeger 搜索 `api-gateway`。

5. 服务产生日志后，在 Kibana 查询：

```text
service.name: api-gateway
```

## 当前边界

- 这是本地开发可用闭环，不是生产告警系统。
- 尚未接入 Alertmanager、告警规则、日志冷热分层和长期指标存储。
- Prometheus targets 依赖 Docker Desktop 的 `host.docker.internal`，Linux 原生环境可能需要改成宿主机 IP。
- 如果本机已占用 `8084/8085/8086/8082/8079/5601/9200/5044`，不要杀现有容器；复制 compose 后调整端口映射即可。
