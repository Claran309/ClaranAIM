// Package service 实现 MCP Gateway 工具发现、调用和审计。
package service

import (
	"ClaranAIM/internal/mcp-gateway-service/dao"
	traceModel "ClaranAIM/internal/mcp-gateway-service/model"
	"ClaranAIM/kitex_gen/conversation_intelligence"
	"ClaranAIM/kitex_gen/conversation_intelligence/conversationintelligenceservice"
	"ClaranAIM/kitex_gen/knowledge"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/mcp_gateway"
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/settings"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/kitex_gen/web_search"
	"ClaranAIM/kitex_gen/web_search/websearchservice"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/observability"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ToolWebSearch             = "web_search"
	ToolSearchMemory          = "search_memory"
	ToolSearchKnowledge       = "search_knowledge"
	ToolQueryKnowledgeGraph   = "query_knowledge_graph"
	ToolSummarizeConversation = "summarize_conversation"

	ToolSourceBuiltin = "builtin"
	ToolSourceRemote  = "remote"
)

// MCPGatewayService 是 mcp-gateway-service 的业务接口。
type MCPGatewayService interface {
	ListTools(ctx context.Context, input ToolContext) ([]Tool, error)
	CallTool(ctx context.Context, input CallToolInput) (CallToolResult, error)
	GetToolSchema(ctx context.Context, input ToolSchemaInput) (*Tool, error)
	ListToolCalls(ctx context.Context, input TraceListInput) ([]TraceDTO, int64, error)
	GetToolCallTrace(ctx context.Context, userID int64, traceID string) (*TraceDTO, error)
}

// Dependencies 聚合 MCP Gateway 调用的下游服务客户端。
type Dependencies struct {
	Repo             dao.Repository
	Settings         settingsservice.Client
	WebSearch        websearchservice.Client
	Memory           memoryservice.Client
	RAG              ragservice.Client
	Knowledge        knowledgeservice.Client
	Conversation     conversationintelligenceservice.Client
	RemoteHTTPClient *http.Client
}

// ToolContext 描述工具发现上下文。
type ToolContext struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
}

// CallToolInput 描述工具调用上下文。
type CallToolInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	ToolName       string
	ArgumentsJSON  string
	TraceID        string
}

// ToolSchemaInput 描述工具 schema 查询上下文。
type ToolSchemaInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	ToolName       string
}

// TraceListInput 描述审计列表查询。
type TraceListInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	Limit          int
	Offset         int
}

// Tool 是内部服务层工具 DTO。
type Tool = mcp_gateway.MCPTool

// CallToolResult 是服务层工具调用结果。
type CallToolResult = mcp_gateway.CallToolResp

// TraceDTO 是工具审计 DTO。
type TraceDTO = mcp_gateway.MCPToolCallTrace

type mcpGatewayServiceImpl struct {
	deps Dependencies
}

// NewMCPGatewayService 创建 MCP Gateway 服务。
func NewMCPGatewayService(deps Dependencies) MCPGatewayService {
	if deps.RemoteHTTPClient == nil {
		deps.RemoteHTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &mcpGatewayServiceImpl{deps: deps}
}

// ListTools 返回内置工具和当前上下文可用的远程 MCP 工具。
func (s *mcpGatewayServiceImpl) ListTools(ctx context.Context, input ToolContext) ([]Tool, error) {
	if input.UserID <= 0 {
		return nil, errors.New("用户不能为空")
	}
	tools := builtinTools()
	remoteTools, _ := s.listRemoteTools(ctx, input)
	tools = append(tools, remoteTools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// GetToolSchema 返回单个工具 schema。
func (s *mcpGatewayServiceImpl) GetToolSchema(ctx context.Context, input ToolSchemaInput) (*Tool, error) {
	tools, err := s.ListTools(ctx, ToolContext{UserID: input.UserID, AgentID: input.AgentID, ConversationID: input.ConversationID})
	if err != nil {
		return nil, err
	}
	for i := range tools {
		if tools[i].Name == input.ToolName {
			return &tools[i], nil
		}
	}
	return nil, errors.New("工具不存在或当前上下文不可用")
}

// CallTool 调用内置或远程 MCP 工具，并记录审计。
func (s *mcpGatewayServiceImpl) CallTool(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	start := time.Now()
	if input.UserID <= 0 {
		return CallToolResult{Success: false, ToolName: input.ToolName, Msg: "用户不能为空"}, nil
	}
	traceID := strings.TrimSpace(input.TraceID)
	if traceID == "" {
		if id, err := idgen.NextID(); err == nil {
			traceID = fmt.Sprintf("mcp_%d", id)
		} else {
			traceID = fmt.Sprintf("mcp_%d", time.Now().UnixNano())
		}
	}
	result, source, serverName, err := s.callTool(ctx, input)
	status := traceModel.TraceStatusSuccess
	errorMessage := ""
	if err != nil {
		status = traceModel.TraceStatusFailed
		errorMessage = err.Error()
		result = CallToolResult{Success: false, ToolName: input.ToolName, TraceId: traceID, Msg: err.Error()}
	} else {
		result.TraceId = traceID
	}
	metricStatus := "success"
	if status == traceModel.TraceStatusFailed {
		metricStatus = "failed"
	}
	observability.RecordBusinessDuration("mcp", strings.TrimSpace(input.ToolName), metricStatus, time.Since(start))
	observability.RecordBusinessEvent("mcp", strings.TrimSpace(input.ToolName), metricStatus)
	s.saveTrace(ctx, input, traceID, source, serverName, status, time.Since(start), errorMessage)
	return result, nil
}

func (s *mcpGatewayServiceImpl) callTool(ctx context.Context, input CallToolInput) (CallToolResult, string, string, error) {
	name := strings.TrimSpace(input.ToolName)
	switch name {
	case ToolWebSearch:
		resp, err := s.callWebSearch(ctx, input)
		return resp, ToolSourceBuiltin, "web-search-service", err
	case ToolSearchMemory:
		resp, err := s.callSearchMemory(ctx, input)
		return resp, ToolSourceBuiltin, "memory-service", err
	case ToolSearchKnowledge:
		resp, err := s.callSearchKnowledge(ctx, input)
		return resp, ToolSourceBuiltin, "rag-service", err
	case ToolQueryKnowledgeGraph:
		resp, err := s.callQueryKnowledgeGraph(ctx, input)
		return resp, ToolSourceBuiltin, "knowledge-service", err
	case ToolSummarizeConversation:
		resp, err := s.callSummarizeConversation(ctx, input)
		return resp, ToolSourceBuiltin, "conversation-intelligence-service", err
	default:
		resp, server, err := s.callRemoteTool(ctx, input)
		return resp, ToolSourceRemote, server, err
	}
}

func (s *mcpGatewayServiceImpl) callWebSearch(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	if s.deps.WebSearch == nil {
		return CallToolResult{}, errors.New("web-search-service未连接")
	}
	var args struct {
		Query       string `json:"query"`
		Limit       int    `json:"limit"`
		MaxFetch    int    `json:"max_fetch"`
		MaxPassages int    `json:"max_passages"`
	}
	if err := decodeArgs(input.ArgumentsJSON, &args); err != nil {
		return CallToolResult{}, err
	}
	if strings.TrimSpace(args.Query) == "" {
		return CallToolResult{}, errors.New("query不能为空")
	}
	result, err := s.deps.WebSearch.Augment(ctx, &web_search.AugmentReq{
		Query:       args.Query,
		Limit:       int64(defaultInt(args.Limit, 5)),
		MaxFetch:    int64(defaultInt(args.MaxFetch, 3)),
		MaxPassages: int64(defaultInt(args.MaxPassages, 6)),
	})
	if err != nil {
		return CallToolResult{}, err
	}
	if !result.GetSuccess() {
		return CallToolResult{}, errors.New(defaultString(result.GetMsg(), "web-search-service调用失败"))
	}
	return toolResult(ToolWebSearch, formatJSON(result), result.GetAnswerContext()), nil
}

func (s *mcpGatewayServiceImpl) callSearchMemory(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	if s.deps.Memory == nil {
		return CallToolResult{}, errors.New("memory-service未连接")
	}
	var args struct {
		Query            string  `json:"query"`
		Limit            int     `json:"limit"`
		MinScore         float64 `json:"min_score"`
		VectorCandidateK int     `json:"vector_candidate_k"`
		UseLLMFilter     bool    `json:"use_llm_filter"`
	}
	if err := decodeArgs(input.ArgumentsJSON, &args); err != nil {
		return CallToolResult{}, err
	}
	result, err := s.deps.Memory.Recall(ctx, &memory.RecallReq{
		BotId:            input.AgentID,
		UserId:           input.UserID,
		ConversationId:   input.ConversationID,
		Query:            args.Query,
		Limit:            int64(defaultInt(args.Limit, 5)),
		MinScore:         args.MinScore,
		VectorCandidateK: int64(args.VectorCandidateK),
		UseLlmFilter:     args.UseLLMFilter,
	})
	if err != nil {
		return CallToolResult{}, err
	}
	if !result.GetSuccess() {
		return CallToolResult{}, errors.New(defaultString(result.GetMsg(), "memory-service调用失败"))
	}
	return toolResult(ToolSearchMemory, formatJSON(result), result.GetContextText()), nil
}

func (s *mcpGatewayServiceImpl) callSearchKnowledge(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	if s.deps.RAG == nil {
		return CallToolResult{}, errors.New("rag-service未连接")
	}
	var args struct {
		Query string `json:"query"`
		Mode  string `json:"mode"`
		Limit int    `json:"limit"`
	}
	if err := decodeArgs(input.ArgumentsJSON, &args); err != nil {
		return CallToolResult{}, err
	}
	if strings.TrimSpace(args.Query) == "" {
		return CallToolResult{}, errors.New("query不能为空")
	}
	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		mode = "hybrid"
	}
	resp, err := s.deps.RAG.Search(ctx, &rag.SearchReq{ViewerId: input.UserID, Query: args.Query, Mode: mode, Limit: int64(defaultInt(args.Limit, 5)), ConversationId: input.ConversationID})
	if err != nil {
		return CallToolResult{}, err
	}
	if !resp.GetSuccess() {
		return CallToolResult{}, errors.New(defaultString(resp.GetMsg(), "rag-service调用失败"))
	}
	return toolResult(ToolSearchKnowledge, formatJSON(resp), resp.GetAnswer()), nil
}

func (s *mcpGatewayServiceImpl) callQueryKnowledgeGraph(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	if s.deps.Knowledge == nil {
		return CallToolResult{}, errors.New("knowledge-service未连接")
	}
	var args struct {
		Query           string   `json:"query"`
		TypeFilters     []string `json:"type_filters"`
		RelationFilters []string `json:"relation_filters"`
		CommunityID     int64    `json:"community_id"`
		Hops            int      `json:"hops"`
		Limit           int      `json:"limit"`
	}
	if err := decodeArgs(input.ArgumentsJSON, &args); err != nil {
		return CallToolResult{}, err
	}
	graph, err := s.deps.Knowledge.GetGraphView(ctx, &knowledge.KnowledgeGraphReq{
		ViewerId:        input.UserID,
		Query:           args.Query,
		TypeFilters:     args.TypeFilters,
		RelationFilters: args.RelationFilters,
		CommunityId:     args.CommunityID,
		Hops:            int64(args.Hops),
		Limit:           int64(defaultInt(args.Limit, 50)),
	})
	if err != nil {
		return CallToolResult{}, err
	}
	if !graph.GetSuccess() {
		return CallToolResult{}, errors.New(defaultString(graph.GetMsg(), "knowledge-service调用失败"))
	}
	return toolResult(ToolQueryKnowledgeGraph, formatJSON(graph), summarizeGraph(graph)), nil
}

func (s *mcpGatewayServiceImpl) callSummarizeConversation(ctx context.Context, input CallToolInput) (CallToolResult, error) {
	if s.deps.Conversation == nil {
		return CallToolResult{}, errors.New("conversation-intelligence-service未连接")
	}
	var args struct {
		ConversationID int64  `json:"conversation_id"`
		StartMessageID int64  `json:"start_message_id"`
		EndMessageID   int64  `json:"end_message_id"`
		StartTime      string `json:"start_time"`
		EndTime        string `json:"end_time"`
		ProcessNow     bool   `json:"process_now"`
	}
	if err := decodeArgs(input.ArgumentsJSON, &args); err != nil {
		return CallToolResult{}, err
	}
	if args.ConversationID <= 0 {
		args.ConversationID = int64Arg(input.ArgumentsJSON, "conversation_id")
	}
	conversationID := input.ConversationID
	if conversationID <= 0 && args.ConversationID > 0 {
		conversationID = args.ConversationID
	} else if conversationID > 0 && args.ConversationID > 0 && args.ConversationID != conversationID {
		// Agent 工具调用时模型有时会把 Agent 用户 ID 当成会话 ID。当前运行上下文里的 conversation_id
		// 才是权限校验过的真实会话，因此优先使用它，避免 summarize_conversation 误报“会话不存在”。
		args.ConversationID = conversationID
	}
	if conversationID <= 0 {
		return CallToolResult{}, errors.New("conversation_id不能为空")
	}
	job, err := s.deps.Conversation.CreateDigestJob(ctx, &conversation_intelligence.CreateDigestJobReq{
		ConversationId: conversationID,
		ViewerId:       input.UserID,
		AgentId:        input.AgentID,
		StartMessageId: args.StartMessageID,
		EndMessageId:   args.EndMessageID,
		StartTime:      args.StartTime,
		EndTime:        args.EndTime,
		Reason:         "mcp_summarize_conversation",
	})
	if err != nil {
		return CallToolResult{}, err
	}
	if !job.GetSuccess() {
		return CallToolResult{}, errors.New(defaultString(job.GetMsg(), "conversation-intelligence-service调用失败"))
	}
	if args.ProcessNow {
		processed, err := s.deps.Conversation.ProcessDigestJob(ctx, &conversation_intelligence.ProcessDigestJobReq{JobId: job.GetJob().GetId(), ViewerId: input.UserID})
		if err != nil {
			return CallToolResult{}, err
		}
		if !processed.GetSuccess() {
			return CallToolResult{}, errors.New(defaultString(processed.GetMsg(), "conversation-intelligence-service处理失败"))
		}
		return toolResult(ToolSummarizeConversation, formatJSON(processed), summarizeArtifacts(processed.GetArtifacts())), nil
	}
	return toolResult(ToolSummarizeConversation, formatJSON(job), fmt.Sprintf("已创建会话总结任务：%d，状态：%s", job.GetJob().GetId(), job.GetJob().GetStatus())), nil
}

func (s *mcpGatewayServiceImpl) listRemoteTools(ctx context.Context, input ToolContext) ([]Tool, error) {
	servers, err := s.resolveRemoteServers(ctx, input)
	if err != nil {
		return nil, err
	}
	var out []Tool
	for _, server := range servers {
		items, err := s.remoteToolsList(ctx, server)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func (s *mcpGatewayServiceImpl) resolveRemoteServers(ctx context.Context, input ToolContext) ([]*settings.MCPServerConfig, error) {
	if s.deps.Settings == nil {
		return nil, nil
	}
	resp, err := s.deps.Settings.ResolveMCPServers(ctx, &settings.ResolveMCPServersReq{UserId: input.UserID, AgentId: input.AgentID, ConversationId: input.ConversationID})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, errors.New(defaultString(resp.GetMsg(), "settings-service解析MCP配置失败"))
	}
	return resp.GetServers(), nil
}

func (s *mcpGatewayServiceImpl) remoteToolsList(ctx context.Context, server *settings.MCPServerConfig) ([]Tool, error) {
	if server == nil || (server.GetTransport() != "streamable_http" && server.GetTransport() != "sse") {
		return nil, nil
	}
	resp, err := s.remoteJSONRPC(ctx, server, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp, &decoded); err != nil {
		return nil, err
	}
	allow := parseStringSet(server.GetAllowToolsJson())
	deny := parseStringSet(server.GetDenyToolsJson())
	out := make([]Tool, 0, len(decoded.Tools))
	for _, item := range decoded.Tools {
		name := strings.TrimSpace(item.Name)
		if name == "" || deny[name] || (len(allow) > 0 && !allow[name]) {
			continue
		}
		out = append(out, Tool{Name: name, Description: item.Description, Source: ToolSourceRemote, ServerName: server.GetName(), InputSchemaJson: string(item.InputSchema), RequiresApproval: remoteRequiresApproval(server)})
	}
	return out, nil
}

func (s *mcpGatewayServiceImpl) callRemoteTool(ctx context.Context, input CallToolInput) (CallToolResult, string, error) {
	servers, err := s.resolveRemoteServers(ctx, ToolContext{UserID: input.UserID, AgentID: input.AgentID, ConversationID: input.ConversationID})
	if err != nil {
		return CallToolResult{}, "", err
	}
	for _, server := range servers {
		tools, _ := s.remoteToolsList(ctx, server)
		for _, tool := range tools {
			if tool.Name != input.ToolName {
				continue
			}
			args, err := rawJSONMap(input.ArgumentsJSON)
			if err != nil {
				return CallToolResult{}, server.Name, err
			}
			params := map[string]interface{}{"name": input.ToolName, "arguments": args}
			result, err := s.remoteJSONRPC(ctx, server, "tools/call", params)
			if err != nil {
				return CallToolResult{}, server.Name, err
			}
			return toolResult(input.ToolName, string(result), remoteResultText(result)), server.Name, nil
		}
	}
	return CallToolResult{}, "", errors.New("远程MCP工具不存在或不在allowlist中")
}

func (s *mcpGatewayServiceImpl) remoteJSONRPC(ctx context.Context, server *settings.MCPServerConfig, method string, params interface{}) (json.RawMessage, error) {
	if server == nil {
		return nil, errors.New("远程MCP配置为空")
	}
	endpoint := strings.TrimSpace(server.GetEndpointUrl())
	if endpoint == "" {
		return nil, errors.New("远程MCP endpoint_url为空")
	}
	payload := map[string]interface{}{"jsonrpc": "2.0", "id": fmt.Sprintf("%d", time.Now().UnixNano()), "method": method, "params": params}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	applyRemoteHeaders(req, server)
	resp, err := s.deps.RemoteHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("远程MCP状态码%d: %s", resp.StatusCode, string(data))
	}
	var decoded struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("远程MCP错误%d: %s", decoded.Error.Code, decoded.Error.Message)
	}
	return decoded.Result, nil
}

func applyRemoteHeaders(req *http.Request, server *settings.MCPServerConfig) {
	if server == nil {
		return
	}
	for k, v := range parseStringMap(server.GetHeadersJson()) {
		req.Header.Set(k, v)
	}
	secret := strings.TrimSpace(server.GetSecret())
	if secret == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(server.GetAuthType())) {
	case "bearer", "token", "":
		req.Header.Set("Authorization", "Bearer "+secret)
	case "basic":
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(secret)))
	case "api_key":
		req.Header.Set("X-API-Key", secret)
	}
}

func (s *mcpGatewayServiceImpl) saveTrace(ctx context.Context, input CallToolInput, traceID, source, serverName, status string, latency time.Duration, errorMessage string) {
	if s.deps.Repo == nil {
		return
	}
	_ = s.deps.Repo.SaveTrace(ctx, &traceModel.ToolCallTrace{
		UserID:         input.UserID,
		AgentID:        input.AgentID,
		ConversationID: input.ConversationID,
		ToolName:       input.ToolName,
		Source:         defaultString(source, ToolSourceBuiltin),
		ServerName:     serverName,
		TraceID:        traceID,
		Status:         status,
		LatencyMS:      latency.Milliseconds(),
		ErrorMessage:   errorMessage,
	})
}

// ListToolCalls 返回工具审计列表。
func (s *mcpGatewayServiceImpl) ListToolCalls(ctx context.Context, input TraceListInput) ([]TraceDTO, int64, error) {
	if s.deps.Repo == nil {
		return nil, 0, errors.New("mcp trace repository未配置")
	}
	traces, total, err := s.deps.Repo.ListTraces(ctx, dao.TraceFilter{UserID: input.UserID, AgentID: input.AgentID, ConversationID: input.ConversationID, Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, 0, err
	}
	return tracesToRPC(traces), total, nil
}

// GetToolCallTrace 读取单条工具审计。
func (s *mcpGatewayServiceImpl) GetToolCallTrace(ctx context.Context, userID int64, traceID string) (*TraceDTO, error) {
	if s.deps.Repo == nil {
		return nil, errors.New("mcp trace repository未配置")
	}
	trace, err := s.deps.Repo.GetTraceByTraceID(ctx, userID, traceID)
	if err != nil || trace == nil {
		return nil, err
	}
	item := traceToRPC(*trace)
	return &item, nil
}

func builtinTools() []Tool {
	return []Tool{
		{Name: ToolWebSearch, Description: "查询外部实时资料、官方文档、价格、版本、API。", Source: ToolSourceBuiltin, ServerName: "web-search-service", InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"max_fetch":{"type":"integer"},"max_passages":{"type":"integer"}},"required":["query"]}`},
		{Name: ToolSearchMemory, Description: "查询用户/会话/Agent 长期记忆。", Source: ToolSourceBuiltin, ServerName: "memory-service", InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"min_score":{"type":"number"},"use_llm_filter":{"type":"boolean"}}}`},
		{Name: ToolSearchKnowledge, Description: "查项目文档、文件知识库、RAG chunks。", Source: ToolSourceBuiltin, ServerName: "rag-service", InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string"},"mode":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`},
		{Name: ToolQueryKnowledgeGraph, Description: "查实体、关系、子图、社区摘要。", Source: ToolSourceBuiltin, ServerName: "knowledge-service", InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string"},"type_filters":{"type":"array","items":{"type":"string"}},"relation_filters":{"type":"array","items":{"type":"string"}},"community_id":{"type":"integer"},"hops":{"type":"integer"},"limit":{"type":"integer"}}}`},
		{Name: ToolSummarizeConversation, Description: "手动让 Agent 总结某个会话。", Source: ToolSourceBuiltin, ServerName: "conversation-intelligence-service", InputSchemaJson: `{"type":"object","properties":{"conversation_id":{"type":"integer"},"start_message_id":{"type":"integer"},"end_message_id":{"type":"integer"},"start_time":{"type":"string"},"end_time":{"type":"string"},"process_now":{"type":"boolean"}}}`},
	}
}

func decodeArgs(raw string, v interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), v)
}

func int64Arg(raw, key string) int64 {
	args, err := rawJSONMap(raw)
	if err != nil {
		return 0
	}
	value, ok := args[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func toolResult(name, resultJSON, text string) CallToolResult {
	if strings.TrimSpace(text) == "" {
		text = resultJSON
	}
	return CallToolResult{Success: true, ToolName: name, ResultJson: resultJSON, ResultText: text, Msg: "ok"}
}

func formatJSON(value interface{}) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func defaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseStringSet(raw string) map[string]bool {
	out := map[string]bool{}
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return out
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func parseStringMap(raw string) map[string]string {
	out := map[string]string{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return map[string]string{}
	}
	return out
}

func rawJSONMap(raw string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return nil, errors.New("arguments_json必须是合法的JSON对象")
	}
	return out, nil
}

func remoteRequiresApproval(server *settings.MCPServerConfig) bool {
	return server != nil && server.GetTrustLevel() == "low"
}

func remoteResultText(raw json.RawMessage) string {
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &decoded); err == nil && len(decoded.Content) > 0 {
		var parts []string
		for _, item := range decoded.Content {
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(raw)
}

func summarizeGraph(graph *knowledge.KnowledgeGraphResp) string {
	if graph == nil {
		return "知识图谱查询无结果。"
	}
	stats := graph.GetStats()
	return fmt.Sprintf("知识图谱命中：%d 个节点、%d 条边、%d 个社区。", stats.GetNodeCount(), stats.GetEdgeCount(), stats.GetCommunityCount())
}

func summarizeArtifacts(artifacts []*conversation_intelligence.ConversationArtifact) string {
	if len(artifacts) == 0 {
		return "会话总结任务已处理，但没有产出摘要。"
	}
	var b strings.Builder
	for _, item := range artifacts {
		if item == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n%s\n\n", item.GetTitle(), item.GetContent()))
	}
	return strings.TrimSpace(b.String())
}

func tracesToRPC(traces []traceModel.ToolCallTrace) []TraceDTO {
	out := make([]TraceDTO, 0, len(traces))
	for _, trace := range traces {
		out = append(out, traceToRPC(trace))
	}
	return out
}

func traceToRPC(trace traceModel.ToolCallTrace) TraceDTO {
	return TraceDTO{Id: trace.ID, UserId: trace.UserID, AgentId: trace.AgentID, ConversationId: trace.ConversationID, ToolName: trace.ToolName, Source: trace.Source, ServerName: trace.ServerName, TraceId: trace.TraceID, Status: trace.Status, LatencyMs: trace.LatencyMS, ErrorMessage: trace.ErrorMessage, CreatedAt: trace.CreatedAt.Format(time.RFC3339)}
}
