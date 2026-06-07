package logic

import (
	"ClaranAIM/kitex_gen/mcp_gateway"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"context"
	"encoding/json"
	"strings"
	"sync"
)

type mcpRuntimeContextKey string

const (
	mcpUserIDKey         mcpRuntimeContextKey = "mcp_user_id"
	mcpAgentIDKey        mcpRuntimeContextKey = "mcp_agent_id"
	mcpConversationIDKey mcpRuntimeContextKey = "mcp_conversation_id"
)

var (
	mcpServiceMu sync.RWMutex
	mcpService   mcpgatewayservice.Client
)

// SetMCPService 注入 mcp-gateway-service RPC 客户端。
// Agent 工具统一通过 MCP Gateway 暴露内部服务和用户自定义远程 MCP Server。
func SetMCPService(svc mcpgatewayservice.Client) {
	mcpServiceMu.Lock()
	defer mcpServiceMu.Unlock()
	mcpService = svc
}

// WithMCPRuntimeContext 把当前 Agent 执行身份写入工具上下文。
// 工具调用时会把这些值传给 mcp-gateway-service，用于权限裁剪、远程 MCP 解析和审计记录。
func WithMCPRuntimeContext(ctx context.Context, userID, agentID, conversationID int64) context.Context {
	ctx = context.WithValue(ctx, mcpUserIDKey, userID)
	ctx = context.WithValue(ctx, mcpAgentIDKey, agentID)
	ctx = context.WithValue(ctx, mcpConversationIDKey, conversationID)
	return ctx
}

// MCPWebSearchParams 是 web_search MCP 工具入参。
type MCPWebSearchParams struct {
	Query       string `json:"query" jsonschema:"description=需要联网搜索的问题、关键词、版本、价格或官方文档主题"`
	Limit       int    `json:"limit" jsonschema:"description=返回来源数量，建议 3 到 5；为空时使用服务默认值"`
	MaxFetch    int    `json:"max_fetch" jsonschema:"description=最多抓取多少个网页正文，建议 3 到 5；为空时使用服务默认值"`
	MaxPassages int    `json:"max_passages" jsonschema:"description=每个来源最多截取多少段相关正文，建议 2 到 3；为空时使用服务默认值"`
}

// MCPSearchMemoryParams 是 search_memory MCP 工具入参。
type MCPSearchMemoryParams struct {
	Query            string  `json:"query" jsonschema:"description=要从长期记忆中召回的当前问题、偏好、目标或上下文关键词"`
	Limit            int     `json:"limit" jsonschema:"description=返回记忆条数，建议 3 到 8；为空时使用 5"`
	MinScore         float64 `json:"min_score" jsonschema:"description=最低融合分数，空值使用 memory-service 默认值"`
	VectorCandidateK int     `json:"vector_candidate_k" jsonschema:"description=向量召回候选上限，空值使用 memory-service 默认值"`
	UseLLMFilter     bool    `json:"use_llm_filter" jsonschema:"description=是否启用轻量 LLM 过滤候选记忆"`
}

// MCPSearchKnowledgeParams 是 search_knowledge MCP 工具入参。
type MCPSearchKnowledgeParams struct {
	Query string `json:"query" jsonschema:"description=要查询的项目文档、代码知识、文件知识库或 RAG 问题"`
	Mode  string `json:"mode" jsonschema:"description=检索模式，可选 adaptive、hybrid、graphrag、strict、web；为空时使用 adaptive"`
	Limit int    `json:"limit" jsonschema:"description=返回来源数量，建议 3 到 8；为空时使用 5"`
}

// MCPQueryKnowledgeGraphParams 是 query_knowledge_graph MCP 工具入参。
type MCPQueryKnowledgeGraphParams struct {
	Query           string   `json:"query" jsonschema:"description=实体、服务、表、Topic、概念或关系问题"`
	TypeFilters     []string `json:"type_filters" jsonschema:"description=实体类型过滤，例如 Service、DatabaseTable、EventTopic、API、Module、Concept"`
	RelationFilters []string `json:"relation_filters" jsonschema:"description=关系类型过滤，例如 CALLS、PUBLISHES、CONSUMES、WRITES、DEPENDS_ON"`
	CommunityID     int64    `json:"community_id" jsonschema:"description=只查询某个图谱社区，空值表示不过滤"`
	Hops            int      `json:"hops" jsonschema:"description=邻域扩展跳数，建议 1 到 2"`
	Limit           int      `json:"limit" jsonschema:"description=返回节点上限，建议 30 到 80；为空时使用 50"`
}

// MCPSummarizeConversationParams 是 summarize_conversation MCP 工具入参。
type MCPSummarizeConversationParams struct {
	ConversationID int64  `json:"conversation_id" jsonschema:"description=要总结的会话ID；为空时使用当前会话"`
	StartMessageID int64  `json:"start_message_id" jsonschema:"description=起始消息ID，可为空"`
	EndMessageID   int64  `json:"end_message_id" jsonschema:"description=结束消息ID，可为空"`
	StartTime      string `json:"start_time" jsonschema:"description=起始时间，支持 RFC3339 或 yyyy-MM-dd HH:mm:ss，可为空"`
	EndTime        string `json:"end_time" jsonschema:"description=结束时间，支持 RFC3339 或 yyyy-MM-dd HH:mm:ss，可为空"`
	ProcessNow     bool   `json:"process_now" jsonschema:"description=是否立即处理任务；true 会同步等待归档结果"`
}

// MCPListToolsParams 是 list_mcp_tools 工具入参。
type MCPListToolsParams struct {
	IncludeSchema bool `json:"include_schema" jsonschema:"description=是否返回每个工具的输入 JSON Schema；默认只返回工具名、来源和说明"`
}

// MCPCallToolParams 是 call_mcp_tool 工具入参。
// 它用于调用用户在 settings-service 中配置的远程 MCP 工具，也可调试内置 MCP 工具。
type MCPCallToolParams struct {
	ToolName      string `json:"tool_name" jsonschema:"description=要调用的 MCP 工具名，例如远程 MCP 返回的 github_search"`
	ArgumentsJSON string `json:"arguments_json" jsonschema:"description=传给该工具的 JSON 字符串参数，例如 {\"query\":\"...\"}"`
}

// MCPWebSearch 通过 mcp-gateway-service 调用 web_search 工具。
func MCPWebSearch(ctx context.Context, input *MCPWebSearchParams) (string, error) {
	if input == nil {
		input = &MCPWebSearchParams{}
	}
	if strings.TrimSpace(input.Query) == "" {
		return "web_search 调用失败：query不能为空。", nil
	}
	return callMCPTool(ctx, "web_search", input)
}

// MCPSearchMemory 通过 mcp-gateway-service 调用 search_memory 工具。
func MCPSearchMemory(ctx context.Context, input *MCPSearchMemoryParams) (string, error) {
	if input == nil {
		input = &MCPSearchMemoryParams{}
	}
	if strings.TrimSpace(input.Query) == "" {
		return "search_memory 调用失败：query不能为空。", nil
	}
	return callMCPTool(ctx, "search_memory", input)
}

// MCPSearchKnowledge 通过 mcp-gateway-service 调用 search_knowledge 工具。
func MCPSearchKnowledge(ctx context.Context, input *MCPSearchKnowledgeParams) (string, error) {
	if input == nil {
		input = &MCPSearchKnowledgeParams{}
	}
	if strings.TrimSpace(input.Query) == "" {
		return "search_knowledge 调用失败：query不能为空。", nil
	}
	return callMCPTool(ctx, "search_knowledge", input)
}

// MCPQueryKnowledgeGraph 通过 mcp-gateway-service 调用 query_knowledge_graph 工具。
func MCPQueryKnowledgeGraph(ctx context.Context, input *MCPQueryKnowledgeGraphParams) (string, error) {
	if input == nil {
		input = &MCPQueryKnowledgeGraphParams{}
	}
	return callMCPTool(ctx, "query_knowledge_graph", input)
}

// MCPSummarizeConversation 通过 mcp-gateway-service 调用 summarize_conversation 工具。
func MCPSummarizeConversation(ctx context.Context, input *MCPSummarizeConversationParams) (string, error) {
	if input == nil {
		input = &MCPSummarizeConversationParams{}
	}
	return callMCPTool(ctx, "summarize_conversation", input)
}

// MCPListTools 返回当前 Agent 上下文可用的内置和远程 MCP 工具。
func MCPListTools(ctx context.Context, input *MCPListToolsParams) (string, error) {
	mcpServiceMu.RLock()
	svc := mcpService
	mcpServiceMu.RUnlock()
	if svc == nil {
		return "MCP工具发现不可用：agent-runtime-service 尚未连接 mcp-gateway-service。", nil
	}
	userID, _ := ctx.Value(mcpUserIDKey).(int64)
	if userID <= 0 {
		return "MCP工具发现失败：缺少当前用户上下文。", nil
	}
	agentID, _ := ctx.Value(mcpAgentIDKey).(int64)
	conversationID, _ := ctx.Value(mcpConversationIDKey).(int64)
	toolsResp, err := svc.ListTools(ctx, &mcp_gateway.ListToolsReq{UserId: userID, AgentId: agentID, ConversationId: conversationID})
	if err != nil {
		return "", err
	}
	if !toolsResp.GetSuccess() {
		return firstNonEmptyTool(toolsResp.GetMsg(), "MCP工具发现失败"), nil
	}
	includeSchema := input != nil && input.IncludeSchema
	type visibleTool struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		Source           string `json:"source"`
		ServerName       string `json:"server_name"`
		RequiresApproval bool   `json:"requires_approval"`
		InputSchemaJSON  string `json:"input_schema_json,omitempty"`
	}
	out := make([]visibleTool, 0, len(toolsResp.GetTools()))
	for _, tool := range toolsResp.GetTools() {
		if tool == nil {
			continue
		}
		item := visibleTool{
			Name:             tool.GetName(),
			Description:      tool.GetDescription(),
			Source:           tool.GetSource(),
			ServerName:       tool.GetServerName(),
			RequiresApproval: tool.GetRequiresApproval(),
		}
		if includeSchema {
			item.InputSchemaJSON = tool.GetInputSchemaJson()
		}
		out = append(out, item)
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return "当前可用 MCP 工具：\n" + string(data), nil
}

// MCPCallTool 按工具名调用任意当前上下文可用的 MCP 工具。
func MCPCallTool(ctx context.Context, input *MCPCallToolParams) (string, error) {
	if input == nil || strings.TrimSpace(input.ToolName) == "" {
		return "call_mcp_tool 调用失败：tool_name不能为空。", nil
	}
	args := strings.TrimSpace(input.ArgumentsJSON)
	if args == "" {
		args = "{}"
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil || parsed == nil {
		return "call_mcp_tool 调用失败：arguments_json必须是合法的JSON对象，例如 {\"query\":\"...\"}。", nil
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return callMCPToolRaw(ctx, strings.TrimSpace(input.ToolName), string(normalized))
}

func callMCPTool(ctx context.Context, toolName string, args interface{}) (string, error) {
	payload, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return callMCPToolRaw(ctx, toolName, string(payload))
}

func callMCPToolRaw(ctx context.Context, toolName, argumentsJSON string) (string, error) {
	mcpServiceMu.RLock()
	svc := mcpService
	mcpServiceMu.RUnlock()
	if svc == nil {
		return "MCP工具不可用：agent-runtime-service 尚未连接 mcp-gateway-service。", nil
	}
	userID, _ := ctx.Value(mcpUserIDKey).(int64)
	if userID <= 0 {
		return "MCP工具调用失败：缺少当前用户上下文，无法进行权限裁剪和审计。", nil
	}
	agentID, _ := ctx.Value(mcpAgentIDKey).(int64)
	conversationID, _ := ctx.Value(mcpConversationIDKey).(int64)
	result, err := svc.CallTool(ctx, &mcp_gateway.CallToolReq{
		UserId:         userID,
		AgentId:        agentID,
		ConversationId: conversationID,
		ToolName:       toolName,
		ArgumentsJson:  argumentsJSON,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.GetResultText()) != "" {
		return result.GetResultText(), nil
	}
	if strings.TrimSpace(result.GetResultJson()) != "" {
		return result.GetResultJson(), nil
	}
	if result.GetMsg() != "" {
		return result.GetMsg(), nil
	}
	return "MCP工具调用完成，但没有返回可读内容。", nil
}
