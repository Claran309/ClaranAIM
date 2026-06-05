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

- Grafana: `http://127.0.0.1:8086`，默认账号 `admin / admin123`
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

## Grafana 使用说明

登录 `http://127.0.0.1:8086` 后，进入 Dashboards，打开 `ClaranAIM / ClaranAIM Overview`。

- `Service Starts`、`Max Service Uptime`、`HTTP QPS`、`HTTP Latency p95`、`Kitex RPC QPS`、`Dependency Checks` 和 `Prometheus Targets Up` 均来自 Prometheus datasource。
- `Jaeger Trace Explorer` 是 Jaeger 使用提示；真正查 trace 时进入左侧 Explore，Datasource 选择 `Jaeger`，Service 选择 `api-gateway`、`agent-runtime-service`、`rag-service` 等服务名。
- `Recent Service Logs` 使用 Elasticsearch datasource，索引为 `claran-services-*`，时间字段为 `@timestamp`。如果面板无数据，先确认 ELK 栈和 Filebeat/Logstash 已启动且本地 `logs/` 目录已有服务日志。
- Datasources 页面应能看到 `Prometheus`、`Jaeger`、`Elasticsearch` 三个自动 provision 的 datasource。Prometheus 指向 `http://prometheus:9090`，Jaeger 指向 `http://jaeger:16686`，Elasticsearch 通过 `http://host.docker.internal:9200` 访问 ELK 栈。

常见排查顺序：

1. Grafana 启动失败：先看 `docker compose -f deployment/docker/observability/otel/docker-compose.yaml logs grafana`，如果出现 `Datasource provisioning error`，优先检查 `deployment/docker/observability/config/grafana/provisioning/datasources/datasources.yaml`。
2. 指标为空：打开 `http://127.0.0.1:8084/targets`，确认对应 Go 服务的 `/metrics` target 为 `UP`。服务未启动时 target 为 `DOWN` 是正常的。
3. Trace 为空：确认服务配置了 OTLP HTTP endpoint `http://127.0.0.1:8082`，并实际发起过请求。
4. 日志为空：先查 Elasticsearch 是否已有 `claran-services-*` 索引，再查 Filebeat/Logstash 日志链路。

## Kibana 使用说明

登录 `http://127.0.0.1:5601` 后，首次使用需要创建 Data View：

1. 进入 Stack Management -> Data Views。
2. 创建 Data View，Name/Index pattern 填 `claran-services-*`。
3. Time field 选择 `@timestamp`。
4. 进入 Discover，选择刚创建的 `claran-services-*`。

常用 KQL 查询：

```text
service.name: api-gateway
```

```text
level: ERROR
```

```text
trace_id: "你的 trace id"
```

```text
service.name: agent-runtime-service and level: ERROR
```

如果 Discover 没有数据，先访问：

```powershell
curl http://127.0.0.1:9200/_cat/indices?v
```

确认是否存在 `claran-services-*` 索引。没有索引时，优先检查本地服务是否写入 `logs/`、Filebeat 是否挂载了项目 `logs` 目录、Logstash 是否连接到 Elasticsearch。

## 当前边界

- 这是本地开发可用闭环，不是生产告警系统。
- Grafana 镜像固定为 `grafana/grafana:11.5.2`，避免 `latest` 升级导致 datasource provisioning schema 变化。
- Elasticsearch datasource 使用 `jsonData.index: claran-services-*`，不再使用已废弃的顶层 `database` 字段。
- 尚未接入 Alertmanager、告警规则、日志冷热分层和长期指标存储。
- Prometheus targets 依赖 Docker Desktop 的 `host.docker.internal`，Linux 原生环境可能需要改成宿主机 IP。
- 如果本机已占用 `8084/8085/8086/8082/8079/5601/9200/5044`，不要杀现有容器；复制 compose 后调整端口映射即可。
