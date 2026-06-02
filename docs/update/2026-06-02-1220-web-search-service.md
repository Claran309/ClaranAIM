# Change Note: Web Search Augmentation Service

## 1. What Changed

本次新增独立 `web-search-service`，用于一次请求内的轻量联网搜索增强。

它做的事情是：

- 搜索候选网页。
- 优先排序官方或高可信域名。
- 抓取网页正文。
- 清洗 HTML、脚本、样式和多余空白。
- 从正文里截取和 query 相关的段落。
- 返回 `answer_context` 和来源列表，供 Agent 或调用方交给 LLM。

它刻意不做这些事：

- 不接 Milvus。
- 不做 embedding。
- 不做 chunk 入库。
- 不建立长期网页索引。
- 不把网页内容写进知识库或长期记忆。

所以它更准确的名字是 `Web Search Augmentation`，不是完整 Web RAG。

## 2. Why This Change Was Needed

旧的联网搜索能力放在 Agent 的 `graphTool/websearch.go` 里，而且 agent-manager 和 agent-runtime 各有一份相似代码。这有两个问题：

- 搜索、抓取、清洗这些基础能力被塞进 Agent 内部，其他服务和网关复用不了。
- Agent 工具代码同时承担搜索源、网页解析和 LLM 摘要编排，边界不清晰。

新的设计把“外部网页临时检索”沉到独立 RPC 服务中。Agent runtime 只保留一个工具入口，调用 `pkg/websearchclient`，由 web-search-service 负责搜索增强。这样以后替换搜索源、增加可信域名、做频率限制或审计时，不需要改 Agent 主流程。

## 3. Files Changed

- `idl/web_search.thrift`: 新增 web-search-service 的 Kitex RPC 契约，包含 `Search` 和 `Augment`。
- `kitex_gen/web_search/`: 由 Kitex 生成的 RPC 代码。
- `internal/web-search-service/service/service.go`: 实现搜索增强主链路、HTML 清洗、相关段落抽取、可信域名排序和默认搜索/抓取实现。
- `internal/web-search-service/service/service_test.go`: 用 TDD 覆盖可信源优先、正文清洗、相关段落截取和抓取失败摘要兜底。
- `internal/web-search-service/handler/handler.go`: Kitex handler，负责 RPC DTO 和业务 DTO 转换。
- `cmd/web-search-service/main.go`: 启动独立 Kitex 服务，注册到 Etcd。
- `config/web-search-service.yaml`: 新服务配置，默认端口 `127.0.0.1:9014`。
- `pkg/websearchclient/types.go`: 跨服务稳定客户端契约。
- `pkg/websearchclient/rpc_client.go`: Kitex RPC client wrapper。
- `internal/api-gateway/client/rpc_client.go`: 新增 `WebSearchClient`。
- `internal/api-gateway/handler/web_search_handler.go`: 新增浏览器 HTTP 门面。
- `internal/api-gateway/router/router.go`: 新增 `/web-search/search` 和 `/web-search/augment`。
- `cmd/api-gateway/main.go`: 初始化 web-search-service RPC client。
- `internal/agent-runtime-service/logic/web_search_tool.go`: 新增 Agent `web_search` 工具逻辑，通过 RPC 调用 web-search-service。
- `internal/agent-runtime-service/agent/tools.go`: 将 runtime 的 `web_search` 工具切到新 RPC 工具，避免继续使用本地 graphTool 复制逻辑。
- `cmd/agent-runtime-service/main.go`: 启动时连接 web-search-service。
- `pkg/config/config.go`: 新增 `WebSearchConfig` 和 `WEB_SEARCH_*` 环境变量覆盖。
- `.env.example`: 增加 Web Search Augmentation 配置示例。
- `scripts/start.bat`: 本地启动顺序增加 web-search-service。
- `README.md`: 补充项目结构、服务边界和 Adaptive RAG 中的 web_rag 状态。
- `docs/plan.md`: 标记 Web Search Augmentation 已落地，并把“真实 Web RAG 闭环”调整为轻量搜索增强能力。

## 4. Core Flow After The Change

### 浏览器调试入口

```text
浏览器
  -> api-gateway /api/v1/web-search/augment
  -> pkg/websearchclient.RPCClient
  -> Kitex web-search-service.Augment
  -> 搜索候选网页
  -> 可信源排序
  -> 抓取网页正文
  -> 清洗正文
  -> 截取相关段落
  -> 返回 answer_context + sources
```

### Agent 工具入口

```text
Agent runtime
  -> Eino tool: web_search
  -> logic.SearchWeb
  -> pkg/websearchclient.RPCClient
  -> web-search-service.Augment
  -> 返回临时网页上下文
  -> Agent 基于来源回答
```

### 搜索增强内部链路

```text
query
  -> SearchProvider.Search
  -> rankResults：可信域名 + 关键词相关性
  -> PageFetcher.Fetch
  -> CleanWebText
  -> ExtractRelevantPassages
  -> BuildAnswerContext
```

## 5. Key Code Concepts

### Web Search Augmentation

这是一次请求内的增强检索。它的输出是临时上下文，不是知识库事实。这样可以低成本回答实时或外部资料问题，同时不污染内部 RAG 和 Memory。

### SearchProvider

`SearchProvider` 是搜索源接口。默认实现按顺序尝试：

- DuckDuckGo HTML
- SearxNG JSON
- Wikipedia API

如果以后要接 Bing、Google Programmable Search、自建 SearxNG 或付费搜索 API，只需要替换这个接口。

### PageFetcher

`PageFetcher` 负责抓网页正文。当前默认实现只读取有限长度，避免一次请求抓超大网页。抓取失败不会让整个 Augment 失败；服务会使用搜索摘要作为 `snippet_fallback`。

### Trusted Domains

`trusted_domains` 是可信源提示，不是绝对真理。命中可信域名的页面会获得排序加权，并在 `answer_context` 中标记“命中可信/官方域名”。最终仍需要 LLM 根据内容和用户问题判断是否采用。

### Relevant Passage Extraction

当前使用轻量词项重叠抽段，不调用 embedding，也不调用 reranker。这样成本低、速度快。测试里明确要求：抓取成功但正文没有相关段落时，不把无关首段硬塞进上下文；只有抓取失败时才用搜索摘要兜底。

## 6. Important Implementation Details

- `web-search-service` 默认端口是 `9014`。
- api-gateway 新增接口：
  - `GET /api/v1/web-search/search?query=&limit=`
  - `POST /api/v1/web-search/augment`
- `Augment` 请求体示例：

```json
{
  "query": "轻量 Web RAG 怎么做",
  "limit": 5,
  "max_fetch": 5,
  "max_passages": 3
}
```

- `answer_context` 开头会提醒 LLM：这些是一次性网页资料，不是长期记忆；如果和当前问题无关，不要强行使用。
- `scripts/start.bat` 会在 agent-runtime-service 前启动 web-search-service，让 runtime 更容易发现它。
- agent-manager-service 的旧 `graphTool/websearch.go` 暂未删除，因为本次只切 runtime 工具。真正执行 Agent 工具的 runtime 已经接入新 RPC 服务。
- README 末尾原本存在 merge conflict 标记，本次已清理标记本身。

常用环境变量：

```text
WEB_SEARCH_MAX_RESULTS=5
WEB_SEARCH_MAX_FETCH=5
WEB_SEARCH_MAX_PASSAGES=3
WEB_SEARCH_MAX_CHARS_PER_PAGE=12000
WEB_SEARCH_TRUSTED_DOMAINS=gov.cn,edu.cn,who.int,cdc.gov,nih.gov,developer.mozilla.org,docs.github.com,openai.com,cloudwego.io,go.dev,docs.docker.com,kubernetes.io
WEB_SEARCH_USER_AGENT=ClaranAIM-WebSearch/1.0
WEB_SEARCH_TIMEOUT_MS=10000
```

## 7. How To Verify

先运行核心服务测试：

```powershell
$env:GOCACHE='D:\CodeStudy\GoProjects\src\ClaranAIM\.gocache'
go test ./internal/web-search-service/service
```

结果：通过。

再运行受影响包测试：

```powershell
go test ./internal/web-search-service/... ./pkg/websearchclient ./internal/agent-runtime-service/logic ./internal/agent-runtime-service/agent ./cmd/agent-runtime-service ./internal/api-gateway/client ./internal/api-gateway/handler ./internal/api-gateway/router ./cmd/api-gateway ./cmd/web-search-service
```

结果：通过。

最后运行全仓库测试：

```powershell
go test ./...
```

结果：通过。

## 8. What To Read Next

- `internal/web-search-service/service/service.go`: 看搜索增强主链路、正文清洗和相关段落抽取。
- `internal/web-search-service/service/service_test.go`: 看可信源优先和摘要兜底的行为边界。
- `pkg/websearchclient/rpc_client.go`: 看其他服务如何通过 Kitex 调用 web-search-service。
- `internal/agent-runtime-service/logic/web_search_tool.go`: 看 Agent 工具如何把搜索增强结果整理给模型。
- `internal/api-gateway/handler/web_search_handler.go`: 看浏览器调试接口如何绑定参数。
