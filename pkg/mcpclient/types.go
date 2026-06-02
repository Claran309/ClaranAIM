// Package mcpclient 定义调用 mcp-gateway-service 的稳定客户端契约。
package mcpclient

import "context"

// Tool 描述 Agent 可调用的 MCP 工具。
type Tool struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Source           string `json:"source"`
	ServerName       string `json:"server_name"`
	InputSchemaJSON  string `json:"input_schema_json"`
	RequiresApproval bool   `json:"requires_approval"`
}

// ListToolsInput 描述一次工具列表查询上下文。
type ListToolsInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
}

// CallToolInput 描述一次工具调用。
type CallToolInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	ToolName       string
	ArgumentsJSON  string
	TraceID        string
}

// ListToolCallsInput 描述工具调用审计列表查询条件。
type ListToolCallsInput struct {
	UserID         int64
	AgentID        int64
	ConversationID int64
	Limit          int
	Offset         int
}

// CallToolResult 是 MCP 工具执行结果。
type CallToolResult struct {
	Success    bool   `json:"success"`
	ToolName   string `json:"tool_name"`
	ResultText string `json:"result_text"`
	ResultJSON string `json:"result_json"`
	TraceID    string `json:"trace_id"`
	Msg        string `json:"msg"`
}

// ToolCallTrace 是 MCP 网关审计记录摘要。
type ToolCallTrace struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	AgentID        int64  `json:"agent_id"`
	ConversationID int64  `json:"conversation_id"`
	ToolName       string `json:"tool_name"`
	Source         string `json:"source"`
	ServerName     string `json:"server_name"`
	TraceID        string `json:"trace_id"`
	Status         string `json:"status"`
	LatencyMS      int64  `json:"latency_ms"`
	ErrorMessage   string `json:"error_message"`
	CreatedAt      string `json:"created_at"`
}

// Service 是 agent-runtime-service 调用 mcp-gateway-service 的最小接口。
type Service interface {
	ListTools(ctx context.Context, input ListToolsInput) ([]Tool, error)
	CallTool(ctx context.Context, input CallToolInput) (CallToolResult, error)
	ListToolCalls(ctx context.Context, input ListToolCallsInput) ([]ToolCallTrace, int64, error)
	GetToolCallTrace(ctx context.Context, userID int64, traceID string) (*ToolCallTrace, error)
}
